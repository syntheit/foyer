CREATE TABLE pastes (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    language TEXT NOT NULL DEFAULT 'plaintext',
    burn_after_read BOOLEAN NOT NULL DEFAULT 0,
    burned BOOLEAN NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL,
    expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_pastes_expires_at ON pastes(expires_at);
CREATE INDEX idx_pastes_created_by ON pastes(created_by);
