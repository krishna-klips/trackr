package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"database/sql"

	"trackr/internal/platform/config"
	"trackr/internal/platform/database"
)

func main() {
	target := flag.String("target", "global", "Migration target: global or tenant")
	direction := flag.String("direction", "up", "Migration direction: up or down")
	orgID := flag.String("org", "", "Organization ID (required for tenant migrations)")
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")

	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	switch *target {
	case "global":
		db, err := database.NewGlobalDB(cfg.Database.Global)
		if err != nil {
			log.Fatalf("Failed to connect to global DB: %v", err)
		}
		defer db.Close()
		if err := migrateGlobal(db, *direction); err != nil {
			log.Fatal(err)
		}
	case "tenant":
		if *orgID == "" {
			log.Fatal("--org flag required for tenant migrations")
		}
		// In a real scenario, we'd need to fetch the DB path for the org from the global DB.
		// For now, we assume a convention or passed path.
		// But wait, the migration tool might need to access the global DB to find the tenant DB path.

		// Let's connect to Global DB to get Org details
		globalDB, err := database.NewGlobalDB(cfg.Database.Global)
		if err != nil {
			log.Fatalf("Failed to connect to global DB: %v", err)
		}

		// Simple query to get db_file_path
		var dbFilePath string
		err = globalDB.QueryRow("SELECT db_file_path FROM organizations WHERE id = ?", *orgID).Scan(&dbFilePath)
		globalDB.Close()

		if err != nil {
			log.Fatalf("Failed to get organization DB path: %v", err)
		}

		pool := database.NewTenantDBPool(cfg.Database.Tenant)
		db, err := pool.Get(*orgID, dbFilePath)
		if err != nil {
			log.Fatalf("Failed to connect to tenant DB: %v", err)
		}
		defer db.Close()

		if err := migrateTenant(db, *direction); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("Invalid target: must be 'global' or 'tenant'")
	}

	fmt.Println("Migration completed successfully")
}

func migrateGlobal(db *sql.DB, direction string) error {
	path := "migrations/global"
	return runMigrations(db, path, direction)
}

func migrateTenant(db *sql.DB, direction string) error {
	path := "migrations/tenant"
	return runMigrations(db, path, direction)
}

func runMigrations(db *sql.DB, dir string, direction string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read migration directory: %w", err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".sql" {
			// A very simple migration runner that runs all SQL files.
			// In production, we should track applied migrations in a table.
			content, err := os.ReadFile(filepath.Join(dir, file.Name()))
			if err != nil {
				return fmt.Errorf("failed to read migration file %s: %w", file.Name(), err)
			}

			log.Printf("Applying migration: %s", file.Name())
			if _, err := db.Exec(string(content)); err != nil {
				return fmt.Errorf("failed to execute migration %s: %w", file.Name(), err)
			}
		}
	}
	return nil
}
