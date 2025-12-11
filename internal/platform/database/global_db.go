package database

import "database/sql"

type GlobalDB struct {
	DB *sql.DB
}

func NewGlobalDBWrapper(db *sql.DB) *GlobalDB {
	return &GlobalDB{DB: db}
}
