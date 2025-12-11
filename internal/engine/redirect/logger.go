package redirect

import (
	"database/sql"
	"log"
	"time"

	"github.com/google/uuid"
	"trackr/internal/engine/links"
)

type ClickLogger struct {
	// In a real system, this might use a channel buffer or a message queue
	// For now, we will just spawn goroutines or use a simple worker pool
}

func NewClickLogger() *ClickLogger {
	return &ClickLogger{}
}

// LogClick is designed to be called in a goroutine
// It takes all necessary data as values to avoid context cancellation issues
func (l *ClickLogger) LogClick(db *sql.DB, linkID, shortCode, destURL string, reqCtx links.RequestContext, utm map[string]string) {
	// Ensure we don't crash the main process
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in LogClick: %v", r)
		}
	}()

	query := `
		INSERT INTO clicks (
			id, link_id, short_code, timestamp, ip_address, user_agent,
			country_code, city, device_type, os, browser, referrer,
			referrer_domain, utm_source, utm_medium, utm_campaign, destination_url
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Simple referrer domain extraction
	// In real implementation, use a proper URL parser
	referrerDomain := ""

	_, err := db.Exec(query,
		uuid.New().String(),
		linkID,
		shortCode,
		time.Now().UnixMilli(),
		reqCtx.IPAddress,
		reqCtx.UserAgent,
		reqCtx.CountryCode,
		"", // City (requires GeoIP DB)
		reqCtx.DeviceType,
		reqCtx.OS,
		reqCtx.Browser,
		reqCtx.Referrer,
		referrerDomain,
		utm["utm_source"],
		utm["utm_medium"],
		utm["utm_campaign"],
		destURL,
	)

	if err != nil {
		log.Printf("Failed to log click: %v", err)
	}

	// Increment click count (fire and forget)
	_, err = db.Exec("UPDATE links SET click_count = click_count + 1, last_click_at = ? WHERE id = ?", time.Now().Unix(), linkID)
	if err != nil {
		log.Printf("Failed to increment click count: %v", err)
	}
}
