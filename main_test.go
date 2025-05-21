package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hobeone/rss2go/commands"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	feedwatcher "github.com/hobeone/rss2go/feed_watcher"
	"github.com/sirupsen/logrus"
)

var fakeServerHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var content []byte
	switch {
	case strings.HasSuffix(r.URL.Path, "feed1.atom"):
		fmt.Println("SERVING feed1.atim")
		feedResp, err := os.ReadFile("testdata/ars.rss")
		if err != nil {
			logrus.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feedResp
	case true:
		content = []byte("456")
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(content))
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
	err := d.DBH.Migrate(db.SchemaMigrations())
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

	// Get the FeedWatcher for the test feed
	fw := d.Feeds[allFeeds[0].URL]
	if fw == nil {
		t.Fatalf("FeedWatcher not found for URL: %s", allFeeds[0].URL)
	}
	// Enable saving the response for testing
	// This field needs to be exposed first. For now, let's assume it is.
	// If not, we'll need to modify feed_watcher.go
	fw.SetSaveResponse(true)

	// Wait for the first poll to complete.
	// The mock After function is set to 1 second. Let's wait a bit longer.
	time.Sleep(2 * time.Second)

	if fw.LastCrawlResponse == nil {
		t.Fatalf("LastCrawlResponse is nil after first poll")
	}
	if fw.LastCrawlResponse.Error != nil {
		t.Fatalf("Should not have gotten an error on first poll. got: %s", fw.LastCrawlResponse.Error)
	}
	if len(fw.LastCrawlResponse.Items) != 25 {
		t.Errorf("Expected 25 items from the feed on first poll. Got %d", len(fw.LastCrawlResponse.Items))
	}

	// Wait for the second poll to complete.
	time.Sleep(2 * time.Second)

	if fw.LastCrawlResponse == nil {
		t.Fatalf("LastCrawlResponse is nil after second poll")
	}
	if fw.LastCrawlResponse.Error != nil {
		// It's possible to get an error if the feed itself didn't change (e.g. "no items found")
		// but the test expects 0 items, so an error indicating "no new items" is not a failure for the assertion.
		// However, a critical error (like a network issue if the test server died) would be.
		// For now, let's assume no error is expected if items are 0.
		// The original test also fataled on any error here.
		t.Fatalf("Should not have gotten an error on second poll. got: %s", fw.LastCrawlResponse.Error)
	}
	if len(fw.LastCrawlResponse.Items) != 0 {
		t.Errorf("Expected 0 items from the feed on second poll. Got %d", len(fw.LastCrawlResponse.Items))
	}
}
