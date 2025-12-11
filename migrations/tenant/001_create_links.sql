CREATE TABLE IF NOT EXISTS links (
    id TEXT PRIMARY KEY, -- UUID v7
    short_code TEXT UNIQUE NOT NULL, -- 6-8 char alphanumeric
    destination_url TEXT NOT NULL,
    title TEXT,
    created_by TEXT NOT NULL, -- user_id from global DB

    -- Redirect behavior
    redirect_type TEXT DEFAULT 'temporary', -- temporary (302), permanent (301)

    -- Rules engine (JSON)
    rules TEXT, -- {"geo":{"US":"https://us.example.com"},"device":{"ios":"https://apps.apple.com/..."}}

    -- UTM & tracking
    default_utm_params TEXT, -- JSON: {"utm_source":"twitter","utm_campaign":"launch"}

    -- Status & metadata
    status TEXT DEFAULT 'active', -- active, paused, archived
    expires_at INTEGER, -- Unix timestamp for auto-expiry
    password_hash TEXT, -- Optional: password-protected links

    -- Stats cache (denormalized for performance)
    click_count INTEGER DEFAULT 0,
    last_click_at INTEGER,

    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,

    -- Indexes
    UNIQUE(short_code)
);

CREATE INDEX IF NOT EXISTS idx_links_created_by ON links(created_by);
CREATE INDEX IF NOT EXISTS idx_links_status ON links(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_links_clicks ON links(click_count DESC);
