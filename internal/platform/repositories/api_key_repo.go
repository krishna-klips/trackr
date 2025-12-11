package repositories

import (
	"database/sql"
	"encoding/json"
	"time"

	"trackr/internal/platform/models"
	"github.com/google/uuid"
)

type APIKeyRepository struct {
	db *sql.DB
}

func NewAPIKeyRepository(db *sql.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) Create(key *models.APIKey) error {
	if key.ID == "" {
		key.ID = "key_" + uuid.New().String()
	}
	key.CreatedAt = time.Now().Unix()

	scopesJSON, err := json.Marshal(key.Scopes)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO api_keys (id, organization_id, user_id, name, key_hash, key_prefix, scopes, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.Exec(query, key.ID, key.OrganizationID, key.UserID, key.Name, key.KeyHash, key.KeyPrefix, string(scopesJSON), key.CreatedAt, key.ExpiresAt)
	return err
}

func (r *APIKeyRepository) GetByHash(hash string) (*models.APIKey, error) {
	query := `SELECT id, organization_id, user_id, name, key_prefix, scopes, created_at, expires_at, revoked_at FROM api_keys WHERE key_hash = ?`
	row := r.db.QueryRow(query, hash)

	var k models.APIKey
	var scopesStr string
	var expiresAt sql.NullInt64
	var revokedAt sql.NullInt64

	err := row.Scan(&k.ID, &k.OrganizationID, &k.UserID, &k.Name, &k.KeyPrefix, &scopesStr, &k.CreatedAt, &expiresAt, &revokedAt)
	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		k.ExpiresAt = new(int64)
		*k.ExpiresAt = expiresAt.Int64
	}
	if revokedAt.Valid {
		k.RevokedAt = new(int64)
		*k.RevokedAt = revokedAt.Int64
	}

	json.Unmarshal([]byte(scopesStr), &k.Scopes)
	k.KeyHash = hash

	return &k, nil
}

func (r *APIKeyRepository) ListByOrg(orgID string) ([]*models.APIKey, error) {
	query := `SELECT id, user_id, name, key_prefix, scopes, created_at, expires_at, revoked_at FROM api_keys WHERE organization_id = ? ORDER BY created_at DESC`
	rows, err := r.db.Query(query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		var k models.APIKey
		var scopesStr string
		var expiresAt sql.NullInt64
		var revokedAt sql.NullInt64

		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &scopesStr, &k.CreatedAt, &expiresAt, &revokedAt); err != nil {
			return nil, err
		}

		if expiresAt.Valid {
			k.ExpiresAt = new(int64)
			*k.ExpiresAt = expiresAt.Int64
		}
		if revokedAt.Valid {
			k.RevokedAt = new(int64)
			*k.RevokedAt = revokedAt.Int64
		}
		json.Unmarshal([]byte(scopesStr), &k.Scopes)
		k.OrganizationID = orgID
		keys = append(keys, &k)
	}
	return keys, nil
}

func (r *APIKeyRepository) Revoke(id string) error {
	_, err := r.db.Exec(`UPDATE api_keys SET revoked_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

func (r *APIKeyRepository) UpdateLastUsed(id string) error {
	_, err := r.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}
