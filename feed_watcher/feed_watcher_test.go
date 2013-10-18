package feed_watcher

import (
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/mail"
	"io/ioutil"
	"testing"
	"time"
)

func SleepForce() {
	Sleep = func(d time.Duration) {
		return
	}
}

func loadTestFixtures(dbh *db.DbDispatcher) []*db.FeedInfo {
	feed, err := dbh.AddFeed("Test Feed", "https://testfeed.com/test")
	if err != nil {
		panic(err.Error())
	}
	return []*db.FeedInfo{feed}
}

/*
* Tests
 */

func TestNewFeedWatcher(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	d := db.NewMemoryDbDispatcher(false, true)
	fixtures := loadTestFixtures(d)
	u := *fixtures[0]

	n := NewFeedWatcher(
		u, crawl_chan, resp_chan, mail_chan, d, make([]string, 50), 10, 100)
	if n.polling == true {
		t.Error("polling attribute set is true")
	}
}

func TestFeedWatcherPollLocking(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	d := db.NewMemoryDbDispatcher(false, true)
	fixtures := loadTestFixtures(d)
	u := *fixtures[0]

	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, d, make([]string, 50), 10, 100)

	if n.Polling() {
		t.Error("A new watcher shouldn't be polling")
	}
	n.lockPoll()
	if !n.Polling() {
		t.Error("Watching didn't set polling lock")
	}
}

func TestFeedWatcherPolling(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	d := db.NewMemoryDbDispatcher(false, true)
	fixtures := loadTestFixtures(d)
	u := *fixtures[0]
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, d, make([]string, 50), 10, 100)

	SleepForce()
	go n.PollFeed()
	req := <-crawl_chan
	if req.URI != u.Url {
		t.Errorf("URI not set on request properly.  Expected: %s Got: %s", u.Url, req.URI)
	}

	feed_resp, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u.Url,
		Body:  feed_resp,
		Error: nil,
	}
	resp := <-resp_chan
	if len(resp.Items) != 25 {
		t.Errorf("Expected 25 items from the feed. Got %d", len(resp.Items))
	}
	// Second Poll, should not have new items
	req = <-crawl_chan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u.Url,
		Body:  feed_resp,
		Error: nil,
	}
	go n.StopPoll()
	resp = <-resp_chan
	if len(resp.Items) != 0 {
		t.Errorf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
	if len(n.KnownGuids) != 26 {
		t.Errorf("Expected 26 known GUIDs got %d", len(n.KnownGuids))
	}
}

func TestFeedWatcherPollingRssWithNoItemDates(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	d := db.NewMemoryDbDispatcher(false, true)
	fixtures := loadTestFixtures(d)
	feed := *fixtures[0]
	n := NewFeedWatcher(
		feed, crawl_chan, resp_chan, mail_chan, d, make([]string, 50), 10, 100)

	SleepForce()
	go n.PollFeed()
	req := <-crawl_chan
	if req.URI != feed.Url {
		t.Errorf("URI not set on request properly.  Expected: %s Got: %s", feed.Url, req.URI)
	}

	feed_resp, err := ioutil.ReadFile("../testdata/bicycling_rss.xml")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   feed.Url,
		Body:  feed_resp,
		Error: nil,
	}
	resp := <-resp_chan
	if len(resp.Items) != 20 {
		t.Errorf("Expected 20 items from the feed. Got %d", len(resp.Items))
	}
	// Second Poll, should not have new items
	req = <-crawl_chan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:   feed.Url,
		Body:  feed_resp,
		Error: nil,
	}
	go n.StopPoll()
	resp = <-resp_chan
	if len(resp.Items) != 0 {
		t.Errorf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
}

func TestFeedWatcherWithMalformedFeed(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	d := db.NewMemoryDbDispatcher(false, true)
	fixtures := loadTestFixtures(d)
	u := *fixtures[0]

	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, d, make([]string,
		50), 10, 100)

	Sleep = func(d time.Duration) {
		return
	}
	go n.PollFeed()
	req := <-crawl_chan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u.Url,
		Body:  []byte("Testing"),
		Error: nil,
	}
	go n.StopPoll()
	response := <-resp_chan
	if response.Error == nil {
		t.Error("Expected error parsing invalid feed.")
	}

	db_feed, err := d.GetFeedByUrl(u.Url)
	if err != nil {
		t.Error(err.Error())
	}
	if db_feed.LastPollError == "" {
		t.Error("Feed should have error set")
	}
}

func TestFeedWatcherWithGuidsSet(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	d := db.NewMemoryDbDispatcher(false, true)
	fixtures := loadTestFixtures(d)
	u := *fixtures[0]

	Sleep = func(d time.Duration) {
		return
	}

	feed_resp, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	_, stories, _ := feed.ParseFeed(u.Url, feed_resp)
	guids := make([]string, 25)
	for _, i := range stories {
		guids = append(guids, i.Id)
	}
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, d, guids, 30, 100)
	go n.PollFeed()
	req := <-crawl_chan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u.Url,
		Body:  feed_resp,
		Error: nil,
	}
	resp := <-resp_chan
	if len(resp.Items) != 0 {
		t.Errorf("Expected 0 items from the feed but got %d.", len(resp.Items))
	}
	// TODO: add a second poll with an updated feed that will return new items.
}

func TestFeedWatcherWithTooRecentLastPoll(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	d := db.NewMemoryDbDispatcher(false, true)
	fixtures := loadTestFixtures(d)
	u := *fixtures[0]
	u.LastPollTime = time.Now()

	feed_resp, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	sleep_calls := 0
	Sleep = func(d time.Duration) {
		sleep_calls++
		return
	}

	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, d, []string{}, 30, 100)
	go n.PollFeed()
	req := <-crawl_chan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u.Url,
		Body:  feed_resp,
		Error: nil,
	}
	_ = <-resp_chan

	if sleep_calls != 2 {
		t.Error("Sleep not called exactly twice.")
	}
}
