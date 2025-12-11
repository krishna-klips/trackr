CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    email_verified BOOLEAN DEFAULT FALSE,
    password_hash TEXT, -- NULL if SAML-only user
    full_name TEXT NOT NULL,
    role TEXT NOT NULL, -- 'owner', 'admin', 'member'
    avatar_url TEXT,
    last_login_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_users_org ON users(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE deleted_at IS NULL;
