package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/dmiller/foyer/internal/config"
)

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
