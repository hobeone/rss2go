package db

import (
	"testing"

	"github.com/hobeone/gomigrate"
)

func TestMigrations(t *testing.T) {
	logger := NullLogger()
	dbh := NewMemoryDBHandle(logger, true)
	migrator, err := gomigrate.NewMigratorWithMigrations(dbh.db.DB, gomigrate.Sqlite3{}, SchemaMigrations())
	if err != nil {
		t.Fatal(err)
	}

	migrator.Logger = logger
	err = migrator.Migrate()

	if err != nil {
		t.Fatal(err)
	}

	migrator, err = gomigrate.NewMigratorWithMigrations(dbh.db.DB, gomigrate.Sqlite3{}, TestFixtures())
	if err != nil {
		t.Fatal(err)
	}

	migrator.Logger = logger
	err = migrator.Migrate()

	if err != nil {
		t.Fatal(err)
	}
}
