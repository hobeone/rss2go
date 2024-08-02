package commands

import (
	"testing"
	"time"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	feedwatcher "github.com/hobeone/rss2go/feed_watcher"
)

func TestConfigUpdater(t *testing.T) {
	cfg := config.NewTestConfig()

	d := NewDaemon(cfg)
	d.Logger = NullLogger()
	d.DBH = db.NewMemoryDBHandle(NullLogger(), true) // Load Fixtures

	f, err := d.DBH.GetFeedByURL("http://localhost/feed1.atom")
	if err != nil {
		t.Fatalf("Error getting feed from db: %v", err)
	}

	d.Feeds["http://test/url"] = feedwatcher.NewFeedWatcher(
		*f, d.CrawlChan, d.MailChan, d.DBH, []string{}, 300, 600,
	)

	d.feedDbUpdate()

	if len(d.Feeds) != 3 {
		t.Errorf("Expected three feed entries after updater runs, got %d", len(d.Feeds))
	}

	if _, ok := d.Feeds[f.URL]; !ok {
		t.Errorf("Expected %s in feed map.", f.URL)
	}
}

func TestFeedSummary(t *testing.T) {
	cfg := config.NewTestConfig()
	d := NewDaemon(cfg)
	d.Logger = NullLogger()
	d.DBH = db.NewMemoryDBHandle(NullLogger(), true) // Load Fixtures
	f, err := d.DBH.GetFeedByURL("http://localhost/feed1.atom")
	if err != nil {
		t.Fatalf("Error getting feed from db: %v", err)
	}

	_ = d.DBH.RecordGUID(1, "foobar")
	_ = d.DBH.RecordGUID(2, "foobaz")
	_ = d.DBH.RecordGUID(3, "foobaz")
	feedItem, err := d.DBH.GetFeedItemByGUID(1, "foobar")
	if err != nil {
		t.Fatalf("Got unexpected error from db: %s", err)
	}

	now := time.Now()
	oneMonthAgo := time.Unix(now.Unix()-(60*60*24*30), 0)

	feedItem.AddedOn = oneMonthAgo
	err = d.DBH.SaveFeedItem(feedItem)
	if err != nil {
		t.Fatalf("Error saving item: %v", err)
	}

	f.LastErrorResponse = "Test Error Resp"
	f.LastPollError = "500 response"
	f.LastPollTime = time.Now().Add(-3000 * time.Hour)
	_ = d.DBH.SaveFeed(f)

	d.feedStateSummary()
}
