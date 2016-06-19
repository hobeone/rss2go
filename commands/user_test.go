package commands

import (
	"testing"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
)

func TestListUsers(t *testing.T) {
	cfg := config.NewTestConfig()

	ucmd := userCommand{
		Config: cfg,
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
	}

	_, err := ucmd.DBH.AddUser("test", "test@test.com", "pass")
	if err != nil {
		t.Fatalf("Error adding user to db: %s", err)
	}
	err = ucmd.list()
	if err != nil {
		t.Fatalf("Error listing users: %v", err)
	}
}

func TestAddUser(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Name:   "test",
		Email:  "test@example.com",
		Pass:   "pass",
	}

	err := ucmd.add()
	if err != nil {
		t.Fatalf("Error adding user: %v", err)
	}

	_, err = ucmd.DBH.GetUser("test")
	if err != nil {
		t.Errorf("Couldn't find user 'test', AddUser didn't add it.")
	}
}

func TestAddDuplicateUser(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Name:   "test",
		Email:  "test@example.com",
		Pass:   "pass",
	}

	err := ucmd.add()
	if err != nil {
		t.Fatalf("Error adding user: %v", err)
	}

	_, err = ucmd.DBH.GetUser("test")
	if err != nil {
		t.Errorf("Couldn't find user 'test', AddUser didn't add it.")
	}
	err = ucmd.add()
	if err == nil {
		t.Fatal("Expected error when adding duplicate user, got none.")
	}
}

func TestAddUserWithFeeds(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Name:   "test",
		Email:  "test@example.com",
		Pass:   "pass",
		Feeds:  []string{"http://test"},
	}

	_, err := ucmd.DBH.AddFeed("testfeed", "http://test")
	if err != nil {
		t.Fatalf("Error adding feed: %s", err)
	}

	err = ucmd.add()
	if err != nil {
		t.Fatalf("Error adding user: %v", err)
	}

	dbUser, err := ucmd.DBH.GetUser("test")
	if err != nil {
		t.Errorf("Couldn't find user 'test', AddUser didn't add it.")
	}

	fi, err := ucmd.DBH.GetUsersFeeds(dbUser)
	if err != nil {
		t.Fatalf("Error getting feeds for user: %s", err)
	}
	if len(fi) != 1 {
		t.Errorf("Expected to get 1 feed, got %d", len(fi))
	}
}

func TestAddUserWithUnknownFeeds(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Name:   "test",
		Email:  "test@example.com",
		Pass:   "pass",
		Feeds:  []string{"http://test"},
	}

	err := ucmd.add()
	if err == nil {
		t.Fatalf("Add user should have thrown and error when adding an unknown feed")
	}
}

func TestAddUserWithInvalidEmailFormat(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Name:   "test",
		Email:  ".test@example.com",
		Pass:   "",
		Feeds:  []string{"http://test"},
	}

	err := ucmd.add()
	if err == nil {
		t.Fatalf("Add user should have thrown and error when adding an invalid email")
	}
}

func TestRemoveUnknownUser(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Email:  "test@example.com",
	}
	err := ucmd.delete()
	if err == nil {
		t.Fatal("Expected error when deleting unknown user, got none.")
	}
}

func TestRemoveUser(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Email:  "test@example.com",
	}
	_, err := ucmd.DBH.AddUser("test", ucmd.Email, "pass")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}

	err = ucmd.delete()
	if err != nil {
		t.Fatalf("Error deleting user: %s", err)
	}
}

func TestSubscribeUser(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Email:  "test@example.com",
		Feeds:  []string{"https://testing/feed.atom"},
	}

	_, err := ucmd.DBH.AddUser("test", ucmd.Email, "pass")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}
	_, err = ucmd.DBH.AddFeed("testing", ucmd.Feeds[0])
	if err != nil {
		t.Fatalf("Error creating feed: %s", err)
	}

	err = ucmd.subscribe()
	if err != nil {
		t.Fatalf("Error subscribing user to feed: %v", err)
	}
}

func TestSubscribeUserToUnknownFeed(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Email:  "test@example.com",
		Feeds:  []string{"https://testing/feed.atom"},
	}

	_, err := ucmd.DBH.AddUser("test", ucmd.Email, "pass")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}
	err = ucmd.subscribe()
	if err == nil {
		t.Fatal("Expected error when subscribing user to unknwon feed, got none.")
	}
}

func TestUnsubscribeUser(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Email:  "test@example.com",
		Feeds:  []string{"https://testing/feed.atom"},
	}

	_, err := ucmd.DBH.AddUser("test", ucmd.Email, "pass")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}
	_, err = ucmd.DBH.AddFeed("testing", ucmd.Feeds[0])
	if err != nil {
		t.Fatalf("Error creating feed: %s", err)
	}

	err = ucmd.unsubscribe()
	if err != nil {
		t.Fatalf("Error unsubscribing user from feed: %v", err)
	}
}

func TestUnsubscribeUserToUnknownFeed(t *testing.T) {
	ucmd := userCommand{
		Config: config.NewTestConfig(),
		DBH:    db.NewMemoryDBHandle(false, NullLogger(), false),
		Email:  "test@example.com",
		Feeds:  []string{"https://testing/feed.atom"},
	}

	_, err := ucmd.DBH.AddUser("test", ucmd.Email, "pass")
	if err != nil {
		t.Fatalf("Error creating user: %s", err)
	}
	err = ucmd.unsubscribe()
	if err != nil {
		t.Fatalf("Error when unsubscribing user from unknwon feed: %v.", err)
	}
}
