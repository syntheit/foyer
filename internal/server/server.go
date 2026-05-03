package server

import (
	"database/sql"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strconv"
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
	"github.com/dmiller/foyer/internal/vmcontrol"
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
	vmClient := vmcontrol.NewClient(cfg.VMControllerSocket)

	r.Route("/api", func(r chi.Router) {
		// Auth routes (public)
		r.Post("/auth/login", loginHandler(authService, db))
		r.Post("/auth/register", registerHandler(authService, db, cfg))
		r.Post("/auth/logout", logoutHandler(authService))
		r.Get("/auth/signups", signupsEnabledHandler(cfg, db))
		r.Get("/auth/invites/validate", validateInviteHandler(db))

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
			r.Get("/health/services", servicesHandler(collector))
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
				r.Get("/services/{id}/recent", serviceRecentHandler(db))
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
				r.Put("/pastes/{id}", updatePasteHandler(db))
				r.Delete("/pastes/{id}", deletePasteHandler(db))

				r.Get("/webhooks/feed", webhookFeedHandler(db))

				if cfg.Jellyfin != nil {
					r.Get("/jellyfin/streams", jellyfinStreamsHandler(collector))
				}

				r.Get("/hosts", hostsHandler(cfg.Hosts))

				r.Get("/tools/ip", ipLookupSelfHandler())
				r.Get("/tools/ip/{address}", ipLookupHandler())

				// VM access for assigned users. Each handler re-validates the
				// assignment against vm_assignments before doing anything.
				r.Get("/vms", listMyVMsHandler(db, vmClient))
				r.Get("/vms/{vm}/stats", getMyVMStatsHandler(db, vmClient))
				r.Get("/vms/{vm}/history", getMyVMHistoryHandler(db))
				r.Post("/vms/{vm}/power", vmPowerHandler(db, vmClient))

				r.Group(func(r chi.Router) {
					r.Use(auth.RequireAdmin)
					r.Get("/admin/users", listUsersHandler(db))
					r.Post("/admin/users", createUserHandler(db))
					r.Patch("/admin/users/{id}", updateUserHandler(db))
					r.Delete("/admin/users/{id}", deleteUserHandler(db))

					r.Get("/admin/invites", listInvitesHandler(db))
					r.Post("/admin/invites", generateInviteHandler(db))
					r.Delete("/admin/invites/{id}", deleteInviteHandler(db))

					r.Get("/admin/settings", getSettingsHandler(db))
					r.Patch("/admin/settings", updateSettingsHandler(db))

					r.Get("/admin/vms", listAllVMsHandler(vmClient))
					r.Get("/admin/vm-assignments", listAssignmentsHandler(db))
					r.Post("/admin/vm-assignments", createAssignmentHandler(db, vmClient))
					r.Delete("/admin/vm-assignments/{id}", deleteAssignmentHandler(db))
					r.Post("/admin/vms/{vm}/power", adminVMPowerHandler(db, vmClient))
				})
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
			Username   string `json:"username"`
			Password   string `json:"password"`
			InviteCode string `json:"invite_code"`
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

		// First user becomes admin (and bypasses invite-only — bootstrapping)
		var userCount int
		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
		role := "user"
		if userCount == 0 {
			role = "admin"
		}

		// If invite-only mode is on (and we're past bootstrapping), require a valid code.
		requireInvite := userCount > 0 && inviteOnlyEnabled(db)
		req.InviteCode = strings.ToUpper(strings.TrimSpace(req.InviteCode))
		if requireInvite && req.InviteCode == "" {
			http.Error(w, "invite code required", http.StatusForbidden)
			return
		}

		// Hash password
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
		if err != nil {
			slog.Error("failed to hash password", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Atomically: create user, consume invite code (if applicable). If consume
		// fails, the user is rolled back so a stale/used code can't leak an account.
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		res, err := tx.Exec(
			"INSERT INTO users (username, password_hash, role, active) VALUES (?, ?, ?, 1)",
			req.Username, string(hash), role,
		)
		if err != nil {
			slog.Error("failed to create user", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		newUserID, _ := res.LastInsertId()

		if requireInvite {
			if err := consumeInviteCode(tx, req.InviteCode, newUserID); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "invalid or expired invite code", http.StatusForbidden)
					return
				}
				slog.Error("consume invite", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(); err != nil {
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

func signupsEnabledHandler(cfg *config.Config, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{
			"enabled":             cfg.SignupsAllowed(),
			"invite_only_enabled": inviteOnlyEnabled(db),
		})
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
			// One query per service computes all uptime windows in a single
			// pass over the daily summaries (vs. one round-trip per window).
			var t7, t30, t90, t365, h7, h30, h90, h365 sql.NullInt64
			db.QueryRow(`
				SELECT
					SUM(CASE WHEN date >= date('now','-7 days')   THEN total_checks   END),
					SUM(CASE WHEN date >= date('now','-7 days')   THEN healthy_checks END),
					SUM(CASE WHEN date >= date('now','-30 days')  THEN total_checks   END),
					SUM(CASE WHEN date >= date('now','-30 days')  THEN healthy_checks END),
					SUM(CASE WHEN date >= date('now','-90 days')  THEN total_checks   END),
					SUM(CASE WHEN date >= date('now','-90 days')  THEN healthy_checks END),
					SUM(CASE WHEN date >= date('now','-365 days') THEN total_checks   END),
					SUM(CASE WHEN date >= date('now','-365 days') THEN healthy_checks END)
				FROM service_daily_summaries
				WHERE service_id = ? AND date >= date('now','-365 days')
			`, s.id).Scan(&t7, &h7, &t30, &h30, &t90, &h90, &t365, &h365)
			for _, w := range []struct {
				key             string
				total, healthy  sql.NullInt64
			}{
				{"uptime_7d", t7, h7},
				{"uptime_30d", t30, h30},
				{"uptime_90d", t90, h90},
				{"uptime_365d", t365, h365},
			} {
				if w.total.Valid && w.total.Int64 > 0 {
					svc[w.key] = float64(w.healthy.Int64) * 100.0 / float64(w.total.Int64)
				}
			}
			results = append(results, svc)
		}
		writeJSON(w, results)
	}
}

// serviceRecentHandler returns 48 hourly buckets covering the last 48 hours.
// Each bucket reports total_checks, healthy_checks, and a coarse status string
// ("up" / "degraded" / "down" / "unknown" if no checks ran in that hour).
// Status thresholds: up >= 99%, degraded >= 50%, down < 50%.
func serviceRecentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// SQLite: bucket by hour using strftime, last 48h inclusive.
		rows, err := db.Query(`
			SELECT strftime('%Y-%m-%dT%H:00:00Z', checked_at) as bucket,
				COUNT(*) as total,
				SUM(CASE WHEN is_healthy THEN 1 ELSE 0 END) as healthy
			FROM service_checks
			WHERE service_id = ? AND checked_at >= datetime('now', '-48 hours')
			GROUP BY bucket
		`, id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type bucketRow struct{ total, healthy int }
		buckets := make(map[string]bucketRow)
		for rows.Next() {
			var b string
			var br bucketRow
			if err := rows.Scan(&b, &br.total, &br.healthy); err != nil {
				continue
			}
			buckets[b] = br
		}

		// Build a contiguous 48-hour series ending at the current hour, so the UI
		// can render a fixed-width strip even if some hours have no data.
		now := time.Now().UTC().Truncate(time.Hour)
		out := make([]map[string]interface{}, 0, 48)
		for i := 47; i >= 0; i-- {
			t := now.Add(-time.Duration(i) * time.Hour)
			key := t.Format("2006-01-02T15:00:00Z")
			br, ok := buckets[key]
			status := "unknown"
			var uptime float64
			if ok && br.total > 0 {
				uptime = float64(br.healthy) * 100.0 / float64(br.total)
				switch {
				case uptime >= 99:
					status = "up"
				case uptime >= 50:
					status = "degraded"
				default:
					status = "down"
				}
			}
			out = append(out, map[string]interface{}{
				"hour":    key,
				"total":   br.total,
				"healthy": br.healthy,
				"uptime":  uptime,
				"status":  status,
			})
		}
		writeJSON(w, out)
	}
}

func serviceHistoryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		days := 90
		if d := r.URL.Query().Get("days"); d != "" {
			if n, err := strconv.Atoi(d); err == nil && n > 0 {
				if n > 365 {
					n = 365
				}
				days = n
			}
		}
		rows, err := db.Query(
			"SELECT date, total_checks, healthy_checks, avg_response_time_ms, uptime_percentage FROM service_daily_summaries WHERE service_id = ? ORDER BY date DESC LIMIT ?",
			id, days,
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

// validMessageCategories mirrors the dropdown on the frontend. Anything else
// is silently coerced to "info" rather than persisted, so a stale or hostile
// client can't pollute the table with junk values.
var validMessageCategories = map[string]struct{}{
	"info":        {},
	"update":      {},
	"maintenance": {},
}

func normalizeMessageCategory(c string) string {
	if _, ok := validMessageCategories[c]; ok {
		return c
	}
	return "info"
}

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
		category := normalizeMessageCategory(req.Category)
		_, err := db.Exec(
			"INSERT INTO messages (title, body, category, pinned, author) VALUES (?, ?, ?, ?, ?)",
			req.Title, req.Body, category, req.Pinned, auth.GetUsername(claims),
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
			req.Title, req.Body, normalizeMessageCategory(req.Category), req.Pinned, id,
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

// --- Services Handler (jellyfin streams + minecraft players, etc.) ---
//
// Reads from the Collector's cached snapshot rather than probing on demand —
// the Collector already runs CollectServices on its tick, so multiple polling
// clients share the same upstream calls.

func servicesHandler(collector *health.Collector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, collector.Current().Services)
	}
}

func jellyfinStreamsHandler(collector *health.Collector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		j := collector.Current().Services.Jellyfin
		if j == nil {
			http.Error(w, "jellyfin unavailable", http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]int{"active_streams": j.ActiveStreams})
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
		doIPLookup(w, r, clientIP(r))
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
