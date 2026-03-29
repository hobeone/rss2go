package watcher

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hobeone/rss2go/internal/crawler"
	"github.com/hobeone/rss2go/internal/mailer"
	"github.com/hobeone/rss2go/internal/models"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/mock"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) GetFeeds(ctx context.Context) ([]models.Feed, error) {
	args := m.Called(ctx)
	return args.Get(0).([]models.Feed), args.Error(1)
}
func (m *mockStore) GetFeedsWithErrors(ctx context.Context) ([]models.Feed, error) {
	args := m.Called(ctx)
	return args.Get(0).([]models.Feed), args.Error(1)
}
func (m *mockStore) GetFeed(ctx context.Context, id int64) (*models.Feed, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*models.Feed), args.Error(1)
}
func (m *mockStore) GetFeedByURL(ctx context.Context, url string) (*models.Feed, error) {
	args := m.Called(ctx, url)
	return args.Get(0).(*models.Feed), args.Error(1)
}
func (m *mockStore) AddFeed(ctx context.Context, url string, title string, fullArticle bool) (int64, error) {
	args := m.Called(ctx, url, title, fullArticle)
	return args.Get(0).(int64), args.Error(1)
}
func (m *mockStore) UpdateFeed(ctx context.Context, id int64, url *string, title *string, fullArticle *bool) error {
	args := m.Called(ctx, id, url, title, fullArticle)
	return args.Error(0)
}
func (m *mockStore) DeleteFeed(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *mockStore) DeleteFeedByURL(ctx context.Context, url string) error {
	args := m.Called(ctx, url)
	return args.Error(0)
}
func (m *mockStore) UpdateFeedLastPoll(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *mockStore) UpdateFeedError(ctx context.Context, id int64, code int, snippet string) error {
	args := m.Called(ctx, id, code, snippet)
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
func (m *mockStore) Unsubscribe(ctx context.Context, userID int64, feedID int64) error {
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
	feed := models.Feed{ID: 1, URL: "http://example.com/rss", Title: "Example & Feed"}
	store := new(mockStore)
	cPool := new(mockCrawler)
	mPool := new(mockMailer)
	logger := slog.New(slog.DiscardHandler)

	w := New(feed, store, cPool, mPool, time.Hour, 0, 600, logger)

	ctx := context.Background()

	// Mock DB behavior
	store.On("GetUsersForFeed", ctx, feed.ID).Return([]models.User{{ID: 1, Email: "user@example.com"}}, nil)
	store.On("IsSeen", ctx, feed.ID, "item-1").Return(false, nil)
	store.On("MarkSeen", ctx, feed.ID, "item-1").Return(nil)
	store.On("UpdateFeedLastPoll", ctx, feed.ID).Return(nil)
	store.On("UpdateFeedError", ctx, feed.ID, 0, "").Return(nil)

	// Mock Mailer behavior
	mPool.On("Submit", mock.MatchedBy(func(req mailer.MailRequest) bool {
		isSubjectSafe := req.Subject == "[Example & Feed] Safe Title & it's test"
		
		hasRealImg := strings.Contains(req.Body, "http://example.com/real.jpg")
		noTrackerImg := !strings.Contains(req.Body, "tracker.gif")
		noBadScript := !strings.Contains(req.Body, "bad")
		
		// New assertions for iframe and feedsportal
		hasIframeReplacement := strings.Contains(req.Body, "Embedded Content: https://www.youtube.com/embed/123")
		noFeedsPortal := !strings.Contains(req.Body, "da.feedsportal.com")

		return isSubjectSafe && hasRealImg && noTrackerImg && noBadScript && hasIframeReplacement && noFeedsPortal && req.To[0] == "user@example.com"
	})).Return()

	rss := `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
  <title>Example &amp; Feed &lt;script&gt;Feed&lt;/script&gt;</title>
  <item>
    <title>Safe Title &amp; it's test &lt;img src="x" onerror="alert(1)"&gt;</title>
    <link>http://example.com/item1</link>
    <guid>item-1</guid>
    <description>Safe &lt;b&gt;Description&lt;/b&gt;</description>
    <content:encoded><![CDATA[Full <b>Content</b>
	<script>bad</script>
	<img src="http://example.com/tracker.gif" width="1" height="1">
	<img src="http://example.com/real.jpg" alt="real">
	<iframe src="https://www.youtube.com/embed/123"></iframe>
	<a href="http://da.feedsportal.com/track">Tracker Link</a>
	]]></content:encoded>
  </item>
</channel>
</rss>`

	w.HandleResponse(ctx, crawler.CrawlResponse{
		FeedID: feed.ID,
		Type:   crawler.RequestTypeFeed,
		Body:   []byte(rss),
	})

	store.AssertExpectations(t)
	mPool.AssertExpectations(t)
}

func TestWatcher_HandleResponse_FullArticle(t *testing.T) {
	feed := models.Feed{ID: 1, URL: "http://example.com/rss", Title: "Full", FullArticle: true}
	store := new(mockStore)
	cPool := new(mockCrawler)
	mPool := new(mockMailer)
	logger := slog.New(slog.DiscardHandler)

	w := New(feed, store, cPool, mPool, time.Hour, 0, 600, logger)
	ctx := context.Background()

	// 1. Initial Feed Response
	store.On("GetUsersForFeed", ctx, feed.ID).Return([]models.User{{ID: 1, Email: "user@example.com"}}, nil).Twice()
	store.On("IsSeen", ctx, feed.ID, "item-1").Return(false, nil)
	store.On("UpdateFeedLastPoll", ctx, feed.ID).Return(nil)
	store.On("UpdateFeedError", ctx, feed.ID, 0, "").Return(nil)

	// Expect a crawl request for the item URL
	cPool.On("Submit", mock.MatchedBy(func(req crawler.CrawlRequest) bool {
		return req.Type == crawler.RequestTypeItem && req.URL == "http://example.com/item1" && req.ItemGUID == "item-1"
	})).Return()

	rss := `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0">
<channel>
  <title>Full</title>
  <item>
    <title>Item 1</title>
    <link>http://example.com/item1</link>
    <guid>item-1</guid>
    <description>Summary</description>
  </item>
</channel>
</rss>`

	w.HandleResponse(ctx, crawler.CrawlResponse{
		FeedID: feed.ID,
		Type:   crawler.RequestTypeFeed,
		Body:   []byte(rss),
	})

	// 2. Item Response
	itemHtml := `<html><body><article><p>Full article content here.</p></article></body></html>`
	
	mPool.On("Submit", mock.MatchedBy(func(req mailer.MailRequest) bool {
		return strings.Contains(req.Body, "Full article content here.") && !strings.Contains(req.Body, "Summary")
	})).Return()
	store.On("MarkSeen", ctx, feed.ID, "item-1").Return(nil)

	w.HandleResponse(ctx, crawler.CrawlResponse{
		FeedID:   feed.ID,
		Type:     crawler.RequestTypeItem,
		ItemGUID: "item-1",
		Body:     []byte(itemHtml),
	})

	store.AssertExpectations(t)
	cPool.AssertExpectations(t)
	mPool.AssertExpectations(t)
}

func TestWatcher_FormatItem_Sanitization(t *testing.T) {
	feed := models.Feed{ID: 1, URL: "http://example.com/rss", Title: "Example"}
	store := new(mockStore)
	cPool := new(mockCrawler)
	mPool := new(mockMailer)
	logger := slog.New(slog.DiscardHandler)

	w := New(feed, store, cPool, mPool, time.Hour, 0, 600, logger)

	item := &gofeed.Item{
		Title: "Test",
		Content: `<img src="javascript:alert(1)" alt="bad">
				 <img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg==" alt="data">
				 <img src="http://example.com/good.jpg" alt="good">`,
	}

	_, body := w.FormatItem("Example", item, "")

	if strings.Contains(body, "javascript:alert") {
		t.Errorf("body contains javascript in img src: %s", body)
	}
	if strings.Contains(body, "data:image") {
		t.Errorf("body contains data URI in img src: %s", body)
	}
	if !strings.Contains(body, "http://example.com/good.jpg") {
		t.Errorf("body missing good image URL: %s", body)
	}
}

func TestWatcher_FormatItem_LargeContent(t *testing.T) {
	feed := models.Feed{ID: 1, URL: "http://example.com/rss", Title: "Example"}
	w := New(feed, nil, nil, nil, time.Hour, 0, 600, slog.New(slog.DiscardHandler))

	// Create content > MaxItemContentSize
	largeContent := strings.Repeat("a", MaxItemContentSize+100)
	item := &gofeed.Item{
		Title:   "Large",
		Content: largeContent,
	}

	_, body := w.FormatItem("Example", item, "")
	if !strings.Contains(body, "[Content omitted: item too large to process safely]") {
		t.Errorf("large content should have been replaced by a placeholder")
	}
	if strings.Contains(body, largeContent) {
		t.Errorf("large content should not be present in the body")
	}
}

func TestWatcher_FormatItem_ImageWidth(t *testing.T) {
	feed := models.Feed{ID: 1, URL: "http://example.com/rss", Title: "Example"}
	// Max width 600
	w := New(feed, nil, nil, nil, time.Hour, 0, 600, slog.New(slog.DiscardHandler))

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "SmallImage",
			content:  `<img src="http://example.com/a.jpg" width="100" height="100">`,
			expected: `width="100" height="100"`,
		},
		{
			name:     "LargeWidthImage",
			content:  `<img src="http://example.com/b.jpg" width="800" height="400">`,
			expected: `<img src="http://example.com/b.jpg"/>`, // both should be stripped, self-closing
		},
		{
			name:     "LargeHeightImage",
			content:  `<img src="http://example.com/c.jpg" width="400" height="1200">`,
			expected: `<img src="http://example.com/c.jpg"/>`, // both should be stripped, self-closing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &gofeed.Item{
				Title:   "Test",
				Content: tt.content,
			}
			_, body := w.FormatItem("Example", item, "")
			if !strings.Contains(body, tt.expected) {
				t.Errorf("expected %q in body, got %q", tt.expected, body)
			}
			if tt.name != "SmallImage" {
				if strings.Contains(body, "width=") || strings.Contains(body, "height=") {
					t.Errorf("expected width/height to be stripped, but found them in body: %q", body)
				}
			}
		})
	}
}
