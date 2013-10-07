package feed_watcher

import (
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/mail"
	"io/ioutil"
	"log"
	"testing"
	"time"
)

func SleepForce() {
	Sleep = func(d time.Duration) {
		return
	}
}

type FakeDbDispatcher struct {
	Guids []string
}

func NewFakeDbDispatcher(guids []string) *FakeDbDispatcher {
	return &FakeDbDispatcher{
		Guids: guids,
	}
}

func (self *FakeDbDispatcher) GetFeedByUrl(u string) (d *db.FeedInfo, e error) {
	return
}

func (self *FakeDbDispatcher) GetAllFeeds() ([]db.FeedInfo, error) {
	var feed db.FeedInfo
	feed.Name = "Test Feed"
	feed.Url = "https://testfeed.com/test"
	feed.LastPollTime = *new(time.Time)
	return []db.FeedInfo{feed}, nil
}

func (self *FakeDbDispatcher) UpdateFeedLastItemTimeByUrl(u string, t time.Time) error {
	return nil
}

func (self *FakeDbDispatcher) GetFeedItemByGuid(guid string) (*db.FeedItem, error) {
	return &db.FeedItem{}, nil
}

func (self *FakeDbDispatcher) RecordGuid(feed_id int, guid string) error {
	return nil
}

func (self *FakeDbDispatcher) AddFeed(name string, url string) (*db.FeedInfo, error) {
	return &db.FeedInfo{}, nil
}

func (self *FakeDbDispatcher) RemoveFeed(url string, purge bool) error {
	return nil
}

func (self *FakeDbDispatcher) CheckGuidsForFeed(feed_id int, guids *[]string) (*[]string, error) {
	return &[]string{}, nil
}

func MakeFeedInfo(url string) db.FeedInfo {
	return db.FeedInfo{
		Id:  1,
		Url: url,
	}
}

type NullWriter int

func (NullWriter) Write([]byte) (int, error) { return 0, nil }

func DisableLogging() {
	log.SetOutput(new(NullWriter))
}

func init() {
	DisableLogging()
}

/*
* Tests
 */

func TestNewFeedWatcher(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	db := NewFakeDbDispatcher([]string{})
	u := MakeFeedInfo("http://test")
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, make([]string, 50), 10, 100)
	if n.polling == true {
		t.Error("polling attribute set is true")
	}
}

func TestFeedWatcherPollLocking(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	db := NewFakeDbDispatcher([]string{})
	u := MakeFeedInfo("http://test")
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, make([]string, 50), 10, 100)

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
	db := NewFakeDbDispatcher([]string{})
	u := MakeFeedInfo("http://test/test.rss")
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, make([]string, 50), 10, 100)

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
	db := NewFakeDbDispatcher([]string{})
	u := MakeFeedInfo("http://test/bicycling_rss.xml")
	n := NewFeedWatcher(
		u, crawl_chan, resp_chan, mail_chan, db, make([]string, 50), 10, 100)

	SleepForce()
	go n.PollFeed()
	req := <-crawl_chan
	if req.URI != u.Url {
		t.Errorf("URI not set on request properly.  Expected: %s Got: %s", u.Url, req.URI)
	}

	feed_resp, err := ioutil.ReadFile("../testdata/bicycling_rss.xml")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u.Url,
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
		URI:   u.Url,
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
	db := NewFakeDbDispatcher([]string{})
	u := MakeFeedInfo("http://test/test.rss")
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, make([]string,
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
}

func TestFeedWatcherWithGuidsSet(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	db := NewFakeDbDispatcher([]string{})
	u := MakeFeedInfo("http://test/test.rss")

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
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, guids, 30, 100)
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
