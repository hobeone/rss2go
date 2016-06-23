package commands

import (
	"testing"
	"time"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
)

func TestConfigUpdater(t *testing.T) {
	cfg := config.NewTestConfig()

	d := NewDaemon(cfg)
	d.Logger = NullLogger()

	f := *new(db.FeedInfo)

	var feed db.FeedInfo
	feed.Name = "Test Feed"
	feed.URL = "https://testfeed.com/test"
	feed.LastPollTime = time.Now()
	err := d.DBH.SaveFeed(&feed)
	if err != nil {
		t.Fatal("Error saving test feed.")
	}

	d.Feeds["http://test/url"] = feedwatcher.NewFeedWatcher(
		f, d.CrawlChan, d.RespChan, d.MailChan, d.DBH, []string{}, 300, 600,
	)

	d.feedDbUpdate()

	if len(d.Feeds) != 1 {
		t.Errorf("Expected no feed entries after updater runs.")
	}

	if _, ok := d.Feeds[feed.URL]; !ok {
		t.Errorf("Expected %s in feed map.", feed.URL)
	}
}
