package models

type APIKey struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	UserID         string   `json:"user_id"`
	Name           string   `json:"name"`
	KeyHash        string   `json:"-"`
	KeyPrefix      string   `json:"key_prefix"`
	Scopes         []string `json:"scopes"` // JSON array in DB
	LastUsedAt     *int64   `json:"last_used_at,omitempty"`
	ExpiresAt      *int64   `json:"expires_at,omitempty"`
	CreatedAt      int64    `json:"created_at"`
	RevokedAt      *int64   `json:"revoked_at,omitempty"`
}
