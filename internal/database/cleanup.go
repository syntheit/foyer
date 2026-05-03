package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Cleanup removes expired files, pastes, and old service checks.
func Cleanup(db *sql.DB, dataDir string) error {
	if err := cleanupFiles(db, dataDir); err != nil {
		return fmt.Errorf("cleanup files: %w", err)
	}
	if err := cleanupPastes(db); err != nil {
		return fmt.Errorf("cleanup pastes: %w", err)
	}
	if err := cleanupServiceChecks(db); err != nil {
		return fmt.Errorf("cleanup service checks: %w", err)
	}
	if err := cleanupWebhookEvents(db); err != nil {
		return fmt.Errorf("cleanup webhook events: %w", err)
	}
	if err := cleanupAuditLog(db); err != nil {
		return fmt.Errorf("cleanup audit log: %w", err)
	}
	if err := cleanupVMSamples(db); err != nil {
		return fmt.Errorf("cleanup vm samples: %w", err)
	}
	return nil
}

func cleanupFiles(db *sql.DB, dataDir string) error {
	rows, err := db.Query("SELECT id, storage_path FROM files WHERE expires_at <= datetime('now')")
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id, storagePath string
		if err := rows.Scan(&id, &storagePath); err != nil {
			return err
		}
		fullPath := filepath.Join(dataDir, "files", storagePath)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove expired file", "path", fullPath, "error", err)
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
		if _, err := db.Exec("DELETE FROM files WHERE id = ?", id); err != nil {
			return err
		}
	}

	if len(ids) > 0 {
		slog.Info("cleaned up expired files", "count", len(ids))
	}
	return nil
}

func cleanupPastes(db *sql.DB) error {
	result, err := db.Exec("DELETE FROM pastes WHERE (expires_at IS NOT NULL AND expires_at <= datetime('now')) OR (burned = 1)")
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count > 0 {
		slog.Info("cleaned up expired/burned pastes", "count", count)
	}
	return nil
}

func cleanupServiceChecks(db *sql.DB) error {
	result, err := db.Exec("DELETE FROM service_checks WHERE checked_at < datetime('now', '-30 days')")
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count > 0 {
		slog.Info("cleaned up old service checks", "count", count)
	}
	return nil
}

func cleanupWebhookEvents(db *sql.DB) error {
	result, err := db.Exec("DELETE FROM webhook_events WHERE received_at < datetime('now', '-90 days')")
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count > 0 {
		slog.Info("cleaned up old webhook events", "count", count)
	}
	return nil
}

func cleanupAuditLog(db *sql.DB) error {
	result, err := db.Exec("DELETE FROM audit_log WHERE created_at < datetime('now', '-180 days')")
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count > 0 {
		slog.Info("cleaned up old audit entries", "count", count)
	}
	return nil
}

func cleanupVMSamples(db *sql.DB) error {
	result, err := db.Exec("DELETE FROM vm_metric_samples WHERE sampled_at < datetime('now', '-90 days')")
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count > 0 {
		slog.Info("cleaned up old vm samples", "count", count)
	}
	return nil
}
