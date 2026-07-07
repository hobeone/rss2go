package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"rss2go"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Open initializes and configures the SQLite connection pool, enabling WAL mode and foreign keys,
// and runs the schema migration setup.
func Open(dbPath string) (*sql.DB, error) {
	// DSN format for modernc.org/sqlite (standard sqlite driver)
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: failed to open connection: %w", err)
	}

	// Configure connection pool limits
	db.SetMaxOpenConns(1) // Keep max open conns to 1 for SQLite to avoid lock issues on writes
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// Verify the connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database: ping failed: %w", err)
	}

	// Apply Schema Migrations
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database: migration failed: %w", err)
	}

	return db, nil
}

// migrate runs the schema migrations via Goose.
func migrate(db *sql.DB) error {
	goose.SetBaseFS(rss2go.MigrationsFS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("setting dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
