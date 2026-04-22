package services

import (
	"database/sql"
	"log/slog"
	"time"
)

// RunRollup aggregates yesterday's raw service_checks into service_daily_summaries.
// Safe to call multiple times — uses INSERT OR REPLACE on the unique constraint.
func RunRollup(db *sql.DB) {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	rollupDate(db, yesterday)
}

// RollupAll rolls up all dates that have raw checks but no summary yet.
func RollupAll(db *sql.DB) {
	rows, err := db.Query(`
		SELECT DISTINCT date(checked_at) as d
		FROM service_checks
		WHERE date(checked_at) NOT IN (SELECT date FROM service_daily_summaries)
		AND date(checked_at) < date('now')
		ORDER BY d
	`)
	if err != nil {
		slog.Error("rollup query failed", "error", err)
		return
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			continue
		}
		dates = append(dates, d)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rollup rows error", "error", err)
		return
	}

	for _, d := range dates {
		rollupDate(db, d)
	}
}

func rollupDate(db *sql.DB, date string) {
	rows, err := db.Query("SELECT DISTINCT service_id FROM service_checks WHERE date(checked_at) = ?", date)
	if err != nil {
		slog.Error("rollup service query failed", "error", err, "date", date)
		return
	}
	defer rows.Close()

	var serviceIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		serviceIDs = append(serviceIDs, id)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rollup service rows error", "error", err)
		return
	}

	for _, sid := range serviceIDs {
		var total, healthy int
		var avgResponse sql.NullFloat64

		err := db.QueryRow(`
			SELECT COUNT(*), SUM(CASE WHEN is_healthy THEN 1 ELSE 0 END), AVG(response_time_ms)
			FROM service_checks WHERE service_id = ? AND date(checked_at) = ?
		`, sid, date).Scan(&total, &healthy, &avgResponse)
		if err != nil || total == 0 {
			continue
		}

		uptime := float64(healthy) * 100.0 / float64(total)
		var avgMs sql.NullInt64
		if avgResponse.Valid {
			avgMs = sql.NullInt64{Int64: int64(avgResponse.Float64), Valid: true}
		}

		_, err = db.Exec(`
			INSERT OR REPLACE INTO service_daily_summaries
			(service_id, date, total_checks, healthy_checks, avg_response_time_ms, uptime_percentage)
			VALUES (?, ?, ?, ?, ?, ?)
		`, sid, date, total, healthy, avgMs, uptime)
		if err != nil {
			slog.Error("rollup insert failed", "error", err, "service_id", sid, "date", date)
		}
	}

	slog.Debug("rolled up service checks", "date", date)
}
