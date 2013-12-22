package commands

import (
	"github.com/hobeone/rss2go/config"
	"testing"
)

func TestSubscribeUser(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Db.Type = "memory"
	su := NewSubscribeUserCommand(cfg)

	user, err := su.Dbh.AddUser("test", "test@test.com")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}
	feed, err := su.Dbh.AddFeed("testing", "https://testing/feed.atom")
	if err != nil {
		t.Fatalf("Error creating feed: %s", err)
	}

	overrideExit()
	defer assertNoPanic(t, "SubscribeUser shouldn't have exited.")
	su.SubscribeUser(user.Email, []string{feed.Url})
}

func TestSubscribeUserToUnknownFeed(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Db.Type = "memory"
	su := NewSubscribeUserCommand(cfg)

	user, err := su.Dbh.AddUser("test", "test@test.com")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}

	overrideExit()
	defer assertPanic(t, "SubscribeUser shouldn't have exited.")
	su.SubscribeUser(user.Email, []string{""})
}
