package server

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/dmiller/foyer/internal/auth"
	"github.com/dmiller/foyer/internal/config"
	"github.com/dmiller/foyer/internal/health"
	"github.com/dmiller/foyer/internal/ws"
)

// maxBodySize limits JSON request bodies to 1 MB.
const maxBodySize = 1 << 20

// httpClient is a shared client with timeout for outbound requests.
var httpClient = &http.Client{Timeout: 10 * time.Second}

func New(cfg *config.Config, db *sql.DB, collector *health.Collector, hub *ws.Hub, frontendFS embed.FS, devMode bool) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	authService := auth.New(cfg.JWTSecret, cfg.CookieDomain)
	healthHandler := health.NewHandler(collector)
	sshStore := auth.NewSSHKeyStore(cfg.AuthorizedKeys)
	apiAuthMiddleware := auth.CombinedAuthMiddleware(sshStore, cfg.APIKeys)

	r.Route("/api", func(r chi.Router) {
		// Auth routes (public)
		r.Post("/auth/login", loginHandler(authService, db))
		r.Post("/auth/register", registerHandler(authService, db, cfg))
		r.Post("/auth/logout", logoutHandler(authService))
		r.Get("/auth/signups", signupsEnabledHandler(cfg))

		// Health API (API key or SSH key auth)
		r.Group(func(r chi.Router) {
			r.Use(apiAuthMiddleware)
			r.Get("/health", healthHandler.GetAll)
			r.Get("/health/cpu", healthHandler.GetCPU)
			r.Get("/health/memory", healthHandler.GetMemory)
			r.Get("/health/disk", healthHandler.GetDisk)
			r.Get("/health/network", healthHandler.GetNetwork)
			r.Get("/health/gpu", healthHandler.GetGPU)
			r.Get("/health/docker", healthHandler.GetDocker)
			r.Get("/health/system", healthHandler.GetSystem)
		})

		// Webhook receiver (API key or SSH key auth)
		r.Group(func(r chi.Router) {
			r.Use(apiAuthMiddleware)
			r.Post("/webhooks", webhookHandler(db))
		})

		// Authenticated routes (JWT)
		if cfg.Mode == "full" {
			r.Group(func(r chi.Router) {
				r.Use(auth.Verifier(authService.TokenAuth()))
				r.Use(auth.RequireAuth)

				r.Get("/auth/me", meHandler())

				r.Get("/services", listServicesHandler(db))
				r.Get("/services/{id}/history", serviceHistoryHandler(db))

				r.Get("/messages", listMessagesHandler(db))
				r.With(auth.RequireAdmin).Post("/messages", createMessageHandler(db))
				r.With(auth.RequireAdmin).Put("/messages/{id}", updateMessageHandler(db))
				r.With(auth.RequireAdmin).Delete("/messages/{id}", deleteMessageHandler(db))

				r.Post("/files", uploadFileHandler(db, cfg.DataDir))
				r.Get("/files", listFilesHandler(db))
				r.Delete("/files/{id}", deleteFileHandler(db, cfg.DataDir))

				r.Post("/pastes", createPasteHandler(db))
				r.Get("/pastes", listPastesHandler(db))
				r.Delete("/pastes/{id}", deletePasteHandler(db))

				r.Get("/webhooks/feed", webhookFeedHandler(db))

				if cfg.Jellyfin != nil {
					r.Get("/jellyfin/streams", jellyfinStreamsHandler(cfg.Jellyfin))
				}

				r.Get("/hosts", hostsHandler(cfg.Hosts))

				r.Get("/tools/ip", ipLookupSelfHandler())
				r.Get("/tools/ip/{address}", ipLookupHandler())
			})
		}
	})

	// Public download/paste routes (no auth)
	r.Get("/d/{id}", downloadFileHandler(db, cfg.DataDir))
	r.Get("/d/{id}/info", fileInfoHandler(db))
	r.Get("/p/{id}", viewPasteHandler(db))
	r.Get("/p/{id}/raw", rawPasteHandler(db))

	// WebSocket
	r.Get("/ws/stats", hub.Handler(authService.TokenAuth()))

	// Serve embedded frontend
	if !devMode {
		sub, err := fs.Sub(frontendFS, "frontend/build")
		if err != nil {
			slog.Error("failed to create sub filesystem", "error", err)
		} else {
			r.Handle("/*", spaFileServer(http.FS(sub)))
		}
	}

	return r
}

// spaFileServer serves static files with cache headers, falling back to index.html for SPA routing.
func spaFileServer(root http.FileSystem) http.Handler {
	fileServer := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filePath := r.URL.Path

		f, err := root.Open(filePath)
		if err != nil {
			serveIndexHTML(w, r, root)
			return
		}

		stat, err := f.Stat()
		f.Close()
		if err != nil || stat.IsDir() {
			serveIndexHTML(w, r, root)
			return
		}

		// SvelteKit puts hashed assets in _app/immutable/; cache them forever.
		// Also cache any .js/.css/.woff2 files aggressively.
		ext := path.Ext(filePath)
		if strings.Contains(filePath, "/_app/immutable/") || ext == ".js" || ext == ".css" || ext == ".woff2" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if ext == ".html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}

		fileServer.ServeHTTP(w, r)
	})
}

// serveIndexHTML directly serves the SPA index.html without going through http.FileServer,
// which avoids unwanted 301 redirects for directory paths.
func serveIndexHTML(w http.ResponseWriter, r *http.Request, root http.FileSystem) {
	f, err := root.Open("/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", stat.ModTime(), rs)
}

// --- Helpers ---

func decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	// Enforce JSON content type to prevent cross-origin form-based CSRF
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		http.Error(w, "content-type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func checkPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err == nil {
		return true
	}
	if err != bcrypt.ErrMismatchedHashAndPassword {
		slog.Error("bcrypt error (malformed hash?)", "error", err)
	}
	return false
}

// scanRows iterates rows, calling scanner for each row. Returns the results and any iteration error.
func scanRows[T any](rows *sql.Rows, scanner func(*sql.Rows) (T, error)) ([]T, error) {
	result := make([]T, 0)
	for rows.Next() {
		item, err := scanner(rows)
		if err != nil {
			slog.Error("row scan failed", "error", err)
			continue
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// --- Auth Handlers ---

func loginHandler(authSvc *auth.Auth, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		var storedHash, role string
		err := db.QueryRow(
			"SELECT password_hash, role FROM users WHERE username = ? AND active = 1",
			req.Username,
		).Scan(&storedHash, &role)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		if !checkPassword(req.Password, storedHash) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		token, err := authSvc.CreateToken(req.Username, role)
		if err != nil {
			slog.Error("failed to create token", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		authSvc.SetCookie(w, token)
		writeJSON(w, map[string]string{"username": req.Username, "role": role})
	}
}

func registerHandler(authSvc *auth.Auth, db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.SignupsAllowed() {
			http.Error(w, "signups are disabled", http.StatusForbidden)
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		// Validate
		req.Username = strings.TrimSpace(req.Username)
		if len(req.Username) < 2 || len(req.Username) > 32 {
			http.Error(w, "username must be 2-32 characters", http.StatusBadRequest)
			return
		}
		if len(req.Password) < 8 {
			http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
			return
		}

		// Check if username is taken
		var exists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", req.Username).Scan(&exists)
		if exists {
			http.Error(w, "username already taken", http.StatusConflict)
			return
		}

		// Hash password
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
		if err != nil {
			slog.Error("failed to hash password", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// First user becomes admin
		var userCount int
		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
		role := "user"
		if userCount == 0 {
			role = "admin"
		}

		_, err = db.Exec(
			"INSERT INTO users (username, password_hash, role, active) VALUES (?, ?, ?, 1)",
			req.Username, string(hash), role,
		)
		if err != nil {
			slog.Error("failed to create user", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		slog.Info("user registered", "username", req.Username, "role", role)

		// Auto-login
		token, err := authSvc.CreateToken(req.Username, role)
		if err != nil {
			slog.Error("failed to create token", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		authSvc.SetCookie(w, token)
		writeJSON(w, map[string]string{"username": req.Username, "role": role})
	}
}

func signupsEnabledHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"enabled": cfg.SignupsAllowed()})
	}
}

func logoutHandler(authSvc *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authSvc.ClearCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

func meHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		writeJSON(w, map[string]string{
			"username": auth.GetUsername(claims),
			"role":     auth.GetRole(claims),
		})
	}
}

// --- Webhook Handlers ---

func webhookHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			EventType string `json:"event_type"`
			Source    string `json:"source"`
			Title    string `json:"title"`
			Body     string `json:"body"`
			Metadata string `json:"metadata"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		// Validate required fields and lengths
		if req.EventType == "" || req.Source == "" || req.Title == "" {
			http.Error(w, "event_type, source, and title are required", http.StatusBadRequest)
			return
		}
		if len(req.Title) > 500 || len(req.Body) > 10000 || len(req.Metadata) > 10000 {
			http.Error(w, "field too long", http.StatusBadRequest)
			return
		}

		_, err := db.Exec(
			"INSERT INTO webhook_events (event_type, source, title, body, metadata) VALUES (?, ?, ?, ?, ?)",
			req.EventType, req.Source, req.Title, req.Body, req.Metadata,
		)
		if err != nil {
			slog.Error("failed to insert webhook event", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func webhookFeedHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, event_type, source, title, body, received_at FROM webhook_events ORDER BY received_at DESC LIMIT 50")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		events, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var id int
			var eventType, source, title string
			var body sql.NullString
			var receivedAt string
			err := rows.Scan(&id, &eventType, &source, &title, &body, &receivedAt)
			return map[string]interface{}{
				"id": id, "event_type": eventType, "source": source,
				"title": title, "body": body.String, "received_at": receivedAt,
			}, err
		})
		if err != nil {
			slog.Error("row iteration error", "error", err)
		}
		writeJSON(w, events)
	}
}

// --- Service Handlers ---

func listServicesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT s.id, s.name, s.url,
				(SELECT is_healthy FROM service_checks WHERE service_id = s.id ORDER BY checked_at DESC LIMIT 1) as is_healthy
			FROM monitored_services s WHERE s.enabled = 1 ORDER BY s.name
		`)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type svcRow struct {
			id        int
			name, url string
			isHealthy sql.NullBool
		}
		var svcs []svcRow
		for rows.Next() {
			var s svcRow
			if err := rows.Scan(&s.id, &s.name, &s.url, &s.isHealthy); err != nil {
				slog.Error("scan service", "error", err)
				continue
			}
			svcs = append(svcs, s)
		}
		if err := rows.Err(); err != nil {
			slog.Error("services rows error", "error", err)
		}

		results := make([]map[string]interface{}, 0, len(svcs))
		for _, s := range svcs {
			svc := map[string]interface{}{
				"id":         s.id,
				"name":       s.name,
				"url":        s.url,
				"is_healthy": s.isHealthy.Valid && s.isHealthy.Bool,
			}
			// Compute uptime for 7d, 30d, 365d from daily summaries
			for _, window := range []struct {
				key  string
				days int
			}{
				{"uptime_7d", 7},
				{"uptime_30d", 30},
				{"uptime_365d", 365},
			} {
				var total, healthy sql.NullInt64
				db.QueryRow(
					`SELECT SUM(total_checks), SUM(healthy_checks) FROM service_daily_summaries
					 WHERE service_id = ? AND date >= date('now', ?)`,
					s.id, fmt.Sprintf("-%d days", window.days),
				).Scan(&total, &healthy)
				if total.Valid && total.Int64 > 0 {
					svc[window.key] = float64(healthy.Int64) * 100.0 / float64(total.Int64)
				}
			}
			results = append(results, svc)
		}
		writeJSON(w, results)
	}
}

func serviceHistoryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		rows, err := db.Query(
			"SELECT date, total_checks, healthy_checks, avg_response_time_ms, uptime_percentage FROM service_daily_summaries WHERE service_id = ? ORDER BY date DESC LIMIT 90",
			id,
		)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		history, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var date string
			var total, healthy int
			var avgResponse sql.NullInt64
			var uptime float64
			err := rows.Scan(&date, &total, &healthy, &avgResponse, &uptime)
			return map[string]interface{}{
				"date": date, "total_checks": total, "healthy_checks": healthy,
				"avg_response_time_ms": avgResponse.Int64, "uptime_percentage": uptime,
			}, err
		})
		if err != nil {
			slog.Error("row iteration error", "error", err)
		}
		writeJSON(w, history)
	}
}

// --- Message Handlers ---

func listMessagesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, title, body, category, pinned, author, created_at, updated_at FROM messages ORDER BY pinned DESC, created_at DESC LIMIT 50")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		messages, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var id int
			var title, body, category, author, createdAt, updatedAt string
			var pinned bool
			err := rows.Scan(&id, &title, &body, &category, &pinned, &author, &createdAt, &updatedAt)
			return map[string]interface{}{
				"id": id, "title": title, "body": body, "category": category,
				"pinned": pinned, "author": author, "created_at": createdAt, "updated_at": updatedAt,
			}, err
		})
		if err != nil {
			slog.Error("row iteration error", "error", err)
		}
		writeJSON(w, messages)
	}
}

func createMessageHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		var req struct {
			Title    string `json:"title"`
			Body     string `json:"body"`
			Category string `json:"category"`
			Pinned   bool   `json:"pinned"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Title == "" || req.Body == "" {
			http.Error(w, "title and body are required", http.StatusBadRequest)
			return
		}
		if len(req.Title) > 500 || len(req.Body) > 50000 {
			http.Error(w, "field too long", http.StatusBadRequest)
			return
		}
		if req.Category == "" {
			req.Category = "info"
		}
		_, err := db.Exec(
			"INSERT INTO messages (title, body, category, pinned, author) VALUES (?, ?, ?, ?, ?)",
			req.Title, req.Body, req.Category, req.Pinned, auth.GetUsername(claims),
		)
		if err != nil {
			slog.Error("failed to create message", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func updateMessageHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Title    string `json:"title"`
			Body     string `json:"body"`
			Category string `json:"category"`
			Pinned   bool   `json:"pinned"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if len(req.Title) > 500 || len(req.Body) > 50000 {
			http.Error(w, "field too long", http.StatusBadRequest)
			return
		}
		_, err := db.Exec(
			"UPDATE messages SET title = ?, body = ?, category = ?, pinned = ?, updated_at = datetime('now') WHERE id = ?",
			req.Title, req.Body, req.Category, req.Pinned, id,
		)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteMessageHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := db.Exec("DELETE FROM messages WHERE id = ?", id); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Jellyfin Handler ---

func jellyfinStreamsHandler(cfg *config.JellyfinConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), "GET", cfg.URL+"/Sessions", nil)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("X-Emby-Token", cfg.APIKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			http.Error(w, "jellyfin unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		var sessions []struct {
			NowPlayingItem interface{} `json:"NowPlayingItem"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
			http.Error(w, "failed to parse jellyfin response", http.StatusBadGateway)
			return
		}

		active := 0
		for _, s := range sessions {
			if s.NowPlayingItem != nil {
				active++
			}
		}
		writeJSON(w, map[string]int{"active_streams": active})
	}
}

// --- Hosts Handler ---

func hostsHandler(hosts []config.HostConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type hostStatus struct {
			Name   string        `json:"name"`
			Online bool          `json:"online"`
			Stats  *health.Stats `json:"stats"`
		}

		results := make([]hostStatus, len(hosts))
		var wg sync.WaitGroup

		for i, h := range hosts {
			results[i].Name = h.Name
			wg.Add(1)
			go func(idx int, host config.HostConfig) {
				defer wg.Done()
				req, err := http.NewRequestWithContext(r.Context(), "GET", host.URL+"/api/health", nil)
				if err != nil {
					return
				}
				req.Header.Set("X-API-Key", host.APIKey)
				resp, err := httpClient.Do(req)
				if err != nil {
					return
				}
				defer resp.Body.Close()

				var stats health.Stats
				if err := json.NewDecoder(resp.Body).Decode(&stats); err == nil {
					results[idx].Online = true
					results[idx].Stats = &stats
				}
			}(i, h)
		}
		wg.Wait()
		writeJSON(w, results)
	}
}

// --- IP Lookup Handlers ---

func ipLookupSelfHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		// Extract first IP from X-Forwarded-For if present
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		} else {
			// Strip port from RemoteAddr
			host, _, err := net.SplitHostPort(ip)
			if err == nil {
				ip = host
			}
		}
		doIPLookup(w, r, ip)
	}
}

func ipLookupHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address := chi.URLParam(r, "address")
		// Validate that it's a valid IP address
		if net.ParseIP(address) == nil {
			http.Error(w, "invalid IP address", http.StatusBadRequest)
			return
		}
		doIPLookup(w, r, address)
	}
}

func doIPLookup(w http.ResponseWriter, r *http.Request, ip string) {
	req, err := http.NewRequestWithContext(r.Context(), "GET", "http://ip-api.com/json/"+ip, nil)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, io.LimitReader(resp.Body, 1<<16)) // 64KB limit
}
