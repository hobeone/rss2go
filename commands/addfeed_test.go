package commands

import (
	"testing"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
)

func TestAddFeed(t *testing.T) {
	cfg := config.NewTestConfig()
	af := NewAddFeedCommand(cfg)

	testFeedURL := "http://testfeedurl"

	overrideExit()
	defer assertNoPanic(t, "AddFeed shouldn't have exited but did")
	af.AddFeed("testfeedname", testFeedURL, []string{})

	_, err := af.DBH.GetFeedByURL(testFeedURL)
	if err != nil {
		t.Errorf("Couldn't find feed with URL '%s', AddFeed didn't add it.", testFeedURL)
	}
}

func TestAddFeedWithUsers(t *testing.T) {
	cfg := config.NewTestConfig()
	af := NewAddFeedCommand(cfg)
	db.LoadFixtures(t, af.DBH, "")

	testFeedURL := "http://testfeedurl"
	testUserEmail := "test1@example.com"

	overrideExit()
	defer assertNoPanic(t, "AddFeed shouldn't have exited but did")
	af.AddFeed("testfeedname", testFeedURL, []string{testUserEmail})
	_, err := af.DBH.GetFeedByURL(testFeedURL)
	if err != nil {
		t.Errorf("Couldn't find feed with URL '%s', AddFeed didn't add it.", testFeedURL)
	}

	users, err := af.DBH.GetFeedUsers(testFeedURL)
	for _, user := range users {
		if user.Email == testUserEmail {
			return
		}
	}
	t.Error("AddFeed didn't subscribe given users to feed.")
}
