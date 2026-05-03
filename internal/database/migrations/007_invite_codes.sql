CREATE TABLE invite_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    created_by_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    used_by_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    used_at DATETIME,
    expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_invite_codes_code ON invite_codes(code);
CREATE INDEX idx_invite_codes_used_at ON invite_codes(used_at);

CREATE TABLE app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Default to invite-only mode (matches retrospend's default).
INSERT INTO app_settings (key, value) VALUES ('invite_only_enabled', 'true');
