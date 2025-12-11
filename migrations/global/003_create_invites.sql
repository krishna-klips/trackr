CREATE TABLE IF NOT EXISTS invites (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    code TEXT UNIQUE NOT NULL, -- 16-char alphanumeric
    email TEXT, -- Optional: lock invite to specific email
    role TEXT DEFAULT 'member',
    invited_by TEXT NOT NULL, -- user_id
    status TEXT DEFAULT 'pending', -- pending, accepted, expired, revoked
    max_uses INTEGER DEFAULT 1,
    current_uses INTEGER DEFAULT 0,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_invites_code ON invites(code) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_invites_org ON invites(organization_id);
