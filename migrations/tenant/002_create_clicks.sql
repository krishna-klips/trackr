CREATE TABLE IF NOT EXISTS clicks (
    id TEXT PRIMARY KEY, -- UUID v7
    link_id TEXT NOT NULL,
    short_code TEXT NOT NULL, -- Denormalized for faster queries

    -- Request metadata
    timestamp INTEGER NOT NULL, -- Unix timestamp in milliseconds
    ip_address TEXT,
    user_agent TEXT,

    -- Parsed attributes
    country_code TEXT, -- ISO 3166-1 alpha-2
    city TEXT,
    device_type TEXT, -- desktop, mobile, tablet, bot
    os TEXT, -- iOS, Android, Windows, macOS, Linux
    browser TEXT, -- Chrome, Safari, Firefox

    -- Referrer analysis
    referrer TEXT,
    referrer_domain TEXT, -- Extracted domain
    utm_source TEXT,
    utm_medium TEXT,
    utm_campaign TEXT,
    utm_term TEXT,
    utm_content TEXT,

    -- Final destination (after rules)
    destination_url TEXT NOT NULL,

    FOREIGN KEY (link_id) REFERENCES links(id) ON DELETE CASCADE
);

-- Performance-critical indexes
CREATE INDEX IF NOT EXISTS idx_clicks_link_time ON clicks(link_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_clicks_time ON clicks(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_clicks_country ON clicks(country_code, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_clicks_device ON clicks(device_type, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_clicks_referrer ON clicks(referrer_domain, timestamp DESC);
