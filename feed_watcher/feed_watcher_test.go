package feedwatcher

import (
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"gopkg.in/gomail.v2"

	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/log"
	"github.com/hobeone/rss2go/mail"
	"github.com/sirupsen/logrus"
)

func NullLogger() logrus.FieldLogger {
	l := logrus.New()
	l.Out = ioutil.Discard
	return l
}

type FailMailer struct{}

func (m *FailMailer) SendMail(msg *gomail.Message) error {
	return fmt.Errorf("testing error")
}

// Mock Database handler to throw errors.
// TODO: rework so failures can be set per test
type MockDBFailer struct{}

func (d *MockDBFailer) GetFeedByURL(string) (*db.FeedInfo, error)             { return nil, nil }
func (d *MockDBFailer) GetFeedUsers(string) ([]db.User, error)                { return nil, nil }
func (d *MockDBFailer) SaveFeed(*db.FeedInfo) error                           { return nil }
func (d *MockDBFailer) RecordGUID(int64, string) error                        { return nil }
func (d *MockDBFailer) GetFeedItemByGUID(int64, string) (*db.FeedItem, error) { return nil, nil }
func (d *MockDBFailer) GetMostRecentGUIDsForFeed(i int64, m int) ([]string, error) {
	return []string{}, fmt.Errorf("test error")
}

func OverrideAfter(fw *FeedWatcher) {
	fw.After = func(d time.Duration) <-chan time.Time {
		return time.After(time.Duration(0))
	}
}

func SetupTest(t *testing.T, feedPath string) (*FeedWatcher, []byte, *mail.Dispatcher) {
	log.SetNullOutput()
	crawlChan := make(chan *FeedCrawlRequest)
	mailDispatcher := mail.CreateAndStartStubMailer()
	d := db.NewMemoryDBHandle(NullLogger(), true)
	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}
	if len(feeds) < 1 {
		t.Fatalf("Error: no feeds returned from database")
	}

	feedResp, err := ioutil.ReadFile(feedPath)
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	fw := NewFeedWatcher(*feeds[0], crawlChan, mailDispatcher.OutgoingMail, d, []string{}, 30, 100)
	fw.saveResponse = true
	return fw, feedResp, mailDispatcher
}

func TestNewFeedWatcher(t *testing.T) {
	t.Parallel()
	n, _, _ := SetupTest(t, "../testdata/empty.rss")
	if n.polling == true {
		t.Fatal("polling attribute set is true")
	}
}

func TestFeedWatcherPollLocking(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	n, feedResp, _ := SetupTest(t, "../testdata/ars.rss")
	OverrideAfter(n)

	n.dbh = &MockDBFailer{}
	go n.PollFeed()
	req := <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp := n.LastCrawlResponse
	if resp.Error == nil {
		t.Fatal("Should have gotten an error got nothing")
	}

}

func TestFeedWatcherPolling(t *testing.T) {
	t.Parallel()
	n, feedResp, mailDispatcher := SetupTest(t, "../testdata/ars.rss")

	feed, err := n.dbh.GetFeedByURL(n.FeedInfo.URL)
	if err != nil {
		t.Fatalf("Error getting expected feed: %s", err)
	}
	if feed.SiteURL != "" {
		t.Fatalf("Expected empty SiteURL before crawling")
	}

	OverrideAfter(n)

	go n.PollFeed()
	req := <-n.crawlChan
	if req.URI != n.FeedInfo.URL {
		t.Fatalf("URI not set on request properly.  Expected: %s Got: %s", n.FeedInfo.URL, req.URI)
	}

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp := n.LastCrawlResponse
	if resp.Error != nil {
		t.Fatalf("Should not have gotten an error. got: %s", resp.Error)
	}
	if len(resp.Items) != 25 {
		t.Fatalf("Expected 25 items from the feed. Got %d", len(resp.Items))
	}

	go n.PollFeed()
	// Second Poll, should not have new items
	req = <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp = n.LastCrawlResponse
	if len(resp.Items) != 0 {
		t.Fatalf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
	if len(n.GUIDCache) != 25 {
		t.Fatalf("Expected 25 known GUIDs got %d", len(n.GUIDCache))
	}
	// Hella Ghetto?
	c := mailDispatcher.MailSender.(*mail.NullMailSender).Count
	// 1 feed * 25 items * 3 users
	if c != 75 {
		t.Fatalf("Expected 75 mails to have been sent, got %d", c)
	}
	feed, err = n.dbh.GetFeedByURL(n.FeedInfo.URL)
	if err != nil {
		t.Fatalf("Error getting expected feed: %s", err)
	}
	feedLink := "http://arstechnica.com"
	if feed.SiteURL != feedLink {
		t.Fatalf("Expected SiteURL to be '%s' got %s", feedLink, feed.SiteURL)
	}
}

func TestFeedWatcherWithEmailErrors(t *testing.T) {
	t.Parallel()
	n, feedResp, _ := SetupTest(t, "../testdata/bicycling_rss.xml")
	OverrideAfter(n)

	mailer := mail.NewDispatcher(
		"from@example.com",
		&FailMailer{},
	)
	go mailer.DispatchLoop()
	n.mailerChan = mailer.OutgoingMail

	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp := n.LastCrawlResponse

	if resp.Error == nil {
		t.Fatalf("Should have gotten an error.")
	}

	if len(resp.Items) != 20 {
		t.Fatalf("Expected 20 items from the feed. Got %d", len(resp.Items))
	}

	if len(n.GUIDCache) != 0 {
		t.Fatalf("Expected no known guids, got %d", len(n.GUIDCache))
	}
}

func TestFeedWatcherPollingRssWithNoItemDates(t *testing.T) {
	t.Parallel()
	n, feedResp, _ := SetupTest(t, "../testdata/bicycling_rss.xml")
	OverrideAfter(n)

	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp := n.LastCrawlResponse
	if len(resp.Items) != 20 {
		t.Fatalf("Expected 20 items from the feed. Got %d", len(resp.Items))
	}
	// Second Poll, should not have new items
	go n.PollFeed()
	req = <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp = n.LastCrawlResponse
	if resp.Error != nil {
		t.Fatalf("Should not have gotten an error. got: %s", resp.Error)
	}

	if len(resp.Items) != 0 {
		t.Fatalf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
}

func TestFeedWatcherWithMalformedFeed(t *testing.T) {
	t.Parallel()
	n, _, _ := SetupTest(t, "../testdata/ars.rss")
	OverrideAfter(n)
	go n.PollFeed()

	req := <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   []byte("Testing"),
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
		Error:                  nil,
	}
	n.StopPoll()
	resp := n.LastCrawlResponse
	if resp.Error == nil {
		t.Error("Expected error parsing invalid feed.")
	}

	dbFeed, err := n.dbh.GetFeedByURL(n.FeedInfo.URL)
	if err != nil {
		t.Error(err.Error())
	}
	if dbFeed.LastPollError == "" {
		t.Error("Feed should have error set")
	}
}

func TestFeedWatcherWithGuidsSet(t *testing.T) {
	t.Parallel()
	n, feedResp, _ := SetupTest(t, "../testdata/ars.rss")
	OverrideAfter(n)

	feed, _ := feed.ParseFeed(n.FeedInfo.URL, feedResp)
	guids := make(map[string]bool, 25)
	for _, i := range feed.Items {
		guids[i.GUID] = true
	}
	n.GUIDCache = guids
	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp := n.LastCrawlResponse
	if len(resp.Items) != 0 {
		t.Fatalf("Expected 0 items from the feed but got %d.", len(resp.Items))
	}
	// Second poll with an new items.
	n.GUIDCache = map[string]bool{}
	go n.PollFeed()
	req = <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp = n.LastCrawlResponse
	if len(resp.Items) != 25 {
		t.Fatalf("Expected 25 items from the feed but got %d.", len(resp.Items))
	}
}

func TestFeedWatcherWithTooRecentLastPoll(t *testing.T) {
	t.Parallel()
	n, feedResp, _ := SetupTest(t, "../testdata/ars.rss")
	n.FeedInfo.LastPollTime = time.Now()

	afterCalls := 0
	n.After = func(d time.Duration) <-chan time.Time {
		afterCalls++
		if afterCalls < 2 {
			d = time.Duration(0)
		}
		return time.After(d)
	}

	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()

	if afterCalls != 2 {
		t.Fatalf("Expecting after to be called twice, called %d times", afterCalls)
	}
	n.StopPoll()
	if n.Polling() {
		t.Fatalf("Called StopPoll but still polling")
	}
}

func TestWithEmptyFeed(t *testing.T) {
	t.Parallel()
	n, feedResp, _ := SetupTest(t, "../testdata/empty.rss")
	OverrideAfter(n)
	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	n.StopPoll()
	resp := n.LastCrawlResponse
	if resp.Error == nil {
		t.Fatalf("Expected and error on an empty feed. Got %d items", len(resp.Items))
	}
}

func TestWithErrorOnCrawl(t *testing.T) {
	t.Parallel()
	n, feedResp, _ := SetupTest(t, "../testdata/empty.rss")
	OverrideAfter(n)
	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  fmt.Errorf("error crawling feed"),
		HTTPResponseStatus:     "403 Forbidden",
		HTTPResponseStatusCode: 403,
	}
	n.StopPoll()
	resp := n.LastCrawlResponse
	if resp.Error == nil {
		t.Fatalf("Expected and error on an empty feed. Got %d items", len(resp.Items))
	}

	go n.PollFeed()
	req = <-n.crawlChan
	// No error on crawl, but got 403
	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		HTTPResponseStatus:     "403 Forbidden",
		HTTPResponseStatusCode: 403,
	}
	n.StopPoll()
	resp = n.LastCrawlResponse
	if resp.Error == nil {
		t.Fatalf("Expected and error on an empty feed.")
	}
	if len(resp.Items) > 0 {
		t.Fatalf("Expected 0 items on error got %d", len(resp.Items))
	}
}

func TestWithDoublePollFeed(t *testing.T) {
	t.Parallel()
	n, _, _ := SetupTest(t, "../testdata/empty.rss")
	//OverrideAfter(n)
	go n.PollFeed()
	// Seems to be a problem where the lock isn't created fast enough if you call
	// these back to back.
	// FIXME: actually use goroutines the right way
	time.Sleep(100 * time.Millisecond)
	r := n.PollFeed()
	n.StopPoll()
	if r {
		t.Fatal("Calling PollFeed twice should return false")
	}
}

func TestCrawlLock(t *testing.T) {
	t.Parallel()
	n, _, _ := SetupTest(t, "../testdata/empty.rss")
	n.lockCrawl()
	err := n.CrawlFeed().Error
	if err == nil || err != ErrAlreadyCrawlingFeed {
		t.Fatalf("Expected error to be ErrAlreadyCrawlingFeed got %v", err)
	}
}
