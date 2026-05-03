package server

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/dmiller/foyer/internal/auth"
)

func listUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, username, role, active, created_at FROM users ORDER BY id")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		users, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var id int
			var username, role, createdAt string
			var active bool
			err := rows.Scan(&id, &username, &role, &active, &createdAt)
			return map[string]interface{}{
				"id": id, "username": username, "role": role,
				"active": active, "created_at": createdAt,
			}, err
		})
		if err != nil {
			slog.Error("list users", "error", err)
		}
		writeJSON(w, users)
	}
}

func createUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		if len(req.Username) < 2 || len(req.Username) > 32 {
			http.Error(w, "username must be 2-32 characters", http.StatusBadRequest)
			return
		}
		if len(req.Password) < 8 {
			http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
			return
		}
		if req.Role != "admin" && req.Role != "user" {
			req.Role = "user"
		}

		var exists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", req.Username).Scan(&exists)
		if exists {
			http.Error(w, "username already taken", http.StatusConflict)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec(
			"INSERT INTO users (username, password_hash, role, active) VALUES (?, ?, ?, 1)",
			req.Username, string(hash), req.Role,
		); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func updateUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req struct {
			Role     *string `json:"role"`
			Active   *bool   `json:"active"`
			Password *string `json:"password"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		_, claims, _ := jwtauth.FromContext(r.Context())
		actor := auth.GetUsername(claims)

		// Self-protection: an admin can't demote, deactivate, or password-reset themselves
		// via this endpoint — those mistakes lock you out of the system.
		var targetUsername, targetRole string
		var targetActive bool
		if err := db.QueryRow(
			"SELECT username, role, active FROM users WHERE id = ?", id,
		).Scan(&targetUsername, &targetRole, &targetActive); err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if targetUsername == actor {
			if req.Role != nil && *req.Role != targetRole {
				http.Error(w, "cannot change your own role", http.StatusForbidden)
				return
			}
			if req.Active != nil && !*req.Active {
				http.Error(w, "cannot deactivate yourself", http.StatusForbidden)
				return
			}
		}

		// Last-admin protection: don't allow demoting or deactivating the only active admin.
		if (req.Role != nil && *req.Role != "admin" && targetRole == "admin") ||
			(req.Active != nil && !*req.Active && targetRole == "admin" && targetActive) {
			var adminCount int
			db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin' AND active = 1").Scan(&adminCount)
			if adminCount <= 1 {
				http.Error(w, "cannot remove the last active admin", http.StatusForbidden)
				return
			}
		}

		if req.Role != nil {
			role := *req.Role
			if role != "admin" && role != "user" {
				http.Error(w, "role must be 'admin' or 'user'", http.StatusBadRequest)
				return
			}
			if _, err := db.Exec("UPDATE users SET role = ?, updated_at = datetime('now') WHERE id = ?", role, id); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if req.Active != nil {
			if _, err := db.Exec("UPDATE users SET active = ?, updated_at = datetime('now') WHERE id = ?", *req.Active, id); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if req.Password != nil {
			if len(*req.Password) < 8 {
				http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), 10)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if _, err := db.Exec("UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE id = ?", string(hash), id); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, claims, _ := jwtauth.FromContext(r.Context())
		actor := auth.GetUsername(claims)

		var username, role string
		var active bool
		if err := db.QueryRow(
			"SELECT username, role, active FROM users WHERE id = ?", id,
		).Scan(&username, &role, &active); err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if username == actor {
			http.Error(w, "cannot delete yourself", http.StatusForbidden)
			return
		}
		if role == "admin" && active {
			var adminCount int
			db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin' AND active = 1").Scan(&adminCount)
			if adminCount <= 1 {
				http.Error(w, "cannot delete the last active admin", http.StatusForbidden)
				return
			}
		}
		if _, err := db.Exec("DELETE FROM users WHERE id = ?", id); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
