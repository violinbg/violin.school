package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
)

// Migrate runs all embedded SQL migration files in lexicographic order.
// It tracks applied migrations in a _migrations table.
func Migrate(db *sql.DB, fs embed.FS) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS _migrations (
		name TEXT PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	entries, err := fs.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		if err := db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE name = ?", name).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %q: %w", name, err)
		}
		if exists > 0 {
			continue
		}

		content, err := fs.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", name, err)
		}

		log.Printf("applying migration: %s", name)
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("apply migration %q: %w", name, err)
		}
		if _, err := db.Exec("INSERT INTO _migrations (name) VALUES (?)", name); err != nil {
			return fmt.Errorf("record migration %q: %w", name, err)
		}
	}

	return nil
}
