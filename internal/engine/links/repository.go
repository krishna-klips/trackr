package links

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(link *Link) error {
	query := `
		INSERT INTO links (
			id, short_code, destination_url, title, created_by,
			redirect_type, rules, default_utm_params, status,
			expires_at, password_hash, click_count, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	rulesJSON, _ := json.Marshal(link.Rules)
	utmJSON, _ := json.Marshal(link.DefaultUTMParams)

	_, err := r.db.Exec(query,
		link.ID,
		link.ShortCode,
		link.DestinationURL,
		link.Title,
		link.CreatedBy,
		link.RedirectType,
		string(rulesJSON),
		string(utmJSON),
		link.Status,
		link.ExpiresAt,
		link.PasswordHash,
		link.ClickCount,
		link.CreatedAt,
		link.UpdatedAt,
	)

	return err
}

func (r *Repository) GetByID(id string) (*Link, error) {
	query := `
		SELECT id, short_code, destination_url, title, created_by,
		       redirect_type, rules, default_utm_params, status,
		       expires_at, password_hash, click_count, last_click_at, created_at, updated_at
		FROM links WHERE id = ?
	`
	row := r.db.QueryRow(query, id)
	return scanLink(row)
}

func (r *Repository) GetByShortCode(shortCode string) (*Link, error) {
	query := `
		SELECT id, short_code, destination_url, title, created_by,
		       redirect_type, rules, default_utm_params, status,
		       expires_at, password_hash, click_count, last_click_at, created_at, updated_at
		FROM links WHERE short_code = ?
	`
	row := r.db.QueryRow(query, shortCode)
	return scanLink(row)
}

func (r *Repository) ExistsByShortCode(shortCode string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM links WHERE short_code = ?)"
	err := r.db.QueryRow(query, shortCode).Scan(&exists)
	return exists, err
}

func (r *Repository) Update(link *Link) error {
	query := `
		UPDATE links SET
			destination_url = ?, title = ?, redirect_type = ?,
			rules = ?, default_utm_params = ?, status = ?,
			expires_at = ?, password_hash = ?, updated_at = ?
		WHERE id = ?
	`

	rulesJSON, _ := json.Marshal(link.Rules)
	utmJSON, _ := json.Marshal(link.DefaultUTMParams)

	_, err := r.db.Exec(query,
		link.DestinationURL,
		link.Title,
		link.RedirectType,
		string(rulesJSON),
		string(utmJSON),
		link.Status,
		link.ExpiresAt,
		link.PasswordHash,
		time.Now().Unix(),
		link.ID,
	)
	return err
}

func (r *Repository) Delete(id string) error {
	query := "UPDATE links SET status = 'archived' WHERE id = ?"
	_, err := r.db.Exec(query, id)
	return err
}

func (r *Repository) IncrementClickCount(id string) error {
	query := `UPDATE links SET click_count = click_count + 1, last_click_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, time.Now().Unix(), id)
	return err
}

func (r *Repository) List(limit, offset int) ([]*Link, error) {
	query := `
		SELECT id, short_code, destination_url, title, created_by,
		       redirect_type, rules, default_utm_params, status,
		       expires_at, password_hash, click_count, last_click_at, created_at, updated_at
		FROM links
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []*Link
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

func scanLink(s interface {
	Scan(dest ...interface{}) error
}) (*Link, error) {
	var link Link
	var rulesRaw, utmRaw []byte
	var expiresAt, lastClickAt sql.NullInt64

	err := s.Scan(
		&link.ID,
		&link.ShortCode,
		&link.DestinationURL,
		&link.Title,
		&link.CreatedBy,
		&link.RedirectType,
		&rulesRaw,
		&utmRaw,
		&link.Status,
		&expiresAt,
		&link.PasswordHash,
		&link.ClickCount,
		&lastClickAt,
		&link.CreatedAt,
		&link.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		val := expiresAt.Int64
		link.ExpiresAt = &val
	}
	if lastClickAt.Valid {
		val := lastClickAt.Int64
		link.LastClickAt = &val
	}

	if len(rulesRaw) > 0 {
		json.Unmarshal(rulesRaw, &link.Rules)
	}
	if len(utmRaw) > 0 {
		json.Unmarshal(utmRaw, &link.DefaultUTMParams)
	}

	return &link, nil
}
