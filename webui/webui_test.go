package webui

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"testing"

	"github.com/codegangsta/martini"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/stretchr/testify/assert"
)

func setupTest(t *testing.T) (*db.DBHandle, *martini.Martini) {
	feeds := make(map[string]*feed_watcher.FeedWatcher)
	dbh := db.NewMemoryDBHandle(false, true)
	authenticateUser = func(res http.ResponseWriter, req *http.Request, dbh *db.DBHandle) {
	}
	m := createMartini(dbh, feeds)
	return dbh, m
}

func failOnError(t *testing.T, err error) {
	if err != nil {
		fmt.Println(string(debug.Stack()))
		t.Fatalf("Error: %s", err.Error())
	}
}

func loadFixtures(t *testing.T, d *db.DBHandle) {
	users := [][]string{
		[]string{"test1", "test1@example.com", "pass"},
		[]string{"test2", "test2@example.com", "pass"},
		[]string{"test3", "test3@example.com", "pass"},
	}
	feeds := [][]string{
		[]string{"test_feed1", "http://testfeed1/feed.atom"},
		[]string{"test_feed2", "http://testfeed2/feed.atom"},
		[]string{"test_feed3", "http://testfeed3/feed.atom"},
	}
	db_feeds := make([]*db.FeedInfo, len(feeds))
	for i, feed_data := range feeds {
		feed, err := d.AddFeed(feed_data[0], feed_data[1])
		if !assert.Nil(t, err, "Error adding feed to db") {
			t.Fail()
		}
		db_feeds[i] = feed
	}

	db_users := make([]*db.User, len(users))
	for i, user_data := range users {
		u, err := d.AddUser(user_data[0], user_data[1], user_data[2])
		assert.Nil(t, err, "Error adding user to db")
		db_users[i] = u

		err = d.AddFeedsToUser(u, db_feeds)
		assert.Nil(t, err, "Error adding feed to user")
	}
	return
}
