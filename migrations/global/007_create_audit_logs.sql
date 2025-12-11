CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    user_id TEXT,
    action TEXT NOT NULL, -- 'link.created', 'user.invited'
    resource_type TEXT NOT NULL, -- 'link', 'user', 'invite'
    resource_id TEXT,
    metadata TEXT, -- JSON
    ip_address TEXT,
    user_agent TEXT,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_audit_org_time ON audit_logs(organization_id, created_at DESC);
