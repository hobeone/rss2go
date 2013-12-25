package commands

import (
	"github.com/hobeone/rss2go/config"
	"testing"
)

func TestAddUser(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Mail.SendMail = false
	cfg.Db.Type = "memory"
	au := NewAddUserCommand(cfg)
	au.AddUser("test", "test@example.com", []string{})

	_, err := au.Dbh.GetUser("test")
	if err != nil {
		t.Errorf("Couldn't find user 'test', AddUser didn't add it.")
	}
}

func TestAddDuplicateUser(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Mail.SendMail = false
	cfg.Db.Type = "memory"
	au := NewAddUserCommand(cfg)
	au.AddUser("test", "test@example.com", []string{})

	_, err := au.Dbh.GetUser("test")
	if err != nil {
		t.Errorf("Couldn't find user 'test', AddUser didn't add it.")
	}
	overrideExit()
	defer assertPanic(t, "AddUser should have exited but didn't.")
	au.AddUser("test", "test@example.com", []string{})
}

func TestAddUserWithFeeds(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Mail.SendMail = false
	cfg.Db.Type = "memory"
	au := NewAddUserCommand(cfg)

	_, err := au.Dbh.AddFeed("testfeed", "http://test")
	if err != nil {
		t.Fatalf("Error adding feed: %s", err)
	}

	au.AddUser("test", "test@example.com", []string{"http://test"})
	db_user, err := au.Dbh.GetUser("test")
	if err != nil {
		t.Errorf("Couldn't find user 'test', AddUser didn't add it.")
	}

	fi, err := au.Dbh.GetUsersFeeds(db_user)
	if err != nil {
		t.Fatalf("Error getting feeds for user: %s", err)
	}
	if len(fi) != 1 {
		t.Errorf("Expected to get 1 feed, got %d", len(fi))
	}
}

func TestAddUserWithUnknownFeeds(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Mail.SendMail = false
	cfg.Db.Type = "memory"
	au := NewAddUserCommand(cfg)

	overrideExit()
	defer assertPanic(t, "AddUser should have exited when given unknown feeds")
	au.AddUser("test", "test@example.com", []string{"http://test"})
}

func TestAddUserWithInvalidEmailFormat(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Mail.SendMail = false
	cfg.Db.Type = "memory"
	au := NewAddUserCommand(cfg)

	overrideExit()
	defer assertPanic(t, "AddUser should have exited when given unknown feeds")
	au.AddUser("test", ".test@example.com", []string{"http://test"})
}
