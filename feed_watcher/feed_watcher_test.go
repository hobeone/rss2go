package feedwatcher

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"gopkg.in/gomail.v2"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/log"
	"github.com/hobeone/rss2go/mail"
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

func (d *MockDBFailer) GetFeedByURL(string) (*db.FeedInfo, error) { return nil, nil }
func (d *MockDBFailer) GetFeedUsers(string) ([]db.User, error)    { return nil, nil }
func (d *MockDBFailer) SaveFeed(*db.FeedInfo) error               { return nil }
func (d *MockDBFailer) RecordGUID(int64, string) error            { return nil }
func (d *MockDBFailer) SetFeedGUIDS(int64, []string) ([]*db.FeedItem, error) {
	return []*db.FeedItem{}, nil
}
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
	responseChan := make(chan *FeedCrawlResponse)
	mailDispatcher := mail.CreateAndStartStubMailer()
	d := db.NewMemoryDBHandle(false, NullLogger(), true)
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

	return NewFeedWatcher(*feeds[0], crawlChan, responseChan, mailDispatcher.OutgoingMail, d, []string{}, 30, 100), feedResp, mailDispatcher
}

func makeRange(min, max int) []int {
	a := make([]int, max-min+1)
	for i := range a {
		a[i] = min + i
	}
	return a
}

func TestPruneGUIDS(t *testing.T) {
	t.Parallel()
	watcher, crawlBody, _ := SetupTest(t, "../testdata/ars.rss")
	guids, err := watcher.LoadGuidsFromDb(1000)
	if err != nil {
		t.Fatalf("Error getting GUIDS from db: %v", err)
	}
	if len(guids) != 0 {
		t.Fatalf("Expected 0 guids but got %d", len(guids))
	}

	r := makeRange(10, 1000)
	for i := range r {
		watcher.dbh.RecordGUID(watcher.FeedInfo.ID, fmt.Sprintf("guid-%d", i))
	}

	resp := &FeedCrawlResponse{
		HTTPResponseStatusCode: http.StatusOK,
		Body: crawlBody,
	}

	// Don't clean
	watcher.PruneGUIDS = false
	err = watcher.UpdateFeed(resp)
	if err != nil {
		t.Fatalf("Error updating feed: %s", err)
	}
	guids, err = watcher.LoadGuidsFromDb(1000)
	if err != nil {
		t.Fatalf("Error getting GUIDS from db: %v", err)
	}
	if len(guids) < 1000 {
		t.Fatalf("Cleaned database of old GUIDS.  Expected %d but got %d", 1000, len(guids))
	}

	//Clean
	watcher.PruneGUIDS = true
	err = watcher.UpdateFeed(resp)
	if err != nil {
		t.Fatalf("Error updating feed: %s", err)
	}

	maxguids := len(watcher.KnownGuids)

	guids, err = watcher.LoadGuidsFromDb(1000)
	if err != nil {
		t.Fatalf("Error getting GUIDS from db: %v", err)
	}
	if len(guids) > maxguids {
		t.Fatalf("Didn't clean database of old GUIDS.  Expected %d but got %d", maxguids, len(guids))
	}
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
	resp := <-n.responseChan
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
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
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
	resp := <-n.responseChan
	if len(resp.Items) != 20 {
		t.Fatalf("Expected 20 items from the feed. Got %d", len(resp.Items))
	}
	// Second Poll, should not have new items
	req = <-n.crawlChan
	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
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
		Error: nil,
	}
	go n.StopPoll()
	response := <-n.responseChan
	if response.Error == nil {
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

	_, stories, _ := feed.ParseFeed(n.FeedInfo.URL, feedResp)
	guids := make(map[string]bool, 25)
	for _, i := range stories {
		guids[i.ID] = true
	}
	n.KnownGuids = guids
	go n.PollFeed()
	req := <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	resp := <-n.responseChan
	if len(resp.Items) != 0 {
		t.Fatalf("Expected 0 items from the feed but got %d.", len(resp.Items))
	}
	// Second poll with an new items.
	n.KnownGuids = map[string]bool{}
	req = <-n.crawlChan

	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		Error:                  nil,
		HTTPResponseStatus:     "200 OK",
		HTTPResponseStatusCode: 200,
	}
	resp = <-n.responseChan
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
	_ = <-n.responseChan

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
	resp := <-n.responseChan
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
	resp := <-n.responseChan
	if resp.Error == nil {
		t.Fatalf("Expected and error on an empty feed. Got %d items", len(resp.Items))
	}

	req = <-n.crawlChan
	// No error on crawl, but got 403
	req.ResponseChan <- &FeedCrawlResponse{
		URI:                    n.FeedInfo.URL,
		Body:                   feedResp,
		HTTPResponseStatus:     "403 Forbidden",
		HTTPResponseStatusCode: 403,
	}
	resp = <-n.responseChan
	if resp.Error == nil {
		t.Fatalf("Expected and error on an empty feed.")
	}
	if len(resp.Items) > 0 {
		t.Fatalf("Expected 0 items on error got %d", len(resp.Items))
	}
}

func TestWithDoublePollFeed(t *testing.T) {
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
	t.Parallel()
	n, _, _ := SetupTest(t, "../testdata/empty.rss")
	n.lockCrawl()
	err := n.CrawlFeed().Error
	if err == nil || err != ErrAlreadyCrawlingFeed {
		t.Fatalf("Expected error to be ErrAlreadyCrawlingFeed got %v", err)
	}
}
