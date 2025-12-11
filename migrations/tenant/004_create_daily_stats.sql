-- Pre-aggregated daily statistics for fast dashboard queries
CREATE TABLE IF NOT EXISTS daily_stats (
    id TEXT PRIMARY KEY,
    link_id TEXT NOT NULL,
    date TEXT NOT NULL, -- YYYY-MM-DD
    clicks INTEGER DEFAULT 0,
    unique_ips INTEGER DEFAULT 0,
    top_country TEXT,
    top_referrer TEXT,
    top_device TEXT,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (link_id) REFERENCES links(id) ON DELETE CASCADE,
    UNIQUE(link_id, date)
);

CREATE INDEX IF NOT EXISTS idx_daily_stats_link ON daily_stats(link_id, date DESC);
