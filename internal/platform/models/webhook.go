package models

type Webhook struct {
	ID              string   `json:"id"`
	OrganizationID  string   `json:"organization_id"`
	URL             string   `json:"url"`
	Events          []string `json:"events"` // JSON array in DB
	Secret          string   `json:"secret"`
	Status          string   `json:"status"` // active, paused, failed
	RetryCount      int      `json:"retry_count"`
	LastTriggeredAt int64    `json:"last_triggered_at,omitempty"`
	LastError       string   `json:"last_error,omitempty"`
	CreatedAt       int64    `json:"created_at"`
	UpdatedAt       int64    `json:"updated_at"`
}

type WebhookEvent struct {
	ID        string      `json:"id"`
	Event     string      `json:"event"`
	Timestamp int64       `json:"timestamp"`
	OrgID     string      `json:"org_id"`
	Data      interface{} `json:"data"`
}
