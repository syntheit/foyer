package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/dmiller/foyer/internal/config"
)

// SyncUsers ensures the database users match the config. Creates new users,
// updates changed password hashes/roles, and deactivates users not in config.
func SyncUsers(db *sql.DB, users []config.UserConfig) error {
	configUsernames := make(map[string]struct{})

	for _, u := range users {
		configUsernames[u.Username] = struct{}{}

		var existingHash string
		var existingRole string
		err := db.QueryRow(
			"SELECT password_hash, role FROM users WHERE username = ?",
			u.Username,
		).Scan(&existingHash, &existingRole)

		if err == sql.ErrNoRows {
			_, err = db.Exec(
				"INSERT INTO users (username, password_hash, role, active) VALUES (?, ?, ?, 1)",
				u.Username, u.PasswordHash, u.Role,
			)
			if err != nil {
				return fmt.Errorf("create user %s: %w", u.Username, err)
			}
			slog.Info("created user", "username", u.Username, "role", u.Role)
			continue
		}
		if err != nil {
			return fmt.Errorf("check user %s: %w", u.Username, err)
		}

		if existingHash != u.PasswordHash || existingRole != u.Role {
			_, err = db.Exec(
				"UPDATE users SET password_hash = ?, role = ?, active = 1, updated_at = datetime('now') WHERE username = ?",
				u.PasswordHash, u.Role, u.Username,
			)
			if err != nil {
				return fmt.Errorf("update user %s: %w", u.Username, err)
			}
			slog.Info("updated user", "username", u.Username)
		}
	}

	// Deactivate users not in config
	rows, err := db.Query("SELECT username FROM users WHERE active = 1")
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var toDeactivate []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return err
		}
		if _, ok := configUsernames[username]; !ok {
			toDeactivate = append(toDeactivate, username)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate users: %w", err)
	}

	for _, username := range toDeactivate {
		if _, err := db.Exec("UPDATE users SET active = 0, updated_at = datetime('now') WHERE username = ?", username); err != nil {
			return fmt.Errorf("deactivate user %s: %w", username, err)
		}
		slog.Info("deactivated user not in config", "username", username)
	}

	return nil
}

// SyncServices ensures the monitored_services table matches the config.
func SyncServices(db *sql.DB, services []config.ServiceConfig) error {
	configNames := make(map[string]struct{})

	for _, s := range services {
		configNames[s.Name] = struct{}{}

		var existingURL string
		err := db.QueryRow("SELECT url FROM monitored_services WHERE name = ?", s.Name).Scan(&existingURL)

		if err == sql.ErrNoRows {
			_, err = db.Exec(
				"INSERT INTO monitored_services (name, url, enabled) VALUES (?, ?, 1)",
				s.Name, s.URL,
			)
			if err != nil {
				return fmt.Errorf("create service %s: %w", s.Name, err)
			}
			slog.Info("added monitored service", "name", s.Name)
			continue
		}
		if err != nil {
			return fmt.Errorf("check service %s: %w", s.Name, err)
		}

		if existingURL != s.URL {
			if _, err := db.Exec("UPDATE monitored_services SET url = ? WHERE name = ?", s.URL, s.Name); err != nil {
				return fmt.Errorf("update service %s: %w", s.Name, err)
			}
		}
	}

	// Disable services not in config
	rows, err := db.Query("SELECT name FROM monitored_services WHERE enabled = 1")
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if _, ok := configNames[name]; !ok {
			if _, err := db.Exec("UPDATE monitored_services SET enabled = 0 WHERE name = ?", name); err != nil {
				return fmt.Errorf("disable service %s: %w", name, err)
			}
			slog.Info("disabled service not in config", "name", name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate services: %w", err)
	}

	return nil
}
