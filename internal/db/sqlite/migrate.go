package sqlite

import (
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

// Migrations is an embed.FS that holds the migration files.
// This allows the migrations to be bundled into the binary.
// We'll need to pass this from the main or a location that has access to the migrations dir.
func (s *Store) Migrate(migrationsFS embed.FS) error {
	goose.SetBaseFS(migrationsFS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.Up(s.db, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
