package webui

import (
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"runtime/debug"
	"testing"
)

func setupTest(t *testing.T) (*db.DbDispatcher, *martini.Martini) {
	feeds := make(map[string]*feed_watcher.FeedWatcher)
	dbh := db.NewMemoryDbDispatcher(false, true)
	m := createMartini(dbh, feeds)
	return dbh, m
}

func failOnError(t *testing.T, err error) {
	if err != nil {
		fmt.Println(string(debug.Stack()))
		t.Fatalf("Error: %s", err.Error())
	}
}

func loadFixtures(dbh *db.DbDispatcher) {
	users := map[string]string{
		"test1": "test1@example.com",
		"test2": "test2@example.com",
		"test3": "test3@example.com",
	}
	feeds := map[string]string{
		"test_feed1": "http://testfeed1/feed.atom",
		"test_feed2": "http://testfeed2/feed.atom",
		"test_feed3": "http://testfeed3/feed.atom",
	}
	db_feeds := make([]*db.FeedInfo, len(feeds))
	i := 0
	for name, url := range feeds {
		feed, _ := dbh.AddFeed(name, url)
		db_feeds[i] = feed
		i++
	}

	for name, email := range users {
		u, _ := dbh.AddUser(name, email)
		dbh.AddFeedsToUser(u, db_feeds)
	}
}
