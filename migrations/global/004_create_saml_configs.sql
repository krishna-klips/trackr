CREATE TABLE IF NOT EXISTS saml_configs (
    id TEXT PRIMARY KEY, -- UUID v7
    organization_id TEXT UNIQUE NOT NULL,
    entity_id TEXT NOT NULL, -- IdP Entity ID
    sso_url TEXT NOT NULL, -- SAML SSO Endpoint
    x509_cert TEXT NOT NULL, -- Base64 encoded certificate
    metadata_url TEXT, -- Optional: for auto-refresh
    name_id_format TEXT DEFAULT 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
    enforce_sso BOOLEAN DEFAULT FALSE, -- Block password login
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);
