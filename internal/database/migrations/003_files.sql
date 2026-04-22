CREATE TABLE files (
    id TEXT PRIMARY KEY,
    filename TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    storage_path TEXT NOT NULL,
    password_hash TEXT,
    max_downloads INTEGER,
    download_count INTEGER NOT NULL DEFAULT 0,
    uploaded_by TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_files_expires_at ON files(expires_at);
CREATE INDEX idx_files_uploaded_by ON files(uploaded_by);
