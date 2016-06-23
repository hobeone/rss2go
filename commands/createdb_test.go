package commands

import (
	"testing"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
)

func TestCreateDBCommand(t *testing.T) {
	cfg := config.NewTestConfig()
	cdbcmd := createDBCommand{
		Config: cfg,
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), true),
	}

	err := cdbcmd.migrate()
	if err != nil {
		t.Fatalf("Error running createdb command: %v", err)
	}
}
