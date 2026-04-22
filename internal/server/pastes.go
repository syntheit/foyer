package server

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"

	"github.com/dmiller/foyer/internal/auth"
	"github.com/dmiller/foyer/internal/id"
)

func createPasteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)

		var req struct {
			Content       string `json:"content"`
			Language      string `json:"language"`
			ExpiresIn     string `json:"expires_in"` // "1h", "1d", "7d", "30d", "" (never)
			BurnAfterRead bool   `json:"burn_after_read"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.Content == "" {
			http.Error(w, "content is required", http.StatusBadRequest)
			return
		}
		if len(req.Content) > 500000 { // 500KB max paste
			http.Error(w, "paste too large (max 500KB)", http.StatusBadRequest)
			return
		}
		if req.Language == "" {
			req.Language = "plaintext"
		}

		var expiresAt sql.NullString
		switch req.ExpiresIn {
		case "1h":
			expiresAt = sql.NullString{String: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Valid: true}
		case "1d":
			expiresAt = sql.NullString{String: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339), Valid: true}
		case "7d":
			expiresAt = sql.NullString{String: time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339), Valid: true}
		case "30d":
			expiresAt = sql.NullString{String: time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339), Valid: true}
		default:
			// never expires
		}

		pasteID := id.New()

		_, err := db.Exec(
			`INSERT INTO pastes (id, content, language, burn_after_read, created_by, expires_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			pasteID, req.Content, req.Language, req.BurnAfterRead, username, expiresAt,
		)
		if err != nil {
			slog.Error("failed to create paste", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]interface{}{
			"id":  pasteID,
			"url": fmt.Sprintf("/p/%s", pasteID),
		})
	}
}

func listPastesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		rows, err := db.Query(
			`SELECT id, language, burn_after_read, created_at, expires_at
			 FROM pastes WHERE created_by = ? AND burned = 0 ORDER BY created_at DESC`,
			username,
		)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		pastes, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var pasteID, language, createdAt string
			var burnAfterRead bool
			var expiresAt sql.NullString
			err := rows.Scan(&pasteID, &language, &burnAfterRead, &createdAt, &expiresAt)
			m := map[string]interface{}{
				"id": pasteID, "language": language, "burn_after_read": burnAfterRead,
				"created_at": createdAt, "url": fmt.Sprintf("/p/%s", pasteID),
			}
			if expiresAt.Valid {
				m["expires_at"] = expiresAt.String
			}
			return m, err
		})
		if err != nil {
			slog.Error("row iteration error", "error", err)
		}
		writeJSON(w, pastes)
	}
}

func deletePasteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		pasteID := chi.URLParam(r, "id")

		var createdBy string
		err := db.QueryRow("SELECT created_by FROM pastes WHERE id = ?", pasteID).Scan(&createdBy)
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if createdBy != username && !auth.IsAdmin(claims) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		db.Exec("DELETE FROM pastes WHERE id = ?", pasteID)
		w.WriteHeader(http.StatusNoContent)
	}
}

func viewPasteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pasteID := chi.URLParam(r, "id")

		var content, language, createdAt string
		var burnAfterRead, burned bool
		var expiresAt sql.NullString

		err := db.QueryRow(
			`SELECT content, language, burn_after_read, burned, created_at, expires_at
			 FROM pastes WHERE id = ?`, pasteID,
		).Scan(&content, &language, &burnAfterRead, &burned, &createdAt, &expiresAt)

		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if burned {
			http.Error(w, "this paste has been burned", http.StatusGone)
			return
		}

		// Check expiry
		if expiresAt.Valid {
			expiry, _ := time.Parse(time.RFC3339, expiresAt.String)
			if time.Now().After(expiry) {
				http.Error(w, "paste has expired", http.StatusGone)
				return
			}
		}

		// If burn after read, mark as burned
		if burnAfterRead {
			db.Exec("UPDATE pastes SET burned = 1 WHERE id = ?", pasteID)
		}

		result := map[string]interface{}{
			"id": pasteID, "content": content, "language": language,
			"burn_after_read": burnAfterRead, "created_at": createdAt,
		}
		if expiresAt.Valid {
			result["expires_at"] = expiresAt.String
		}
		writeJSON(w, result)
	}
}

func rawPasteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pasteID := chi.URLParam(r, "id")

		var content string
		var burned bool
		var expiresAt sql.NullString

		err := db.QueryRow(
			"SELECT content, burned, expires_at FROM pastes WHERE id = ?", pasteID,
		).Scan(&content, &burned, &expiresAt)

		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if burned {
			http.Error(w, "this paste has been burned", http.StatusGone)
			return
		}
		if expiresAt.Valid {
			expiry, _ := time.Parse(time.RFC3339, expiresAt.String)
			if time.Now().After(expiry) {
				http.Error(w, "paste has expired", http.StatusGone)
				return
			}
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	}
}
