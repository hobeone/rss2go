package commands

import (
	"testing"

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
