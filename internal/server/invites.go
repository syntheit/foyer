package server

import (
	"crypto/rand"
	"database/sql"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"

	"github.com/dmiller/foyer/internal/auth"
)

const inviteCodeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const inviteCodeLen = 8

func generateInviteCode() (string, error) {
	b := make([]byte, inviteCodeLen)
	max := big.NewInt(int64(len(inviteCodeChars)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = inviteCodeChars[n.Int64()]
	}
	return string(b), nil
}

// getSetting returns the value of a setting, or "" if it doesn't exist.
func getSetting(db *sql.DB, key string) string {
	var v string
	if err := db.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&v); err != nil {
		return ""
	}
	return v
}

func inviteOnlyEnabled(db *sql.DB) bool {
	return getSetting(db, "invite_only_enabled") == "true"
}

// consumeInviteCode atomically marks a code as used by a given user.
// Returns nil on success, sql.ErrNoRows if no active code matches.
func consumeInviteCode(tx *sql.Tx, code string, userID int64) error {
	res, err := tx.Exec(
		`UPDATE invite_codes
		 SET used_by_id = ?, used_at = datetime('now')
		 WHERE code = ? AND used_at IS NULL
		   AND (expires_at IS NULL OR expires_at > datetime('now'))`,
		userID, code,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- Public validation ---

func validateInviteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Match retrospend's brute-force mitigation: 200-500ms artificial delay.
		jitter, _ := rand.Int(rand.Reader, big.NewInt(300))
		time.Sleep(200*time.Millisecond + time.Duration(jitter.Int64())*time.Millisecond)

		code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
		if len(code) != inviteCodeLen {
			writeJSON(w, map[string]bool{"valid": false})
			return
		}

		var usedAt sql.NullTime
		var expiresAt sql.NullTime
		err := db.QueryRow(
			"SELECT used_at, expires_at FROM invite_codes WHERE code = ?",
			code,
		).Scan(&usedAt, &expiresAt)
		valid := err == nil && !usedAt.Valid &&
			(!expiresAt.Valid || expiresAt.Time.After(time.Now()))

		writeJSON(w, map[string]bool{"valid": valid})
	}
}

// --- Admin handlers ---

func listInvitesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "active"
		}

		var where string
		switch status {
		case "active":
			where = "WHERE i.used_at IS NULL"
		case "used":
			where = "WHERE i.used_at IS NOT NULL"
		case "all":
			where = ""
		default:
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}

		query := `
			SELECT i.id, i.code, i.used_at, i.expires_at, i.created_at,
				cb.username, ub.username
			FROM invite_codes i
			LEFT JOIN users cb ON cb.id = i.created_by_id
			LEFT JOIN users ub ON ub.id = i.used_by_id
			` + where + `
			ORDER BY i.created_at DESC
		`
		rows, err := db.Query(query)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		invites, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var id int
			var code string
			var usedAt, expiresAt sql.NullString
			var createdAt string
			var createdBy, usedBy sql.NullString
			err := rows.Scan(&id, &code, &usedAt, &expiresAt, &createdAt, &createdBy, &usedBy)
			out := map[string]interface{}{
				"id":         id,
				"code":       code,
				"created_at": createdAt,
				"created_by": createdBy.String,
				"used_at":    nil,
				"used_by":    nil,
				"expires_at": nil,
			}
			if usedAt.Valid {
				out["used_at"] = usedAt.String
				out["used_by"] = usedBy.String
			}
			if expiresAt.Valid {
				out["expires_at"] = expiresAt.String
			}
			return out, err
		})
		if err != nil {
			slog.Error("list invites", "error", err)
		}
		writeJSON(w, invites)
	}
}

func generateInviteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)

		var creatorID int64
		if err := db.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&creatorID); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Generate a unique code, retry on collision (same pattern as retrospend).
		var code string
		for attempt := 0; attempt < 10; attempt++ {
			c, err := generateInviteCode()
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			var exists bool
			db.QueryRow("SELECT EXISTS(SELECT 1 FROM invite_codes WHERE code = ?)", c).Scan(&exists)
			if !exists {
				code = c
				break
			}
		}
		if code == "" {
			http.Error(w, "could not generate unique code", http.StatusInternalServerError)
			return
		}

		if _, err := db.Exec(
			"INSERT INTO invite_codes (code, created_by_id) VALUES (?, ?)",
			code, creatorID,
		); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"code": code})
	}
}

func deleteInviteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		res, err := db.Exec("DELETE FROM invite_codes WHERE id = ?", id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Settings handlers ---

func getSettingsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"invite_only_enabled": getSetting(db, "invite_only_enabled") == "true",
		})
	}
}

func updateSettingsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			InviteOnlyEnabled *bool `json:"invite_only_enabled"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.InviteOnlyEnabled != nil {
			val := "false"
			if *req.InviteOnlyEnabled {
				val = "true"
			}
			if _, err := db.Exec(
				`INSERT INTO app_settings (key, value) VALUES ('invite_only_enabled', ?)
				 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')`,
				val,
			); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

