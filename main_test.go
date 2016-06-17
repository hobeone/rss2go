package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/commands"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
)

var fakeServerHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var content []byte
	switch {
	case strings.HasSuffix(r.URL.Path, "feed1.atom"):
		fmt.Println("SERVING feed1.atim")
		feedResp, err := ioutil.ReadFile("testdata/ars.rss")
		if err != nil {
			logrus.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feedResp
	case true:
		content = []byte("456")
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
})

func TestEndToEndIntegration(t *testing.T) {
	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	// Override the sleep function
	feedwatcher.After = func(d time.Duration) <-chan time.Time {
		logrus.Infof("Call to mock After, waiting for just 1 second.")
		return time.After(time.Second * time.Duration(1))
	}

	cfg := config.NewTestConfig()
	d := commands.NewDaemon(cfg)
	err := d.DBH.Migrate("db/migrations/sqlite3")
	if err != nil {
		t.Fatalf("Error loading fixture data: %v", err)
	}

	u, err := d.DBH.AddUser("user1", "foo@localhost", "foo")
	if err != nil {
		t.Fatalf("Error adding user: %v", err)
	}

	f, err := d.DBH.AddFeed("test feed", fmt.Sprintf("%s/feed1.atom", ts.URL))
	if err != nil {
		t.Fatalf("Error adding feed: %v", err)
	}

	err = d.DBH.AddFeedsToUser(u, []*db.FeedInfo{f})
	if err != nil {
		t.Fatalf("Error adding feed to user: %v", err)
	}

	allFeeds, err := d.DBH.GetAllFeeds()

	if err != nil {
		t.Fatalf("Error reading feeds: %s", err.Error())
	}

	d.CreateAndStartFeedWatchers(allFeeds[0:1])

	resp := <-d.RespChan
	if resp.Error != nil {
		t.Fatalf("Should not have gotten an error. got: %s", resp.Error)
	}
	if len(resp.Items) != 25 {
		t.Errorf("Expected 25 items from the feed. Got %d", len(resp.Items))
	}

	resp = <-d.RespChan
	if resp.Error != nil {
		t.Fatalf("Should not have gotten an error. got: %s", resp.Error)
	}
	if len(resp.Items) != 0 {
		t.Errorf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
}
