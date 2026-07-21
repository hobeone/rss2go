package scheduler

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rss2go/internal/crawler"
	"rss2go/internal/database"
	"rss2go/internal/extractor"
	"rss2go/internal/sanitizer"
	"rss2go/internal/types"
)

const mockFeedXML = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0">
  <channel>
    <title>Mock Feed</title>
    <link>http://mock.site</link>
    <description>Test Description</description>
    <item>
      <title>Article 1</title>
      <link>%s/article-1</link>
      <guid>guid-1</guid>
      <description>Summary content of Article 1</description>
    </item>
  </channel>
</rss>`

type mockServerController struct {
	server        *httptest.Server
	notModified   bool
	failCrawl     bool
	rateLimit     bool
	retryAfterVal string
	mu            sync.Mutex
}

func makeMockServer(t *testing.T) *mockServerController {
	c := &mockServerController{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.failCrawl {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if c.rateLimit {
			val := "120"
			if c.retryAfterVal != "" {
				val = c.retryAfterVal
			}
			w.Header().Set("Retry-After", val)
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		if c.notModified {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/feed.xml") {
			w.Header().Set("Content-Type", "application/xml")
			w.Header().Set("ETag", "etag-v1")
			w.Header().Set("Last-Modified", "last-mod-v1")
			serverURL := fmt.Sprintf("http://%s", r.Host)
			_, _ = w.Write(fmt.Appendf(nil, mockFeedXML, serverURL))
			return
		}

		if strings.HasSuffix(r.URL.Path, "/article-1") {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><article><h1>Article 1</h1><div class="content"><p>Full body text extracted.</p></div></article></body></html>`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))

	return c
}

func setupTestDB(t *testing.T) *database.Repository {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return database.NewRepository(db)
}

func TestSchedulerHappyPath(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	ctrl := makeMockServer(t)
	defer ctrl.server.Close()

	cr := crawler.NewCrawler(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	ex := extractor.NewExtractor(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	sa := sanitizer.NewSanitizer(600)
	s := New(repo, cr, ex, sa, Config{
		PollInterval: 5 * time.Millisecond,
		MaxWorkers:   2,
	}, nil)

	// Create user
	u := &types.User{Email: "subscriber@test.com"}
	if err := repo.CreateUser(ctx, u); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Create feed
	feed := &types.Feed{
		Title:              "Mock Feed",
		URL:                ctrl.server.URL + "/feed.xml",
		PollIntervalSecs:   60,
		BackoffFactor:      1.0,
		NextPollAt:         time.Now().Add(-time.Hour),
		ExtractFullArticle: true,
		ExtractionStrategy: types.StrategyHeuristic,
	}
	if err := repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	// Subscribe
	if err := repo.Subscribe(ctx, u.ID, feed.ID); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Process
	s.processFeed(ctx, feed)

	// Verify updates to feed metadata
	updatedFeed, err := repo.GetFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("failed to fetch updated feed: %v", err)
	}
	if updatedFeed.ETag != "etag-v1" || updatedFeed.LastModified != "last-mod-v1" {
		t.Errorf("feed metadata cache markers not updated: %+v", updatedFeed)
	}
	if updatedFeed.BackoffFactor != 1.0 || updatedFeed.LastErrorStr != "" {
		t.Errorf("expected clean feed status, got: %+v", updatedFeed)
	}
	diff := time.Until(updatedFeed.NextPollAt)
	expectedInterval := time.Duration(feed.PollIntervalSecs) * time.Second
	if diff < expectedInterval-10*time.Second || diff > expectedInterval+10*time.Second {
		t.Errorf("expected next poll time on success to be around %v from now, got diff %v", expectedInterval, diff)
	}

	// Verify outbox item was enqueued
	items, err := repo.ListPendingOutboxItems(ctx, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("failed to list pending outbox items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 outbox item, got %d", len(items))
	}

	item := items[0]
	if !strings.Contains(item.Subject, "Article 1") {
		t.Errorf("expected subject to contain Article 1, got %q", item.Subject)
	}
	if len(item.Recipients) != 1 || item.Recipients[0] != "subscriber@test.com" {
		t.Errorf("expected recipient subscriber@test.com, got %v", item.Recipients)
	}
	if !strings.Contains(item.Body, "Full body text extracted") {
		t.Errorf("expected body to contain extracted content, got %q", item.Body)
	}

	seen, err := repo.IsItemSeen(ctx, feed.ID, "guid-1")
	if err != nil {
		t.Fatalf("failed to check seen: %v", err)
	}
	if !seen {
		t.Errorf("expected item to be marked seen")
	}
}

func TestSchedulerExtractionStrategiesAndFailures(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	ctrl := makeMockServer(t)
	defer ctrl.server.Close()

	cr := crawler.NewCrawler(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	ex := extractor.NewExtractor(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	sa := sanitizer.NewSanitizer(600)
	s := New(repo, cr, ex, sa, Config{}, nil)

	// Create user
	u := &types.User{Email: "subscriber@test.com"}
	if err := repo.CreateUser(ctx, u); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	t.Run("css selector strategy success", func(t *testing.T) {
		feed := &types.Feed{
			Title:              "CSS Selector Feed",
			URL:                ctrl.server.URL + "/feed.xml",
			PollIntervalSecs:   60,
			BackoffFactor:      1.0,
			NextPollAt:         time.Now().Add(-time.Hour),
			ExtractFullArticle: true,
			ExtractionStrategy: types.StrategySelector,
			CSSSelector:        "div.content",
		}
		if err := repo.CreateFeed(ctx, feed); err != nil {
			t.Fatalf("failed to create feed: %v", err)
		}
		if err := repo.Subscribe(ctx, u.ID, feed.ID); err != nil {
			t.Fatalf("failed to subscribe: %v", err)
		}

		s.processFeed(ctx, feed)

		items, err := repo.ListPendingOutboxItems(ctx, time.Now().Add(time.Second))
		if err != nil {
			t.Fatalf("failed to list: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 enqueued item, got %d", len(items))
		}
		// Confirm the css selector matched content is in the email
		if !strings.Contains(items[0].Body, "Full body text extracted") {
			t.Errorf("expected body to contain CSS selector match, got %q", items[0].Body)
		}
	})

	t.Run("extraction failure fallback to summary", func(t *testing.T) {
		// Clean database seen items
		if err := repo.UnmarkSeenItems(ctx, 1, 100); err != nil {
			t.Logf("cleanup error: %v", err)
		}

		feed := &types.Feed{
			Title:              "Fallback Feed",
			URL:                ctrl.server.URL + "/feed.xml?fallback=1",
			PollIntervalSecs:   60,
			BackoffFactor:      1.0,
			NextPollAt:         time.Now().Add(-time.Hour),
			ExtractFullArticle: true,
			ExtractionStrategy: types.StrategySelector,
			CSSSelector:        "invalid-selector", // Will fail to match
		}
		if err := repo.CreateFeed(ctx, feed); err != nil {
			t.Fatalf("failed to create feed: %v", err)
		}
		if err := repo.Subscribe(ctx, u.ID, feed.ID); err != nil {
			t.Fatalf("failed to subscribe: %v", err)
		}

		s.processFeed(ctx, feed)

		items, err := repo.ListPendingOutboxItems(ctx, time.Now().Add(time.Second))
		if err != nil {
			t.Fatalf("failed to list: %v", err)
		}
		// Expecting a new item from this feed too
		var fallbackItem *types.OutboxItem
		for _, it := range items {
			if strings.Contains(it.Subject, "Fallback Feed") {
				fallbackItem = it
				break
			}
		}
		if fallbackItem == nil {
			t.Fatalf("failed to find enqueued email for fallback feed")
		}

		// Should fall back to the feed summary content
		if !strings.Contains(fallbackItem.Body, "Summary content of Article 1") {
			t.Errorf("expected fallback content in body, got %q", fallbackItem.Body)
		}
	})
}

func TestSchedulerNotModified(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	ctrl := makeMockServer(t)
	defer ctrl.server.Close()

	cr := crawler.NewCrawler(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	ex := extractor.NewExtractor(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	sa := sanitizer.NewSanitizer(600)
	s := New(repo, cr, ex, sa, Config{}, nil)

	feed := &types.Feed{
		Title:            "Mock Feed",
		URL:              ctrl.server.URL + "/feed.xml",
		PollIntervalSecs: 60,
		BackoffFactor:    1.0,
		NextPollAt:       time.Now().Add(-time.Hour),
		ETag:             "old-etag",
	}
	if err := repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	// Trigger 304 response
	ctrl.mu.Lock()
	ctrl.notModified = true
	ctrl.mu.Unlock()

	s.processFeed(ctx, feed)

	updatedFeed, err := repo.GetFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("failed to fetch updated feed: %v", err)
	}
	if updatedFeed.ETag != "old-etag" {
		t.Errorf("expected ETag to remain old-etag, got %q", updatedFeed.ETag)
	}
	diff := time.Until(updatedFeed.NextPollAt)
	expectedInterval := time.Duration(feed.PollIntervalSecs) * time.Second
	if diff < expectedInterval-10*time.Second || diff > expectedInterval+10*time.Second {
		t.Errorf("expected next poll time on NotModified to be around %v from now, got diff %v", expectedInterval, diff)
	}

	items, err := repo.ListPendingOutboxItems(ctx, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("failed to list outbox items: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 outbox items, got %d", len(items))
	}
}

func TestSchedulerCrawlErrorsAndBackoff(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	ctrl := makeMockServer(t)
	defer ctrl.server.Close()

	cr := crawler.NewCrawler(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	ex := extractor.NewExtractor(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	sa := sanitizer.NewSanitizer(600)
	s := New(repo, cr, ex, sa, Config{}, nil)

	feed := &types.Feed{
		Title:            "Mock Feed",
		URL:              ctrl.server.URL + "/feed.xml",
		PollIntervalSecs: 60,
		BackoffFactor:    1.0,
		NextPollAt:       time.Now().Add(-time.Hour),
	}
	if err := repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	// 1. Trigger failure (HTTP 500)
	ctrl.mu.Lock()
	ctrl.failCrawl = true
	ctrl.mu.Unlock()

	s.processFeed(ctx, feed)

	updatedFeed, err := repo.GetFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("failed to fetch updated feed: %v", err)
	}
	if updatedFeed.BackoffFactor != 1.5 {
		t.Errorf("expected backoff factor to grow to 1.5, got %v", updatedFeed.BackoffFactor)
	}
	if updatedFeed.LastErrorStr == "" {
		t.Errorf("expected last error string to be non-empty")
	}
	if updatedFeed.LastErrorTime == nil {
		t.Errorf("expected last error time to be set")
	}
	diff := time.Until(updatedFeed.NextPollAt)
	expectedBackoff := time.Duration(60.0*1.5) * time.Second
	if diff < expectedBackoff-10*time.Second || diff > expectedBackoff+10*time.Second {
		t.Errorf("expected next poll time on failure to be around %v from now, got diff %v", expectedBackoff, diff)
	}

	// 2. Trigger 429 Retry-After rate limiting
	ctrl.mu.Lock()
	ctrl.failCrawl = false
	ctrl.rateLimit = true
	ctrl.mu.Unlock()

	s.processFeed(ctx, updatedFeed)

	rateLimitedFeed, err := repo.GetFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("failed to fetch updated feed: %v", err)
	}
	if rateLimitedFeed.BackoffFactor != 2.25 {
		t.Errorf("expected backoff factor to grow to 2.25, got %v", rateLimitedFeed.BackoffFactor)
	}

	diff = time.Until(rateLimitedFeed.NextPollAt)
	expectedLimitBackoff := time.Duration(60.0 * 2.25 * float64(time.Second)) // 135s (expBackoff 135s > Retry-After 120s)
	if diff < expectedLimitBackoff-10*time.Second || diff > expectedLimitBackoff+10*time.Second {
		t.Errorf("expected next poll time to be ~135s from now, got diff %v", diff)
	}

	// 3. Trigger 429 Retry-After rate limiting but with a very small Retry-After (10s)
	// Since the backoff factor grew to 3.375, the exponential backoff is 60 * 3.375 = 202.5s.
	// This is larger than 10s, so it must wait 202.5s (exponential backoff takes precedence).
	ctrl.mu.Lock()
	ctrl.retryAfterVal = "10"
	ctrl.mu.Unlock()

	s.processFeed(ctx, rateLimitedFeed)

	smallRetryFeed, err := repo.GetFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("failed to fetch updated feed: %v", err)
	}
	if smallRetryFeed.BackoffFactor != 3.375 {
		t.Errorf("expected backoff factor to grow to 3.375, got %v", smallRetryFeed.BackoffFactor)
	}

	diff = time.Until(smallRetryFeed.NextPollAt)
	expectedMin := time.Duration(60.0 * 3.375 * float64(time.Second))
	if diff < expectedMin-10*time.Second || diff > expectedMin+10*time.Second {
		t.Errorf("expected next poll time to respect exponential backoff (min %v), got diff %v", expectedMin, diff)
	}
}

func TestSchedulerNoSubscribers(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	ctrl := makeMockServer(t)
	defer ctrl.server.Close()

	cr := crawler.NewCrawler(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	ex := extractor.NewExtractor(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	sa := sanitizer.NewSanitizer(600)
	s := New(repo, cr, ex, sa, Config{}, nil)

	feed := &types.Feed{
		Title:            "Mock Feed",
		URL:              ctrl.server.URL + "/feed.xml",
		PollIntervalSecs: 60,
		BackoffFactor:    1.0,
		NextPollAt:       time.Now().Add(-time.Hour),
	}
	if err := repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	s.processFeed(ctx, feed)

	items, err := repo.ListPendingOutboxItems(ctx, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("failed to list outbox items: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 outbox items, got %d", len(items))
	}

	seen, err := repo.IsItemSeen(ctx, feed.ID, "guid-1")
	if err != nil {
		t.Fatalf("failed to check seen: %v", err)
	}
	if !seen {
		t.Errorf("expected item to be marked seen directly")
	}
}

func TestSchedulerStartStop(t *testing.T) {
	repo := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())

	cr := crawler.NewCrawler(nil, slog.New(slog.DiscardHandler))
	ex := extractor.NewExtractor(nil, slog.New(slog.DiscardHandler))
	sa := sanitizer.NewSanitizer(600)
	s := New(repo, cr, ex, sa, Config{
		PollInterval: 2 * time.Millisecond,
		MaxWorkers:   1,
	}, nil)

	var runErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		runErr = s.Start(ctx)
	})

	time.Sleep(10 * time.Millisecond)
	cancel()
	wg.Wait()

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		t.Errorf("expected context.Canceled error or nil, got: %v", runErr)
	}
}

type erroringDBTX struct {
	database.DBTX
	failList   bool
	failUpdate bool
}

func (e *erroringDBTX) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if e.failList && strings.Contains(query, "FROM feeds") {
		return nil, fmt.Errorf("forced feed query error")
	}
	return e.DBTX.QueryContext(ctx, query, args...)
}

func (e *erroringDBTX) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if e.failUpdate && (strings.Contains(query, "UPDATE feeds") || strings.Contains(query, "INSERT INTO seen_items")) {
		return nil, fmt.Errorf("forced update/seen error")
	}
	return e.DBTX.ExecContext(ctx, query, args...)
}

func TestSchedulerDBErrors(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	mockTX := &erroringDBTX{
		DBTX:     db,
		failList: true,
	}
	repo := database.NewRepository(mockTX)
	s := New(repo, nil, nil, nil, Config{}, nil)

	err = s.pollFeeds(context.Background())
	if err == nil {
		t.Error("expected pollFeeds to fail when database fails, got nil")
	}
}

type dynamicStdoutWriter struct{}

func (dynamicStdoutWriter) Write(p []byte) (n int, err error) {
	return os.Stdout.Write(p)
}

func testStdoutLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(dynamicStdoutWriter{}, &slog.HandlerOptions{Level: slog.LevelDebug})).With("component", "scheduler")
}

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return ""
	}
	os.Stdout = w

	outChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outChan <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = old
	return <-outChan
}

func TestSchedulerDBErrorsExtended(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctrl := makeMockServer(t)
	defer ctrl.server.Close()

	cr := crawler.NewCrawler(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	ex := extractor.NewExtractor(ctrl.server.Client(), slog.New(slog.DiscardHandler))
	sa := sanitizer.NewSanitizer(600)

	// Test UpdateFeed fails on crawl failure
	t.Run("UpdateFeed fails on crawl failure", func(t *testing.T) {
		mockTX := &erroringDBTX{DBTX: db, failUpdate: true}
		repo := database.NewRepository(mockTX)
		s := New(repo, cr, ex, sa, Config{}, testStdoutLogger())
		feed := &types.Feed{ID: 1, URL: ctrl.server.URL + "/feed.xml?err1=1", PollIntervalSecs: 60, BackoffFactor: 1.0}

		ctrl.mu.Lock()
		ctrl.failCrawl = true
		ctrl.mu.Unlock()

		output := captureStdout(func() {
			s.processFeed(context.Background(), feed)
		})

		if !strings.Contains(output, "Failed to update feed status on crawl failure") {
			t.Errorf("expected crawl failure update feed error log, got: %q", output)
		}
	})

	// Test UpdateFeed fails on NotModified
	t.Run("UpdateFeed fails on NotModified", func(t *testing.T) {
		mockTX := &erroringDBTX{DBTX: db, failUpdate: true}
		repo := database.NewRepository(mockTX)
		s := New(repo, cr, ex, sa, Config{}, testStdoutLogger())
		feed := &types.Feed{ID: 2, URL: ctrl.server.URL + "/feed.xml?err2=1", PollIntervalSecs: 60, BackoffFactor: 1.0}

		ctrl.mu.Lock()
		ctrl.failCrawl = false
		ctrl.notModified = true
		ctrl.mu.Unlock()

		output := captureStdout(func() {
			s.processFeed(context.Background(), feed)
		})

		if !strings.Contains(output, "Failed to update feed status on NotModified") {
			t.Errorf("expected NotModified update feed error log, got: %q", output)
		}
	})

	// Test UpdateFeed fails on success path
	t.Run("UpdateFeed fails on success", func(t *testing.T) {
		mockTX := &erroringDBTX{DBTX: db, failUpdate: true}
		repo := database.NewRepository(mockTX)
		s := New(repo, cr, ex, sa, Config{}, testStdoutLogger())
		feed := &types.Feed{ID: 3, URL: ctrl.server.URL + "/feed.xml?err3=1", PollIntervalSecs: 60, BackoffFactor: 1.0}

		ctrl.mu.Lock()
		ctrl.notModified = false
		ctrl.mu.Unlock()

		output := captureStdout(func() {
			s.processFeed(context.Background(), feed)
		})

		if !strings.Contains(output, "Failed to update feed cache markers") {
			t.Errorf("expected success update feed error log, got: %q", output)
		}
	})

	// Test MarkItemSeen fails when no subscribers
	t.Run("MarkItemSeen fails when no subscribers", func(t *testing.T) {
		mockTX := &erroringDBTX{DBTX: db, failUpdate: true}
		repo := database.NewRepository(mockTX)
		s := New(repo, cr, ex, sa, Config{}, testStdoutLogger())
		feed := &types.Feed{ID: 4, URL: ctrl.server.URL + "/feed.xml?err4=1", PollIntervalSecs: 60, BackoffFactor: 1.0}

		output := captureStdout(func() {
			s.processFeed(context.Background(), feed)
		})

		if !strings.Contains(output, "Failed to mark item seen with 0 subscribers") {
			t.Errorf("expected mark seen error log, got: %q", output)
		}
	})

	// Test transaction fails when enqueuing email
	t.Run("transaction fails on WithTx due to type assertion", func(t *testing.T) {
		mockTX := &erroringDBTX{DBTX: db}
		cleanRepo := database.NewRepository(db)
		u := &types.User{Email: "txfail@test.com"}
		_ = cleanRepo.CreateUser(context.Background(), u)
		feed := &types.Feed{Title: "Tx Fail Feed", URL: ctrl.server.URL + "/feed.xml?err5=1", PollIntervalSecs: 60, BackoffFactor: 1.0}
		_ = cleanRepo.CreateFeed(context.Background(), feed)
		_ = cleanRepo.Subscribe(context.Background(), u.ID, feed.ID)

		mockRepo := database.NewRepository(mockTX)
		s := New(mockRepo, cr, ex, sa, Config{}, testStdoutLogger())

		output := captureStdout(func() {
			s.processFeed(context.Background(), feed)
		})

		if !strings.Contains(output, "Failed to queue notification and mark seen") {
			t.Errorf("expected WithTx assertion fail error log, got: %q", output)
		}
	})
}

func TestSchedulerDefaultConfigFallback(t *testing.T) {
	s := New(nil, nil, nil, nil, Config{}, nil)
	if s.cfg.PollInterval != 10*time.Second {
		t.Errorf("expected default poll interval 10s, got %v", s.cfg.PollInterval)
	}
	if s.cfg.MaxWorkers != 10 {
		t.Errorf("expected default max workers 10, got %d", s.cfg.MaxWorkers)
	}
}

func TestSchedulerStartPollErrors(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	mockTX := &erroringDBTX{
		DBTX:     db,
		failList: true,
	}
	repo := database.NewRepository(mockTX)
	s := New(repo, nil, nil, nil, Config{
		PollInterval: 1 * time.Millisecond,
	}, testStdoutLogger())

	output := captureStdout(func() {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Go(func() {
			_ = s.Start(ctx)
		})

		// Wait for ticker to run at least once
		time.Sleep(5 * time.Millisecond)
		cancel()
		wg.Wait()
	})

	if !strings.Contains(output, "Initial poll error") {
		t.Errorf("expected initial poll error log, got: %q", output)
	}
	if !strings.Contains(output, "Poll error") {
		t.Errorf("expected poll error log, got: %q", output)
	}
}
