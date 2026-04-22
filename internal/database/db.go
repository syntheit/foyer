package database

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(dataDir string) (*sql.DB, error) {
	dbPath := filepath.Join(dataDir, "foyer.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Retry busy operations for up to 5 seconds instead of failing immediately
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	// WAL mode supports concurrent readers with a single writer.
	// Allow multiple read connections but SQLite still serializes writes.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

	return db, nil
}
