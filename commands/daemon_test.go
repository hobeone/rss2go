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

	f := *new(db.FeedInfo)

	var feed db.FeedInfo
	feed.Name = "Test Feed"
	feed.Url = "https://testfeed.com/test"
	feed.LastPollTime = time.Now()
	err := d.Dbh.DB.Save(&feed).Error
	if err != nil {
		t.Fatal("Error saving test feed.")
	}

	d.Feeds["http://test/url"] = feed_watcher.NewFeedWatcher(
		f, d.CrawlChan, d.RespChan, d.MailChan, d.Dbh, []string{}, 300, 600,
	)

	d.feedDbUpdate()

	if len(d.Feeds) != 1 {
		t.Errorf("Expected no feed entries after updater runs.")
	}

	if _, ok := d.Feeds[feed.Url]; !ok {
		t.Errorf("Expected %s in feed map.", feed.Url)
	}
}
