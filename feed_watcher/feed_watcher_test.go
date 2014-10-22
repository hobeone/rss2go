package feedwatcher

import (
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/mail"
	. "github.com/smartystreets/goconvey/convey"
)

type FailMailer struct{}

func (m *FailMailer) SendMail(msg mail.Message) error {
	return fmt.Errorf("testing error")
}

// Mock Database handler to throw errors.
// TODO: rework so failures can be set per test
type MockDBFailer struct{}

func (d *MockDBFailer) GetFeedByUrl(string) (*db.FeedInfo, error)             { return nil, nil }
func (d *MockDBFailer) GetFeedUsers(string) ([]db.User, error)                { return nil, nil }
func (d *MockDBFailer) SaveFeed(*db.FeedInfo) error                           { return nil }
func (d *MockDBFailer) RecordGuid(int64, string) error                        { return nil }
func (d *MockDBFailer) GetFeedItemByGuid(int64, string) (*db.FeedItem, error) { return nil, nil }
func (d *MockDBFailer) GetMostRecentGuidsForFeed(i int64, m int) ([]string, error) {
	return []string{}, fmt.Errorf("test error")
}

func OverrideAfter(fw *FeedWatcher) {
	fw.After = func(d time.Duration) <-chan time.Time {
		return time.After(time.Duration(0))
	}
}

func SetupTest(t *testing.T, feedPath string) (*FeedWatcher, []byte, *mail.MailDispatcher) {
	crawlChan := make(chan *FeedCrawlRequest)
	responseChan := make(chan *FeedCrawlResponse)
	mailDispatcher := mail.CreateAndStartStubMailer()
	d := db.NewMemoryDBHandle(false, true)
	feeds, _ := db.LoadFixtures(t, d, "http://localhost")

	feedResp, err := ioutil.ReadFile(feedPath)
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	return NewFeedWatcher(*feeds[0], crawlChan, responseChan, mailDispatcher.OutgoingMail, d, []string{}, 30, 100), feedResp, mailDispatcher
}

func TestNewFeedWatcher(t *testing.T) {
	n, _, _ := SetupTest(t, "../testdata/empty.rss")
	if n.polling == true {
		t.Fatal("polling attribute set is true")
	}
}

func TestFeedWatcherPollLocking(t *testing.T) {
	n, _, _ := SetupTest(t, "../testdata/empty.rss")
	if n.Polling() {
		t.Fatal("A new watcher shouldn't be polling")
	}
	n.lockPoll()
	if !n.Polling() {
		t.Fatal("Watching didn't set polling lock")
	}
}

func TestPollFeedWithDBErrors(t *testing.T) {
	n, feedResp, _ := SetupTest(t, "../testdata/ars.rss")
	OverrideAfter(n)

	n.dbh = &MockDBFailer{}
	go n.PollFeed()
	req := <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	resp := <-n.responseChan
	if resp.Error == nil {
		t.Fatal("Should have gotten an error got nothing")
	}

}

func TestFeedWatcherPolling(t *testing.T) {
	n, feedResp, mailDispatcher := SetupTest(t, "../testdata/ars.rss")
	OverrideAfter(n)

	go n.PollFeed()
	req := <-n.crawlChan
	if req.URI != n.FeedInfo.Url {
		t.Fatalf("URI not set on request properly.  Expected: %s Got: %s", n.FeedInfo.Url, req.URI)
	}

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	resp := <-n.responseChan
	if resp.Error != nil {
		t.Fatalf("Should not have gotten an error. got: %s", resp.Error)
	}
	if len(resp.Items) != 25 {
		t.Fatalf("Expected 25 items from the feed. Got %d", len(resp.Items))
	}
	// Second Poll, should not have new items
	req = <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	go n.StopPoll()
	resp = <-n.responseChan
	if len(resp.Items) != 0 {
		t.Fatalf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
	if len(n.KnownGuids) != 25 {
		t.Fatalf("Expected 25 known GUIDs got %d", len(n.KnownGuids))
	}
	// Hella Ghetto?
	c := mailDispatcher.MailSender.(*mail.NullMailSender).Count
	// 1 feed * 25 items * 3 users
	if c != 75 {
		t.Fatalf("Expected 75 mails to have been sent, got %d", c)
	}
}

func TestFeedWatcherWithEmailErrors(t *testing.T) {
	n, feedResp, _ := SetupTest(t, "../testdata/bicycling_rss.xml")
	OverrideAfter(n)

	mailer := mail.NewMailDispatcher(
		"from@example.com",
		&FailMailer{},
	)
	go mailer.DispatchLoop()
	n.mailerChan = mailer.OutgoingMail

	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	resp := <-n.responseChan

	if resp.Error == nil {
		t.Fatalf("Should have gotten an error.")
	}

	if len(resp.Items) != 20 {
		t.Fatalf("Expected 20 items from the feed. Got %d", len(resp.Items))
	}

	if len(n.KnownGuids) != 0 {
		t.Fatalf("Expected no known guids, got %d", len(n.KnownGuids))
	}
}

func TestFeedWatcherPollingRssWithNoItemDates(t *testing.T) {
	n, feedResp, _ := SetupTest(t, "../testdata/bicycling_rss.xml")
	OverrideAfter(n)

	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	resp := <-n.responseChan
	if len(resp.Items) != 20 {
		t.Fatalf("Expected 20 items from the feed. Got %d", len(resp.Items))
	}
	// Second Poll, should not have new items
	req = <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	go n.StopPoll()
	resp = <-n.responseChan
	if resp.Error != nil {
		t.Fatalf("Should not have gotten an error. got: %s", resp.Error)
	}

	if len(resp.Items) != 0 {
		t.Fatalf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
}

func TestFeedWatcherWithMalformedFeed(t *testing.T) {
	n, _, _ := SetupTest(t, "../testdata/ars.rss")
	OverrideAfter(n)
	go n.PollFeed()

	req := <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  []byte("Testing"),
		Error: nil,
	}
	go n.StopPoll()
	response := <-n.responseChan
	if response.Error == nil {
		t.Error("Expected error parsing invalid feed.")
	}

	dbFeed, err := n.dbh.GetFeedByUrl(n.FeedInfo.Url)
	if err != nil {
		t.Error(err.Error())
	}
	if dbFeed.LastPollError == "" {
		t.Error("Feed should have error set")
	}
}

func TestFeedWatcherWithGuidsSet(t *testing.T) {
	n, feedResp, _ := SetupTest(t, "../testdata/ars.rss")
	OverrideAfter(n)

	_, stories, _ := feed.ParseFeed(n.FeedInfo.Url, feedResp)
	guids := make(map[string]bool, 25)
	for _, i := range stories {
		guids[i.Id] = true
	}
	n.KnownGuids = guids
	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	resp := <-n.responseChan
	if len(resp.Items) != 0 {
		t.Fatalf("Expected 0 items from the feed but got %d.", len(resp.Items))
	}
	// Second poll with an new items.
	n.KnownGuids = map[string]bool{}
	req = <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	resp = <-n.responseChan
	if len(resp.Items) != 25 {
		t.Fatalf("Expected 25 items from the feed but got %d.", len(resp.Items))
	}
}

func TestFeedWatcherWithTooRecentLastPoll(t *testing.T) {
	n, feedResp, _ := SetupTest(t, "../testdata/ars.rss")
	n.FeedInfo.LastPollTime = time.Now()

	afterCalls := 0
	n.After = func(d time.Duration) <-chan time.Time {
		afterCalls++
		return time.After(time.Duration(0))
	}

	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	_ = <-n.responseChan

	if afterCalls != 2 {
		t.Fatalf("Expecting after to be called twice, called %d times", afterCalls)
	}
}

func TestWithEmptyFeed(t *testing.T) {
	n, feedResp, _ := SetupTest(t, "../testdata/empty.rss")
	OverrideAfter(n)
	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	resp := <-n.responseChan
	if resp.Error == nil {
		t.Fatalf("Expected and error on an empty feed. Got %d items", len(resp.Items))
	}
}

func TestWithErrorOnCrawl(t *testing.T) {
	n, feedResp, _ := SetupTest(t, "../testdata/empty.rss")
	OverrideAfter(n)
	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: fmt.Errorf("error crawling feed"),
	}
	resp := <-n.responseChan
	if resp.Error == nil {
		t.Fatalf("Expected and error on an empty feed. Got %d items", len(resp.Items))
	}
}
func TestWithDoublePollFeed(t *testing.T) {
	n, feedResp, _ := SetupTest(t, "../testdata/empty.rss")
	OverrideAfter(n)
	go n.PollFeed()

	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:   n.FeedInfo.Url,
		Body:  feedResp,
		Error: nil,
	}
	resp := <-n.responseChan
	if len(resp.Items) != 0 {
		t.Fatalf("Expected 0 items from the feed but got %d.", len(resp.Items))
	}
	r := n.PollFeed()
	if r {
		t.Fatal("Calling PollFeed twice should return false")
	}
}

func TestCrawlLock(t *testing.T) {
	Convey("Subject FeedWatcher Crawl Lock:", t, func() {
		n, _, _ := SetupTest(t, "../testdata/empty.rss")
		Convey("Given already crawling", func() {
			n.lockCrawl()
			So(n.CrawlFeed().Error, ShouldEqual, ErrAlreadyCrawlingFeed)
		})
	})
}
