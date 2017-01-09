package commands

import (
	"io/ioutil"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
)

func NullLogger() logrus.FieldLogger {
	l := logrus.New()
	l.Out = ioutil.Discard
	return l
}

func TestAddFeed(t *testing.T) {
	cfg := config.NewTestConfig()

	fcmd := feedCommand{
		Config: cfg,
		DBH:    db.NewMemoryDBHandle(NullLogger(), true),
	}

	testFeedURL := "http://testfeedurl"

	fcmd.FeedName = "testfeedname"
	fcmd.FeedURL = testFeedURL
	fcmd.UserEmails = []string{}
	err := fcmd.add()
	if err != nil {
		t.Fatalf("Error adding feed: %v", err)
	}

	_, err = fcmd.DBH.GetFeedByURL(testFeedURL)
	if err != nil {
		t.Errorf("Couldn't find feed with URL '%s', AddFeed didn't add it.", testFeedURL)
	}
}

func TestAddFeedWithUsers(t *testing.T) {
	cfg := config.NewTestConfig()

	fcmd := feedCommand{
		Config: cfg,
		DBH:    db.NewMemoryDBHandle(NullLogger(), true),
	}

	testFeedURL := "http://testfeedurl"
	testUserEmail := "test1@example.com"

	fcmd.FeedName = "testfeedname"
	fcmd.FeedURL = testFeedURL
	fcmd.UserEmails = []string{testUserEmail}
	err := fcmd.add()
	if err != nil {
		t.Fatalf("Error adding feed: %v", err)
	}

	_, err = fcmd.DBH.GetFeedByURL(testFeedURL)
	if err != nil {
		t.Errorf("Couldn't find feed with URL '%s', AddFeed didn't add it.", testFeedURL)
	}

	users, err := fcmd.DBH.GetFeedUsers(testFeedURL)
	for _, user := range users {
		if user.Email == testUserEmail {
			return
		}
	}
	t.Error("AddFeed didn't subscribe given users to feed.")
}
