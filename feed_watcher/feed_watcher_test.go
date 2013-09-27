package feed_watcher

import (
	"fmt"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/mail"
	"io/ioutil"
	"testing"
	"time"
)

func SleepForce() {
	Sleep = func(d time.Duration) {
		fmt.Println("mock sleep")
		return
	}
}

type FakeDbDispatcher struct {
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

func TestNewFeedWatcher(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	db := new(FakeDbDispatcher)
	u := "http://test"
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, *new(time.Time))
	if n.URI != u {
		t.Error("URI not set correctly: %v != %v ", u, n.URI)
	}
	if n.polling == true {
		t.Error("polling attribute set is true")
	}
}

func TestFeedWatcherPollLocking(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	db := new(FakeDbDispatcher)
	u := "http://test"
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, *new(time.Time))

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
	db := new(FakeDbDispatcher)
	u := "http://test/test.rss"
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, *new(time.Time))

	SleepForce()
	go n.PollFeed()
	req := <-crawl_chan
	if req.URI != u {
		t.Errorf("URI not set on request properly.  Expected: %+v Got: %+v", u, req.URI)
	}

	feed_resp, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u,
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
		URI:   u,
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
	db := new(FakeDbDispatcher)
	u := "http://test/test.rss"
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, *new(time.Time))

	Sleep = func(d time.Duration) {
		fmt.Println("Called mock sleep")
		expected := time.Minute * time.Duration(1)
		if d != expected {
			t.Fatalf("Expected to sleep for %+v. Got %+v", expected, d)
		}
		return
	}
	go n.PollFeed()
	req := <-crawl_chan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u,
		Body:  []byte("Testing"),
		Error: nil,
	}
	go n.StopPoll()
	response := <-resp_chan
	if response.Error == nil {
		t.Error("Expected error parsing invalid feed.")
	}
}

func TestFeedWatcherWithLastDateSet(t *testing.T) {
	crawl_chan := make(chan *FeedCrawlRequest)
	resp_chan := make(chan *FeedCrawlResponse)
	mail_chan := mail.CreateAndStartStubMailer().OutgoingMail
	db := new(FakeDbDispatcher)
	u := "http://test/test.rss"
	last_date := time.Date(2013, time.December, 1, 1, 0, 0, 0, time.UTC)
	n := NewFeedWatcher(u, crawl_chan, resp_chan, mail_chan, db, last_date)

	Sleep = func(d time.Duration) {
		expected := time.Minute * time.Duration(30)
		if d != expected {
			t.Fatalf("Expected to sleep for %+v. Got %+v", expected, d)
		}
		return
	}
	go n.PollFeed()
	req := <-crawl_chan
	if req.URI != u {
		t.Errorf("URI not set on request properly.  Expected: %+v Got: %+v", u, req.URI)
	}

	feed_resp, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   u,
		Body:  feed_resp,
		Error: nil,
	}
	resp := <-resp_chan
	if len(resp.Items) != 0 {
		t.Errorf("Expected 0 items from the feed but got %d.", len(resp.Items))
	}
	// TODO: add a second poll with an updated feed that will return new items.
}
