package commands

import (
	"github.com/hobeone/rss2go/config"
	"testing"
)

func TestUnsubscribeUser(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Db.Type = "memory"
	su := NewUnsubscribeUserCommand(cfg)

	user, err := su.Dbh.AddUser("test", "test@test.com", "pass")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}
	feed, err := su.Dbh.AddFeed("testing", "https://testing/feed.atom")
	if err != nil {
		t.Fatalf("Error creating feed: %s", err)
	}

	overrideExit()
	defer assertNoPanic(t, "UnsubscribeUser shouldn't have exited.")
	su.UnsubscribeUser(user.Email, []string{feed.Url})
}

func TestUnsubscribeUserToUnknownFeed(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Db.Type = "memory"
	su := NewUnsubscribeUserCommand(cfg)

	user, err := su.Dbh.AddUser("test", "test@test.com", "pass")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}

	overrideExit()
	defer assertNoPanic(t, "UnsubscribeUser shouldn't have exited.")
	su.UnsubscribeUser(user.Email, []string{""})
}
