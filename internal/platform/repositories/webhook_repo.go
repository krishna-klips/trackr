package repositories

import (
	"database/sql"
	"encoding/json"
	"time"

	"trackr/internal/platform/models"
	"github.com/google/uuid"
)

type WebhookRepository struct {
	db *sql.DB
}

func NewWebhookRepository(db *sql.DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

func (r *WebhookRepository) Create(webhook *models.Webhook) error {
	webhook.ID = "wh_" + uuid.New().String()
	webhook.CreatedAt = time.Now().Unix()
	webhook.UpdatedAt = time.Now().Unix()
	webhook.Status = "active"

	eventsJSON, err := json.Marshal(webhook.Events)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO webhooks (id, url, events, secret, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.Exec(query, webhook.ID, webhook.URL, string(eventsJSON), webhook.Secret, webhook.Status, webhook.CreatedAt, webhook.UpdatedAt)
	return err
}

func (r *WebhookRepository) GetByID(id string) (*models.Webhook, error) {
	query := `SELECT id, url, events, secret, status, retry_count, last_triggered_at, last_error, created_at, updated_at FROM webhooks WHERE id = ?`
	row := r.db.QueryRow(query, id)

	var w models.Webhook
	var eventsStr string
	var lastTriggeredAt sql.NullInt64
	var lastError sql.NullString

	err := row.Scan(&w.ID, &w.URL, &eventsStr, &w.Secret, &w.Status, &w.RetryCount, &lastTriggeredAt, &lastError, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if lastTriggeredAt.Valid {
		w.LastTriggeredAt = lastTriggeredAt.Int64
	}
	if lastError.Valid {
		w.LastError = lastError.String
	}

	json.Unmarshal([]byte(eventsStr), &w.Events)

	return &w, nil
}

func (r *WebhookRepository) List() ([]*models.Webhook, error) {
	query := `SELECT id, url, events, secret, status, retry_count, last_triggered_at, last_error, created_at, updated_at FROM webhooks ORDER BY created_at DESC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []*models.Webhook
	for rows.Next() {
		var w models.Webhook
		var eventsStr string
		var lastTriggeredAt sql.NullInt64
		var lastError sql.NullString

		if err := rows.Scan(&w.ID, &w.URL, &eventsStr, &w.Secret, &w.Status, &w.RetryCount, &lastTriggeredAt, &lastError, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}

		if lastTriggeredAt.Valid {
			w.LastTriggeredAt = lastTriggeredAt.Int64
		}
		if lastError.Valid {
			w.LastError = lastError.String
		}
		json.Unmarshal([]byte(eventsStr), &w.Events)
		webhooks = append(webhooks, &w)
	}
	return webhooks, nil
}

func (r *WebhookRepository) Update(webhook *models.Webhook) error {
	eventsJSON, err := json.Marshal(webhook.Events)
	if err != nil {
		return err
	}
	webhook.UpdatedAt = time.Now().Unix()

	query := `
		UPDATE webhooks
		SET url = ?, events = ?, secret = ?, status = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = r.db.Exec(query, webhook.URL, string(eventsJSON), webhook.Secret, webhook.Status, webhook.UpdatedAt, webhook.ID)
	return err
}

func (r *WebhookRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM webhooks WHERE id = ?`, id)
	return err
}

func (r *WebhookRepository) UpdateStatus(id, status string) error {
	_, err := r.db.Exec(`UPDATE webhooks SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().Unix(), id)
	return err
}

func (r *WebhookRepository) UpdateLastTriggered(id string, timestamp int64) error {
	_, err := r.db.Exec(`UPDATE webhooks SET last_triggered_at = ? WHERE id = ?`, timestamp, id)
	return err
}

func (r *WebhookRepository) IncrementRetryCount(id string) error {
	_, err := r.db.Exec(`UPDATE webhooks SET retry_count = retry_count + 1 WHERE id = ?`, id)
	return err
}

func (r *WebhookRepository) ResetRetryCount(id string) error {
	_, err := r.db.Exec(`UPDATE webhooks SET retry_count = 0 WHERE id = ?`, id)
	return err
}

func (r *WebhookRepository) UpdateLastError(id, lastError string) error {
	_, err := r.db.Exec(`UPDATE webhooks SET last_error = ? WHERE id = ?`, lastError, id)
	return err
}

func (r *WebhookRepository) GetByEvent(eventType string) ([]*models.Webhook, error) {
	// This is a simplified search. Ideally we'd use JSON operators if supported by SQLite or handle it in app.
	// Since we store events as ["event1", "event2"], we can do a LIKE query or fetch all active and filter.
	// For performance with many webhooks, this should be optimized.
	// Assuming low volume of webhooks per tenant for now.

	query := `SELECT id, url, events, secret, status FROM webhooks WHERE status = 'active'`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matched []*models.Webhook
	for rows.Next() {
		var w models.Webhook
		var eventsStr string
		if err := rows.Scan(&w.ID, &w.URL, &eventsStr, &w.Secret, &w.Status); err != nil {
			continue
		}

		var events []string
		if err := json.Unmarshal([]byte(eventsStr), &events); err == nil {
			for _, e := range events {
				if e == eventType {
					w.Events = events
					matched = append(matched, &w)
					break
				}
			}
		}
	}
	return matched, nil
}

func (r *WebhookRepository) GetFailed(since int64) ([]*models.Webhook, error) {
	query := `SELECT id, url, events, secret, status, retry_count, last_triggered_at FROM webhooks WHERE status = 'active' AND last_triggered_at < ? AND retry_count > 0`
	rows, err := r.db.Query(query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []*models.Webhook
	for rows.Next() {
		var w models.Webhook
		var eventsStr string
		var lastTriggeredAt sql.NullInt64
		if err := rows.Scan(&w.ID, &w.URL, &eventsStr, &w.Secret, &w.Status, &w.RetryCount, &lastTriggeredAt); err != nil {
			return nil, err
		}
		if lastTriggeredAt.Valid {
			w.LastTriggeredAt = lastTriggeredAt.Int64
		}
		json.Unmarshal([]byte(eventsStr), &w.Events)
		webhooks = append(webhooks, &w)
	}
	return webhooks, nil
}
