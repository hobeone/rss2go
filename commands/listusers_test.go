package commands

import (
	"github.com/hobeone/rss2go/config"
	"testing"
)

func TestListUsers(t *testing.T) {
	cfg := config.NewTestConfig()
	cfg.Mail.SendMail = false
	cfg.Db.Type = "memory"

	lu := NewListUsersCommand(cfg)
	_, err := lu.Dbh.AddUser("test", "test@test.com", "pass")
	if err != nil {
		t.Fatalf("Error adding user to db: %s", err)
	}
	overrideExit()

	defer assertNoPanic(t, "ListUsers exited when it shouldn't have.")
	lu.ListUsers()
}
