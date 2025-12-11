package links

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type Link struct {
	ID               string           `json:"id"`
	ShortCode        string           `json:"short_code"`
	DestinationURL   string           `json:"destination_url"`
	Title            string           `json:"title"`
	CreatedBy        string           `json:"created_by"`
	RedirectType     string           `json:"redirect_type"`      // temporary (302), permanent (301)
	Rules            *RedirectRules   `json:"rules,omitempty"`    // JSON
	DefaultUTMParams *UTMParams       `json:"default_utm_params,omitempty"` // JSON
	Status           string           `json:"status"`             // active, paused, archived
	ExpiresAt        *int64           `json:"expires_at,omitempty"`
	PasswordHash     string           `json:"password_hash,omitempty"`
	ClickCount       int              `json:"click_count"`
	LastClickAt      *int64           `json:"last_click_at,omitempty"`
	CreatedAt        int64            `json:"created_at"`
	UpdatedAt        int64            `json:"updated_at"`
}

type RedirectRules struct {
	Geo    map[string]string `json:"geo,omitempty"`    // {"US": "https://...", "GB": "..."}
	Device map[string]string `json:"device,omitempty"` // {"ios": "...", "android": "..."}
}

// Value implements the driver.Valuer interface for RedirectRules
func (r RedirectRules) Value() (driver.Value, error) {
	return json.Marshal(r)
}

// Scan implements the sql.Scanner interface for RedirectRules
func (r *RedirectRules) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &r)
}

type UTMParams struct {
	Source   string `json:"utm_source,omitempty"`
	Medium   string `json:"utm_medium,omitempty"`
	Campaign string `json:"utm_campaign,omitempty"`
	Term     string `json:"utm_term,omitempty"`
	Content  string `json:"utm_content,omitempty"`
}

// Value implements the driver.Valuer interface for UTMParams
func (p UTMParams) Value() (driver.Value, error) {
	return json.Marshal(p)
}

// Scan implements the sql.Scanner interface for UTMParams
func (p *UTMParams) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &p)
}
