package database

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"trackr/internal/platform/config"
)

type TenantContext struct {
	OrgID   string
	OrgSlug string
	DB      *sql.DB
}

type TenantDBPool struct {
	pools  map[string]*sql.DB
	mu     sync.RWMutex
	config config.TenantDBConfig
}

func NewTenantDBPool(cfg config.TenantDBConfig) *TenantDBPool {
	return &TenantDBPool{
		pools:  make(map[string]*sql.DB),
		config: cfg,
	}
}

func (p *TenantDBPool) Get(orgID string, dbPath string) (*sql.DB, error) {
	p.mu.RLock()
	if db, exists := p.pools[orgID]; exists {
		p.mu.RUnlock()
		return db, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if db, exists := p.pools[orgID]; exists {
		return db, nil
	}

	dsn := fmt.Sprintf("%s?cache=shared&mode=rwc", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(p.config.MaxConnectionsPerOrg)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	p.pools[orgID] = db
	return db, nil
}

func (p *TenantDBPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, db := range p.pools {
		db.Close()
	}
	p.pools = make(map[string]*sql.DB)
}
