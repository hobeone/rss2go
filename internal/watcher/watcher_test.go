package watcher

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/hobe/rss2go/internal/crawler"
	"github.com/hobe/rss2go/internal/mailer"
	"github.com/hobe/rss2go/internal/models"
	"github.com/stretchr/testify/mock"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) GetFeeds(ctx context.Context) ([]models.Feed, error) {
	args := m.Called(ctx)
	return args.Get(0).([]models.Feed), args.Error(1)
}
func (m *mockStore) GetFeed(ctx context.Context, id int64) (*models.Feed, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*models.Feed), args.Error(1)
}
func (m *mockStore) AddFeed(ctx context.Context, url string, title string) (int64, error) {
	args := m.Called(ctx, url, title)
	return args.Get(0).(int64), args.Error(1)
}
func (m *mockStore) UpdateFeedLastPoll(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *mockStore) AddUser(ctx context.Context, email string) (int64, error) {
	args := m.Called(ctx, email)
	return args.Get(0).(int64), args.Error(1)
}
func (m *mockStore) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *mockStore) GetUsersForFeed(ctx context.Context, feedID int64) ([]models.User, error) {
	args := m.Called(ctx, feedID)
	return args.Get(0).([]models.User), args.Error(1)
}
func (m *mockStore) Subscribe(ctx context.Context, userID int64, feedID int64) error {
	args := m.Called(ctx, userID, feedID)
	return args.Error(0)
}
func (m *mockStore) IsSeen(ctx context.Context, feedID int64, guid string) (bool, error) {
	args := m.Called(ctx, feedID, guid)
	return args.Bool(0), args.Error(1)
}
func (m *mockStore) MarkSeen(ctx context.Context, feedID int64, guid string) error {
	args := m.Called(ctx, feedID, guid)
	return args.Error(0)
}
func (m *mockStore) Close() error {
	return nil
}

type mockCrawler struct {
	mock.Mock
}

func (m *mockCrawler) Submit(req crawler.CrawlRequest) {
	m.Called(req)
}

type mockMailer struct {
	mock.Mock
}

func (m *mockMailer) Submit(req mailer.MailRequest) {
	m.Called(req)
}

func TestWatcher_HandleResponse(t *testing.T) {
	feed := models.Feed{ID: 1, URL: "http://example.com/rss", Title: "Example"}
	store := new(mockStore)
	cPool := new(mockCrawler)
	mPool := new(mockMailer)
	logger := slog.New(slog.DiscardHandler)

	w := New(feed, store, cPool, mPool, time.Hour, 0, logger)

	ctx := context.Background()

	// Mock DB behavior
	store.On("GetUsersForFeed", ctx, feed.ID).Return([]models.User{{ID: 1, Email: "user@example.com"}}, nil)
	store.On("IsSeen", ctx, feed.ID, "item-1").Return(false, nil)
	store.On("MarkSeen", ctx, feed.ID, "item-1").Return(nil)
	store.On("UpdateFeedLastPoll", ctx, feed.ID).Return(nil)

	// Mock Mailer behavior
	mPool.On("Submit", mock.MatchedBy(func(req mailer.MailRequest) bool {
		// Verify sanitization
		// bluemonday StrictPolicy strips the entire tag, not just the brackets, leaving trailing space in this specific XML.
		// We use strings.TrimSpace in the actual code, so trailing spaces are removed.
		isSubjectSafe := req.Subject == "[Example] Safe Title"
		// The test now verifies Content is used if available. 
		isBodySafe := req.Body == "Full <b>Content</b><br><br><a href=\"http://example.com/item1\">Read more</a>"
		return isSubjectSafe && isBodySafe && req.To[0] == "user@example.com"
	})).Return()

	rss := `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
  <title>Example &lt;script&gt;Feed&lt;/script&gt;</title>
  <item>
    <title>Safe Title &lt;img src="x" onerror="alert(1)"&gt;</title>
    <link>http://example.com/item1</link>
    <guid>item-1</guid>
    <description>Safe &lt;b&gt;Description&lt;/b&gt;</description>
    <content:encoded><![CDATA[Full <b>Content</b><script>bad</script><img src="tracker.gif">]]></content:encoded>
  </item>
</channel>
</rss>`

	w.HandleResponse(ctx, crawler.CrawlResponse{
		FeedID: feed.ID,
		Body:   []byte(rss),
	})

	store.AssertExpectations(t)
	mPool.AssertExpectations(t)
}
