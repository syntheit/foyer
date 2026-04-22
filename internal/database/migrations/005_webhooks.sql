CREATE TABLE webhook_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    source TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    metadata TEXT,
    received_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_webhook_events_received_at ON webhook_events(received_at);
