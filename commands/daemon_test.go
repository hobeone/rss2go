package commands

import (
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"testing"
	"time"
)

func TestConfigUpdater(t *testing.T) {
	feeds := make(map[string]*feed_watcher.FeedWatcher)

	crawl_chan := make(chan *feed_watcher.FeedCrawlRequest)
	resp_chan := make(chan *feed_watcher.FeedCrawlResponse)
	mail_chan := make(chan *mail.MailRequest)
	cfg := config.NewConfig()

	f := *new(db.FeedInfo)

	dbh := db.NewMemoryDbDispatcher(false, true)
	var feed db.FeedInfo
	feed.Name = "Test Feed"
	feed.Url = "https://testfeed.com/test"
	feed.LastPollTime = time.Now()
	err := dbh.Orm.Save(&feed)
	if err != nil {
		t.Fatal("Error saving test feed.")
	}

	feeds["http://test/url"] = feed_watcher.NewFeedWatcher(
		f,
		crawl_chan,
		resp_chan,
		mail_chan,
		dbh,
		[]string{},
		300,
		600,
	)

	feedDbUpdate(dbh, cfg, crawl_chan, resp_chan, mail_chan, feeds)

	if len(feeds) != 1 {
		t.Errorf("Expected no feed entries after updater runs.")
	}

	if _, ok := feeds[feed.Url]; !ok {
		t.Errorf("Expected %s in feed map.", feed.Url)
	}
}
