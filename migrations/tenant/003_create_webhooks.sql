CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY, -- UUID v7
    url TEXT NOT NULL,
    events TEXT NOT NULL, -- JSON array: ["click.created", "link.created"]
    secret TEXT NOT NULL, -- For HMAC signature
    status TEXT DEFAULT 'active', -- active, paused, failed
    retry_count INTEGER DEFAULT 0,
    last_triggered_at INTEGER,
    last_error TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_webhooks_status ON webhooks(status);
