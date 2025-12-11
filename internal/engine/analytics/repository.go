package analytics

import (
	"database/sql"
	"fmt"
	"time"
)

type ClickStat struct {
	Timestamp      int64  `json:"timestamp"`
	CountryCode    string `json:"country_code"`
	City           string `json:"city"`
	DeviceType     string `json:"device_type"`
	Browser        string `json:"browser"`
	OS             string `json:"os"`
	ReferrerDomain string `json:"referrer_domain"`
}

type DailyStat struct {
	Date        string `json:"date"`
	Clicks      int    `json:"clicks"`
	UniqueIPs   int    `json:"unique_ips"`
	TopCountry  string `json:"top_country"`
	TopReferrer string `json:"top_referrer"`
	TopDevice   string `json:"top_device"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetClicks(linkID string, start, end int64, limit, offset int) ([]ClickStat, error) {
	query := `
		SELECT timestamp, country_code, city, device_type, browser, os, referrer_domain
		FROM clicks
		WHERE link_id = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.Query(query, linkID, start, end, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clicks []ClickStat
	for rows.Next() {
		var c ClickStat
		if err := rows.Scan(&c.Timestamp, &c.CountryCode, &c.City, &c.DeviceType, &c.Browser, &c.OS, &c.ReferrerDomain); err != nil {
			return nil, err
		}
		clicks = append(clicks, c)
	}
	return clicks, nil
}

func (r *Repository) GetDailyStats(linkID string, startDate, endDate string) ([]DailyStat, error) {
	query := `
		SELECT date, clicks, unique_ips, top_country, top_referrer, top_device
		FROM daily_stats
		WHERE link_id = ? AND date >= ? AND date <= ?
		ORDER BY date DESC
	`
	rows, err := r.db.Query(query, linkID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []DailyStat
	for rows.Next() {
		var s DailyStat
		var topCountry, topReferrer, topDevice sql.NullString
		if err := rows.Scan(&s.Date, &s.Clicks, &s.UniqueIPs, &topCountry, &topReferrer, &topDevice); err != nil {
			return nil, err
		}
		s.TopCountry = topCountry.String
		s.TopReferrer = topReferrer.String
		s.TopDevice = topDevice.String
		stats = append(stats, s)
	}
	return stats, nil
}

// Aggregation queries used by the worker (or on-demand if needed)
func (r *Repository) ComputeDailyStats(linkID, date string) (*DailyStat, error) {
	startTime, _ := time.Parse("2006-01-02", date)
	startTs := startTime.UnixMilli()
	endTs := startTime.Add(24 * time.Hour).UnixMilli()

	stat := &DailyStat{Date: date}

	// Total Clicks
	r.db.QueryRow("SELECT COUNT(*) FROM clicks WHERE link_id = ? AND timestamp >= ? AND timestamp < ?", linkID, startTs, endTs).Scan(&stat.Clicks)

	// Unique IPs
	r.db.QueryRow("SELECT COUNT(DISTINCT ip_address) FROM clicks WHERE link_id = ? AND timestamp >= ? AND timestamp < ?", linkID, startTs, endTs).Scan(&stat.UniqueIPs)

	// Top Country
	r.db.QueryRow(`
		SELECT country_code FROM clicks
		WHERE link_id = ? AND timestamp >= ? AND timestamp < ?
		GROUP BY country_code ORDER BY COUNT(*) DESC LIMIT 1
	`, linkID, startTs, endTs).Scan(&stat.TopCountry)

	return stat, nil
}

func (r *Repository) UpsertDailyStats(stat *DailyStat, linkID string) error {
	// SQLite upsert
	query := `
		INSERT INTO daily_stats (id, link_id, date, clicks, unique_ips, top_country, top_referrer, top_device, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(link_id, date) DO UPDATE SET
			clicks=excluded.clicks,
			unique_ips=excluded.unique_ips,
			top_country=excluded.top_country,
			top_referrer=excluded.top_referrer,
			top_device=excluded.top_device
	`
	// Simple ID gen for now
	id := fmt.Sprintf("%s_%s", linkID, stat.Date)

	_, err := r.db.Exec(query,
		id, linkID, stat.Date, stat.Clicks, stat.UniqueIPs,
		stat.TopCountry, stat.TopReferrer, stat.TopDevice,
		time.Now().Unix(),
	)
	return err
}
