package models

type Organization struct {
	ID             string `json:"id"`
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	Domain         string `json:"domain"`
	DBFilePath     string `json:"db_file_path"`
	PlanTier       string `json:"plan_tier"`
	LinkQuota      int    `json:"link_quota"`
	MemberQuota    int    `json:"member_quota"`
	SAMLEnabled    bool   `json:"saml_enabled"`
	WebhookSecret  string `json:"webhook_secret"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	DeletedAt      *int64 `json:"deleted_at,omitempty"`
}

type User struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
	EmailVerified  bool   `json:"email_verified"`
	PasswordHash   string `json:"-"`
	FullName       string `json:"full_name"`
	Role           string `json:"role"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	LastLoginAt    *int64 `json:"last_login_at,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	DeletedAt      *int64 `json:"deleted_at,omitempty"`

	Organization   *Organization `json:"organization,omitempty"`
}

type Invite struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Code           string `json:"code"`
	Email          string `json:"email,omitempty"`
	Role           string `json:"role"`
	InvitedBy      string `json:"invited_by"`
	Status         string `json:"status"`
	MaxUses        int    `json:"max_uses"`
	CurrentUses    int    `json:"current_uses"`
	ExpiresAt      int64  `json:"expires_at"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

type SAMLConfig struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	EntityID       string `json:"entity_id"`
	SSOURL         string `json:"sso_url"`
	X509Cert       string `json:"x509_cert"`
	MetadataURL    string `json:"metadata_url,omitempty"`
	NameIDFormat   string `json:"name_id_format"`
	EnforceSSO     bool   `json:"enforce_sso"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}
