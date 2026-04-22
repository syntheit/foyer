CREATE TABLE monitored_services (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE service_checks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id INTEGER NOT NULL REFERENCES monitored_services(id) ON DELETE CASCADE,
    status_code INTEGER,
    response_time_ms INTEGER,
    is_healthy BOOLEAN NOT NULL,
    error TEXT,
    checked_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_checks_service_time ON service_checks(service_id, checked_at);
CREATE INDEX idx_checks_checked_at ON service_checks(checked_at);

CREATE TABLE service_daily_summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id INTEGER NOT NULL REFERENCES monitored_services(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    total_checks INTEGER NOT NULL,
    healthy_checks INTEGER NOT NULL,
    avg_response_time_ms INTEGER,
    uptime_percentage REAL NOT NULL,
    UNIQUE(service_id, date)
);

CREATE INDEX idx_daily_summaries_service_date ON service_daily_summaries(service_id, date);
