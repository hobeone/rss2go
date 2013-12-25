package commands

import (
	"github.com/hobeone/rss2go/config"
	"testing"
)

func TestRemoveUnknownUser(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Db.Type = "memory"
	ru := NewRemoveUserCommand(cfg)
	overrideExit()
	defer assertPanic(t, "RemoveUser should exit on unknown users.")
	ru.RemoveUser("test@example.com")
}

func TestRemoveUser(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Db.Type = "memory"
	ru := NewRemoveUserCommand(cfg)

	_, err := ru.Dbh.AddUser("test", "test@example.com")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}

	overrideExit()
	defer assertNoPanic(t, "RemoveUser should exit on unknown users.")
	ru.RemoveUser("test@example.com")
}
