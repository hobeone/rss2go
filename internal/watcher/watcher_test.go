package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hobeone/rss2go/internal/crawler"
	"github.com/hobeone/rss2go/internal/mailer"
	"github.com/hobeone/rss2go/internal/models"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
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
	f, _ := args.Get(0).(*models.Feed)
	return f, args.Error(1)
}
func (m *mockStore) GetFeedByURL(ctx context.Context, url string) (*models.Feed, error) {
	args := m.Called(ctx, url)
	f, _ := args.Get(0).(*models.Feed)
	return f, args.Error(1)
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
func (m *mockStore) UpdateFeedLastPoll(ctx context.Context, id int64, etag string, lastModified string) error {
	args := m.Called(ctx, id, etag, lastModified)
	return args.Error(0)
}
func (m *mockStore) SetFeedError(ctx context.Context, id int64, code int, snippet string) error {
	args := m.Called(ctx, id, code, snippet)
	return args.Error(0)
}
func (m *mockStore) ClearFeedError(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *mockStore) AddUser(ctx context.Context, email string) (int64, error) {
	args := m.Called(ctx, email)
	return args.Get(0).(int64), args.Error(1)
}
func (m *mockStore) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	u, _ := args.Get(0).(*models.User)
	return u, args.Error(1)
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
func (m *mockStore) UpdateFeedBackoff(ctx context.Context, id int64, backoffUntil time.Time) error {
	args := m.Called(ctx, id, backoffUntil)
	return args.Error(0)
}
func (m *mockStore) EnqueueEmail(ctx context.Context, recipients []string, subject, body string) error {
	args := m.Called(ctx, recipients, subject, body)
	return args.Error(0)
}
func (m *mockStore) ClaimPendingEmail(ctx context.Context) (*models.OutboxEntry, error) {
	args := m.Called(ctx)
	e, _ := args.Get(0).(*models.OutboxEntry)
	return e, args.Error(1)
}
func (m *mockStore) MarkEmailDelivered(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *mockStore) ResetEmailToPending(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *mockStore) ResetDeliveringToPending(ctx context.Context) error {
	args := m.Called(ctx)
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

func (m *mockMailer) Submit(ctx context.Context, req mailer.MailRequest) {
	m.Called(ctx, req)
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
	store.On("UpdateFeedLastPoll", ctx, feed.ID, mock.Anything, mock.Anything).Return(nil)
	store.On("ClearFeedError", ctx, feed.ID).Return(nil)
	store.On("UpdateFeedBackoff", ctx, feed.ID, mock.Anything).Return(nil)

	// Mock Mailer behavior
	mPool.On("Submit", mock.Anything, mock.MatchedBy(func(req mailer.MailRequest) bool {
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

func TestWatcher_HandleResponse_BackoffPersisted(t *testing.T) {
	feed := models.Feed{ID: 1, URL: "http://example.com/rss", Title: "Example"}
	store := new(mockStore)
	cPool := new(mockCrawler)
	mPool := new(mockMailer)
	logger := slog.New(slog.DiscardHandler)

	w := New(feed, store, cPool, mPool, time.Hour, 0, 600, logger)
	ctx := context.Background()

	store.On("SetFeedError", ctx, feed.ID, 429, mock.Anything).Return(nil)
	store.On("UpdateFeedBackoff", ctx, feed.ID, mock.MatchedBy(func(t time.Time) bool {
		// backoff_until should be in the future
		return t.After(time.Now())
	})).Return(nil)

	interval, isFeed := w.HandleResponse(ctx, crawler.CrawlResponse{
		FeedID:     feed.ID,
		Type:       crawler.RequestTypeFeed,
		StatusCode: 429,
		RetryAfter: 2 * time.Minute,
		Error:      fmt.Errorf("unexpected status code: 429"),
	})

	assert.True(t, isFeed)
	assert.Equal(t, 2*time.Minute, interval)
	store.AssertExpectations(t)
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
	store.On("UpdateFeedLastPoll", ctx, feed.ID, mock.Anything, mock.Anything).Return(nil)
	store.On("ClearFeedError", ctx, feed.ID).Return(nil)
	store.On("UpdateFeedBackoff", ctx, feed.ID, mock.Anything).Return(nil)

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
	
	mPool.On("Submit", mock.Anything, mock.MatchedBy(func(req mailer.MailRequest) bool {
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
		name          string
		content       string
		expectedBody  string
		expectedStyle bool
		expectStrip   bool
	}{
		{
			name:          "SmallImage",
			content:       `<img src="http://example.com/a.jpg" width="100" height="100">`,
			expectedBody:  `width="100" height="100"`,
			expectedStyle: false,
			expectStrip:   false,
		},
		{
			name:          "MediumImage",
			content:       `<img src="http://example.com/m.jpg" width="400" height="400">`,
			expectedBody:  `width="400" height="400"`,
			expectedStyle: true,
			expectStrip:   false,
		},
		{
			name:          "LargeWidthImage",
			content:       `<img src="http://example.com/b.jpg" width="800" height="400">`,
			expectedBody:  `<img src="http://example.com/b.jpg"`,
			expectedStyle: true,
			expectStrip:   true,
		},
		{
			name:          "LargeHeightImage",
			content:       `<img src="http://example.com/c.jpg" width="400" height="1200">`,
			expectedBody:  `<img src="http://example.com/c.jpg"`,
			expectedStyle: true,
			expectStrip:   true,
		},
		{
			name:          "UnknownSizeImage",
			content:       `<img src="http://example.com/u.jpg">`,
			expectedBody:  `<img src="http://example.com/u.jpg"`,
			expectedStyle: true,
			expectStrip:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &gofeed.Item{
				Title:   "Test",
				Content: tt.content,
			}
			_, body := w.FormatItem("Example", item, "")
			if !strings.Contains(body, tt.expectedBody) {
				t.Errorf("expected %q in body, got %q", tt.expectedBody, body)
			}
			hasStyle := strings.Contains(body, `style="max-width: 100%; height: auto"`)
			if hasStyle != tt.expectedStyle {
				t.Errorf("expected style presence: %v, got %v in body: %q", tt.expectedStyle, hasStyle, body)
			}
			hasWidth := strings.Contains(body, "width=")
			if tt.expectStrip && hasWidth {
				t.Errorf("expected width to be stripped, but it was found in body: %q", body)
			}
			if !tt.expectStrip && strings.Contains(tt.content, "width=") && !hasWidth {
				t.Errorf("expected width to be preserved, but it was missing from body: %q", body)
			}
		})
	}
}

func TestWatcher_FormatItem_BikerumorFeed(t *testing.T) {
	feed := models.Feed{ID: 1, URL: "https://bikerumor.com/feed/", Title: "Bikerumor"}
	w := New(feed, nil, nil, nil, time.Hour, 0, 600, slog.New(slog.DiscardHandler))

	item := &gofeed.Item{
		Title: "XFusion Teases 2 New 32” Suspension Forks, But When Will They Be Ready?",
		Link:  "https://bikerumor.com/prototype-32er-xfusion-rezza-34-get-30-suspension-forks/",
		Content: `<p><img src="https://bikerumor.com/wp-content/uploads/2026/03/IMG_1398-1536x1024.jpeg" width="1536" height="1024" alt="<i>(Photo/Cory Benson)</i>"></p>
<figure class="wp-block-image size-full"><a href="https://bikerumor.com/wp-content/uploads/2026/03/IMG_1378-scaled.jpeg"><img fetchpriority="high" decoding="async" width="2560" height="1707" src="https://bikerumor.com/wp-content/uploads/2026/03/IMG_1378-scaled.jpeg" alt="XFusion Rezza 32er mockup XC suspension fork up close" class="wp-image-412574 first-image" style="object-fit:full" srcset="https://bikerumor.com/wp-content/uploads/2026/03/IMG_1378-scaled.jpeg 2560w, https://bikerumor.com/wp-content/uploads/2026/03/IMG_1378-297x198.jpeg 297w" sizes="(max-width: 2560px) 100vw, 2560px" /></a><figcaption class="wp-element-caption">(All photos/Cory Benson)</figcaption></figure>`,
	}

	_, body := w.FormatItem("Bikerumor", item, "")

	// Assertions for large images
	assert.NotContains(t, body, `width="1536"`)
	assert.NotContains(t, body, `height="1024"`)
	assert.NotContains(t, body, `width="2560"`)
	assert.NotContains(t, body, `height="1707"`)
	
	// Assertions for responsive style
	assert.Contains(t, body, `style="max-width: 100%; height: auto"`)
	
	// Assertions for stripped attributes
	assert.NotContains(t, body, `srcset=`)
	assert.NotContains(t, body, `sizes=`)
	assert.NotContains(t, body, `fetchpriority=`)
	assert.NotContains(t, body, `decoding=`)
	
	// Ensure allowed content remains
	assert.Contains(t, body, `https://bikerumor.com/wp-content/uploads/2026/03/IMG_1398-1536x1024.jpeg`)
	assert.Contains(t, body, `XFusion Rezza 32er mockup XC suspension fork up close`)
}
