CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY, -- UUID v7
    slug TEXT UNIQUE NOT NULL, -- trackr-corp
    name TEXT NOT NULL,
    domain TEXT UNIQUE NOT NULL, -- trackr.io (for auto-assignment)
    db_file_path TEXT NOT NULL, -- /data/dbs/org_<uuid>.db
    plan_tier TEXT DEFAULT 'enterprise', -- enterprise, team
    link_quota INTEGER DEFAULT 50000,
    member_quota INTEGER DEFAULT 100,
    saml_enabled BOOLEAN DEFAULT FALSE,
    webhook_secret TEXT, -- For signature verification
    created_at INTEGER NOT NULL, -- Unix timestamp
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER -- Soft delete
);

CREATE INDEX IF NOT EXISTS idx_orgs_domain ON organizations(domain) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_orgs_slug ON organizations(slug) WHERE deleted_at IS NULL;
