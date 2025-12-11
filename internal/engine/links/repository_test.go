package links

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}

	query := `
	CREATE TABLE links (
		id TEXT PRIMARY KEY,
		short_code TEXT UNIQUE NOT NULL,
		destination_url TEXT NOT NULL,
		title TEXT,
		created_by TEXT NOT NULL,
		redirect_type TEXT DEFAULT 'temporary',
		rules TEXT,
		default_utm_params TEXT,
		status TEXT DEFAULT 'active',
		expires_at INTEGER,
		password_hash TEXT,
		click_count INTEGER DEFAULT 0,
		last_click_at INTEGER,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	`
	_, err = db.Exec(query)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}
	return db
}

func TestRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)

	now := time.Now().Unix()
	link := &Link{
		ID:             "link1",
		ShortCode:      "abc",
		DestinationURL: "https://example.com",
		CreatedBy:      "user1",
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	err := repo.Create(link)
	if err != nil {
		t.Errorf("Failed to create link: %v", err)
	}

	fetched, err := repo.GetByID("link1")
	if err != nil {
		t.Errorf("Failed to get link: %v", err)
	}

	if fetched.ShortCode != "abc" {
		t.Errorf("Expected short code abc, got %s", fetched.ShortCode)
	}
}
