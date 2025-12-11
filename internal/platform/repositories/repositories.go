package repositories

import (
	"database/sql"
	"time"

	"trackr/internal/platform/models"
)

type OrganizationRepository struct {
	db *sql.DB
}

func NewOrganizationRepository(db *sql.DB) *OrganizationRepository {
	return &OrganizationRepository{db: db}
}

func (r *OrganizationRepository) BeginTx() (*sql.Tx, error) {
	return r.db.Begin()
}

func (r *OrganizationRepository) CreateTx(tx *sql.Tx, org *models.Organization) error {
	_, err := tx.Exec(`
		INSERT INTO organizations (id, slug, name, domain, db_file_path, plan_tier, link_quota, member_quota, saml_enabled, webhook_secret, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, org.ID, org.Slug, org.Name, org.Domain, org.DBFilePath, org.PlanTier, org.LinkQuota, org.MemberQuota, org.SAMLEnabled, org.WebhookSecret, org.CreatedAt, org.UpdatedAt)
	return err
}

func (r *OrganizationRepository) Create(org *models.Organization) error {
	_, err := r.db.Exec(`
		INSERT INTO organizations (id, slug, name, domain, db_file_path, plan_tier, link_quota, member_quota, saml_enabled, webhook_secret, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, org.ID, org.Slug, org.Name, org.Domain, org.DBFilePath, org.PlanTier, org.LinkQuota, org.MemberQuota, org.SAMLEnabled, org.WebhookSecret, org.CreatedAt, org.UpdatedAt)
	return err
}

func (r *OrganizationRepository) GetByID(id string) (*models.Organization, error) {
	org := &models.Organization{}
	err := r.db.QueryRow(`
		SELECT id, slug, name, domain, db_file_path, plan_tier, link_quota, member_quota, saml_enabled, webhook_secret, created_at, updated_at, deleted_at
		FROM organizations WHERE id = ?
	`, id).Scan(&org.ID, &org.Slug, &org.Name, &org.Domain, &org.DBFilePath, &org.PlanTier, &org.LinkQuota, &org.MemberQuota, &org.SAMLEnabled, &org.WebhookSecret, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return org, nil
}

func (r *OrganizationRepository) GetByDomain(domain string) (*models.Organization, error) {
	org := &models.Organization{}
	err := r.db.QueryRow(`
		SELECT id, slug, name, domain, db_file_path, plan_tier, link_quota, member_quota, saml_enabled, webhook_secret, created_at, updated_at, deleted_at
		FROM organizations WHERE domain = ?
	`, domain).Scan(&org.ID, &org.Slug, &org.Name, &org.Domain, &org.DBFilePath, &org.PlanTier, &org.LinkQuota, &org.MemberQuota, &org.SAMLEnabled, &org.WebhookSecret, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Return nil, nil if not found
		}
		return nil, err
	}
	return org, nil
}


type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateTx(tx *sql.Tx, user *models.User) error {
	_, err := tx.Exec(`
		INSERT INTO users (id, organization_id, email, email_verified, password_hash, full_name, role, avatar_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, user.ID, user.OrganizationID, user.Email, user.EmailVerified, user.PasswordHash, user.FullName, user.Role, user.AvatarURL, user.CreatedAt, user.UpdatedAt)
	return err
}

func (r *UserRepository) Create(user *models.User) error {
	_, err := r.db.Exec(`
		INSERT INTO users (id, organization_id, email, email_verified, password_hash, full_name, role, avatar_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, user.ID, user.OrganizationID, user.Email, user.EmailVerified, user.PasswordHash, user.FullName, user.Role, user.AvatarURL, user.CreatedAt, user.UpdatedAt)
	return err
}

func (r *UserRepository) GetByID(id string) (*models.User, error) {
	user := &models.User{}
	err := r.db.QueryRow(`
		SELECT id, organization_id, email, email_verified, password_hash, full_name, role, avatar_url, last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.OrganizationID, &user.Email, &user.EmailVerified, &user.PasswordHash, &user.FullName, &user.Role, &user.AvatarURL, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) GetByEmail(email string) (*models.User, error) {
	user := &models.User{}
	err := r.db.QueryRow(`
		SELECT id, organization_id, email, email_verified, password_hash, full_name, role, avatar_url, last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE email = ?
	`, email).Scan(&user.ID, &user.OrganizationID, &user.Email, &user.EmailVerified, &user.PasswordHash, &user.FullName, &user.Role, &user.AvatarURL, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) UpdateLastLogin(userID string, timestamp int64) error {
	_, err := r.db.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, timestamp, userID)
	return err
}

type InviteRepository struct {
	db *sql.DB
}

func NewInviteRepository(db *sql.DB) *InviteRepository {
	return &InviteRepository{db: db}
}

func (r *InviteRepository) GetByCode(code string) (*models.Invite, error) {
	invite := &models.Invite{}
	err := r.db.QueryRow(`
		SELECT id, organization_id, code, email, role, invited_by, status, max_uses, current_uses, expires_at, created_at, updated_at
		FROM invites WHERE code = ?
	`, code).Scan(&invite.ID, &invite.OrganizationID, &invite.Code, &invite.Email, &invite.Role, &invite.InvitedBy, &invite.Status, &invite.MaxUses, &invite.CurrentUses, &invite.ExpiresAt, &invite.CreatedAt, &invite.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return invite, nil
}

func (r *InviteRepository) IncrementUsesTx(tx *sql.Tx, id string) error {
	_, err := tx.Exec(`UPDATE invites SET current_uses = current_uses + 1, updated_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

func (r *InviteRepository) IncrementUses(id string) error {
	_, err := r.db.Exec(`UPDATE invites SET current_uses = current_uses + 1, updated_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

func (r *InviteRepository) Create(invite *models.Invite) error {
    _, err := r.db.Exec(`
        INSERT INTO invites (id, organization_id, code, email, role, invited_by, status, max_uses, current_uses, expires_at, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, invite.ID, invite.OrganizationID, invite.Code, invite.Email, invite.Role, invite.InvitedBy, invite.Status, invite.MaxUses, invite.CurrentUses, invite.ExpiresAt, invite.CreatedAt, invite.UpdatedAt)
    return err
}
