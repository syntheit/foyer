CREATE TABLE vm_assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    vm_name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, vm_name)
);

CREATE INDEX idx_vm_assignments_user ON vm_assignments(user_id);
CREATE INDEX idx_vm_assignments_vm ON vm_assignments(vm_name);

CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    actor TEXT NOT NULL, -- username, captured at write time so log survives user deletion
    action TEXT NOT NULL,
    target TEXT NOT NULL,
    ip TEXT,
    success BOOLEAN NOT NULL,
    message TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_audit_log_created ON audit_log(created_at);
CREATE INDEX idx_audit_log_target ON audit_log(target);

CREATE TABLE vm_metric_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    vm_name TEXT NOT NULL,
    sampled_at DATETIME NOT NULL DEFAULT (datetime('now')),
    cpu_percent REAL,
    mem_rss_kib INTEGER,
    mem_max_kib INTEGER,
    disk_alloc_b INTEGER,
    disk_capacity_b INTEGER,
    net_rx_bytes INTEGER,
    net_tx_bytes INTEGER
);

CREATE INDEX idx_vm_samples_vm_time ON vm_metric_samples(vm_name, sampled_at);
