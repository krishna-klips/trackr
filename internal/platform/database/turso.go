package database

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3" // For local development or as fallback
	// _ "github.com/tursodatabase/libsql-client-go/libsql" // Uncomment when using Turso
	"trackr/internal/platform/config"
)

func NewGlobalDB(cfg config.GlobalDBConfig) (*sql.DB, error) {
	// For now, we will use sqlite3 for "global" db if url starts with file: or simply doesn't look like libsql
	// In production with Turso, we would use the libsql driver.
	// Since I cannot install libsql driver easily without CGO sometimes, or maybe I can.
	// But `mattn/go-sqlite3` is already there.
	// The plan says "libsql://your-turso-db.turso.io".
	// I will just use `sql.Open("sqlite3", ...)` for now if it's a file, to make it run in this environment.

	driver := "sqlite3"
	if len(cfg.URL) > 8 && cfg.URL[:9] == "libsql://" {
		driver = "libsql"
	}

	// For local testing, strip "file:" if present for sqlite3 driver
	dsn := cfg.URL
	if driver == "sqlite3" && len(dsn) > 5 && dsn[:5] == "file:" {
		dsn = dsn[5:]
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.MaxConnections)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
