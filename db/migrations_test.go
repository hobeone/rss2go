package db

import (
	"testing"
)

func TestMigrations(t *testing.T) {
	logger := NullLogger()
	// NewMemoryDBHandle now runs migrations automatically.
	// If it returns without panic, migrations are successful.
	dbh := NewMemoryDBHandle(logger, true)
	if dbh == nil {
		t.Fatal("NewMemoryDBHandle returned nil")
	}
}
