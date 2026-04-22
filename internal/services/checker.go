package services

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"
)

type Checker struct {
	db     *sql.DB
	client *http.Client
}

func NewChecker(db *sql.DB) *Checker {
	return &Checker{
		db: db,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Checker) Run(ctx context.Context) {
	// Roll up any past days that haven't been summarized yet
	RollupAll(c.db)

	// Run immediately on start
	c.checkAll()

	checkTicker := time.NewTicker(60 * time.Second)
	defer checkTicker.Stop()

	// Schedule daily rollup at midnight
	rollupTicker := time.NewTicker(time.Hour)
	defer rollupTicker.Stop()
	lastRollupDay := time.Now().Day()

	for {
		select {
		case <-ctx.Done():
			return
		case <-checkTicker.C:
			c.checkAll()
		case now := <-rollupTicker.C:
			// Run rollup once when the day changes
			if now.Day() != lastRollupDay {
				lastRollupDay = now.Day()
				RunRollup(c.db)
			}
		}
	}
}

func (c *Checker) checkAll() {
	rows, err := c.db.Query("SELECT id, name, url FROM monitored_services WHERE enabled = 1")
	if err != nil {
		slog.Error("failed to query services", "error", err)
		return
	}
	defer rows.Close()

	type svc struct {
		id   int
		name string
		url  string
	}
	var services []svc
	for rows.Next() {
		var s svc
		if err := rows.Scan(&s.id, &s.name, &s.url); err != nil {
			slog.Error("failed to scan service", "error", err)
			continue
		}
		services = append(services, s)
	}
	if err := rows.Err(); err != nil {
		slog.Error("service rows iteration error", "error", err)
		return
	}

	for _, s := range services {
		c.check(s.id, s.name, s.url)
	}
}

func (c *Checker) check(id int, name, url string) {
	start := time.Now()
	resp, err := c.client.Get(url)
	elapsed := time.Since(start).Milliseconds()

	var statusCode sql.NullInt64
	var isHealthy bool
	var errMsg sql.NullString

	if err != nil {
		isHealthy = false
		errMsg = sql.NullString{String: err.Error(), Valid: true}
	} else {
		statusCode = sql.NullInt64{Int64: int64(resp.StatusCode), Valid: true}
		isHealthy = resp.StatusCode >= 200 && resp.StatusCode < 400
		resp.Body.Close()
	}

	_, dbErr := c.db.Exec(
		"INSERT INTO service_checks (service_id, status_code, response_time_ms, is_healthy, error) VALUES (?, ?, ?, ?, ?)",
		id, statusCode, elapsed, isHealthy, errMsg,
	)
	if dbErr != nil {
		slog.Error("failed to insert service check", "service", name, "error", dbErr)
	}
}
