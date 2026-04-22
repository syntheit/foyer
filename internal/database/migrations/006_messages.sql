CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'info' CHECK (category IN ('maintenance', 'update', 'info')),
    pinned BOOLEAN NOT NULL DEFAULT 0,
    author TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_messages_pinned ON messages(pinned, created_at);
