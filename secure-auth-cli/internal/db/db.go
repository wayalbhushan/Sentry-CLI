// Package db manages database initialization, schema migrations, and persistent storage access.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"secure-auth-cli/migrations"

	_ "modernc.org/sqlite"
)

const defaultDBPath = "./data/auth.db"

// Open initializes and opens a connection to the SQLite database.
// If path is empty, it attempts to use DB_PATH env var, falling back to "./data/auth.db".
// Ensures parent directories exist prior to opening the database file.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		path = os.Getenv("DB_PATH")
	}
	if path == "" {
		path = defaultDBPath
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", path, err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Enable foreign key constraints in SQLite
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign key support: %w", err)
	}

	return db, nil
}

// Migrate executes all pending embedded SQL migrations in filename order.
// Maintains idempotency by tracking applied versions in schema_migrations.
func Migrate(db *sql.DB) error {
	// Ensure schema_migrations table exists
	createSchemaTableSQL := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(createSchemaTableSQL); err != nil {
		return fmt.Errorf("failed to initialize schema_migrations table: %w", err)
	}

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	var filenames []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			filenames = append(filenames, entry.Name())
		}
	}
	sort.Strings(filenames)

	for _, filename := range filenames {
		var count int
		err := db.QueryRow("SELECT COUNT(1) FROM schema_migrations WHERE version = ?", filename).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check migration state for %s: %w", filename, err)
		}

		if count > 0 {
			// Migration already applied
			continue
		}

		content, err := migrations.FS.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("failed to read embedded migration file %s: %w", filename, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", filename, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", filename, err)
		}

		recordSQL := "INSERT INTO schema_migrations (version) VALUES (?)"
		if _, err := tx.Exec(recordSQL, filename); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record applied migration %s: %w", filename, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration transaction for %s: %w", filename, err)
		}
	}

	return nil
}
