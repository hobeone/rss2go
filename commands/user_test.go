package commands

import (
	"testing"

	"github.com/alecthomas/kingpin/v2" // Added kingpin import
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
)

func TestListUsers(t *testing.T) {
	cfg := config.NewTestConfig()

	ucmd := userCommand{
		Config: cfg,
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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
		DBH:    db.NewMemoryDBHandle(NullLogger(), false),
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

// TestUserUnsubscribeCommandWiring verifies that the 'users unsubscribe' command
// is correctly wired and parses arguments.
func TestUserUnsubscribeCommandWiring(t *testing.T) {
	if err != nil {
		t.Fatalf("Error when unsubscribing user from unknwon feed: %v.", err)
	}
}

// TestUserUnsubscribeCommandWiring verifies that the 'users unsubscribe' command
// is correctly wired and parses arguments.
func TestUserUnsubscribeCommandWiring(t *testing.T) {
	app := kingpin.New("testapp_unsubscribe_wiring", "Test App Unsubscribe Wiring")
	// Prevent kingpin from exiting the test on error or help flags by providing a NOP terminate.
	nopTerminate := func(int) {}
	app.Terminate(nopTerminate)

	// Create a userCommand instance that kingpin will populate.
	uc := &userCommand{}
	// Configure the "users" command and its subcommands.
	uc.configure(app)

	// Simulate command-line arguments for "users unsubscribe"
	args := []string{
		"users",
		"unsubscribe",
		"test@example.com", // Positional argument for email
		"http://feed1.com",
		"http://feed2.com",
	}

	// Parse the arguments. Kingpin will attempt to execute the Action.
	// The Action (unsubscribeCmd) calls init(), which calls commonInit(), which calls loadConfig().
	// loadConfig("") tries to read current dir, causing "is a directory" error.
	// We expect this specific error if wiring is correct up to action execution.
	cmd, err := app.Parse(args)

	// Check which command was parsed
	if !strings.Contains(cmd, "users unsubscribe") {
		t.Fatalf("Expected 'users unsubscribe' command to be parsed, got '%s'", cmd)
	}

	// Check if the error is the expected config loading error.
	// If err is nil, it means commonInit was somehow perfectly mocked or didn't run,
	// which would be unexpected given current constraints.
	// If err is different, it might indicate a kingpin parsing issue before our action.
	if err == nil {
		// This path would ideally be taken if commonInit was successfully mocked to prevent config load.
		// Given current constraints, we expect an error.
		// t.Logf("Command parsed and action likely run without config load error (unexpected in current setup).")
	} else {
		// This is the expected path due to config load issues.
		// We're checking that the error isn't a kingpin "unknown command" type error.
		// A more specific error check could be "is a directory" or "failed reading config".
		// For now, any error here (assuming it's not "unknown command") means kingpin found the command.
		if strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("Kingpin reported an 'unknown command', wiring might be incorrect: %v", err)
		}
		// t.Logf("Got an error as expected (due to config load): %v", err)
	}

	// Verify that the fields in 'uc' were populated correctly by kingpin,
	// this shows argument parsing for the command worked.
	expectedEmail := "test@example.com"
	if uc.Email != expectedEmail {
		t.Errorf("Expected Email to be '%s', got '%s'", expectedEmail, uc.Email)
	}

	if len(uc.Feeds) != 2 {
		t.Fatalf("Expected 2 feed URLs, got %d", len(uc.Feeds))
	}
	expectedFeeds := []string{"http://feed1.com", "http://feed2.com"}
	for i, feedURL := range expectedFeeds {
		if uc.Feeds[i] != feedURL {
			t.Errorf("Expected Feeds[%d] to be '%s', got '%s'", i, feedURL, uc.Feeds[i])
		}
	}
}
