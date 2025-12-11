CREATE TABLE IF NOT EXISTS domains (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT NOT NULL,
    domain TEXT UNIQUE NOT NULL, -- acme.com
    verified BOOLEAN DEFAULT FALSE,
    verification_token TEXT, -- DNS TXT record value
    verified_at INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_domains_org ON domains(organization_id);
CREATE INDEX IF NOT EXISTS idx_domains_lookup ON domains(domain) WHERE verified = TRUE;
