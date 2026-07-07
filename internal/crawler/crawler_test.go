package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"rss2go/internal/types"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
)

const sampleRSS = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed Title</title>
    <link>https://example.com</link>
    <description>Test feed description</description>
    <item>
      <title>Item Title</title>
      <link>https://example.com/item1</link>
      <description>Item body description</description>
      <guid>guid-12345</guid>
    </item>
  </channel>
</rss>`

func TestCrawlHappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"etag-value"`)
		w.Header().Set("Last-Modified", "Wed, 24 Jun 2026 12:00:00 GMT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleRSS))
	}))
	defer server.Close()

	c := NewCrawler(nil)
	feed := &types.Feed{
		URL: server.URL,
	}

	res, err := c.Crawl(context.Background(), feed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.NotModified {
		t.Error("expected NotModified to be false")
	}
	if res.ETag != `"etag-value"` {
		t.Errorf("expected ETag to be \"etag-value\", got %q", res.ETag)
	}
	if res.LastModified != "Wed, 24 Jun 2026 12:00:00 GMT" {
		t.Errorf("expected LastModified to be match, got %q", res.LastModified)
	}
	if res.Feed == nil || res.Feed.Title != "Test Feed Title" {
		t.Errorf("feed parsing failed: %+v", res.Feed)
	}
	if len(res.Feed.Items) != 1 || res.Feed.Items[0].Title != "Item Title" || res.Feed.Items[0].GUID != "guid-12345" {
		t.Errorf("feed item mismatch: %+v", res.Feed.Items)
	}
}

func TestCrawlNotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers are propagated correctly
		if r.Header.Get("If-None-Match") != `"etag-123"` {
			t.Errorf("expected If-None-Match to be \"etag-123\", got %q", r.Header.Get("If-None-Match"))
		}
		if r.Header.Get("If-Modified-Since") != "Wed, 24 Jun 2026 12:00:00 GMT" {
			t.Errorf("expected If-Modified-Since to match, got %q", r.Header.Get("If-Modified-Since"))
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	c := NewCrawler(nil)
	feed := &types.Feed{
		URL:          server.URL,
		ETag:         `"etag-123"`,
		LastModified: "Wed, 24 Jun 2026 12:00:00 GMT",
	}

	res, err := c.Crawl(context.Background(), feed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.NotModified {
		t.Error("expected NotModified to be true")
	}
	if res.Feed != nil {
		t.Error("expected feed parsing result to be nil on 304")
	}
}

func TestCrawlRetryAfterSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c := NewCrawler(nil)
	feed := &types.Feed{URL: server.URL}

	res, err := c.Crawl(context.Background(), feed)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if res == nil || res.RetryAfter == nil || *res.RetryAfter != 120*time.Second {
		t.Errorf("expected Retry-After to be 120s, got: %+v", res)
	}
}

func TestCrawlRetryAfterDate(t *testing.T) {
	targetTime := time.Now().Add(5 * time.Minute).Round(time.Second).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", targetTime.Format(http.TimeFormat))
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := NewCrawler(nil)
	feed := &types.Feed{URL: server.URL}

	res, err := c.Crawl(context.Background(), feed)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if res == nil || res.RetryAfter == nil {
		t.Fatalf("expected RetryAfter to be returned, got nil")
	}

	// Should be around 5 minutes (allow 5 seconds skew)
	diff := *res.RetryAfter - 5*time.Minute
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("expected RetryAfter around 5m, got: %v", *res.RetryAfter)
	}
}

func TestCrawlErrorConditions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewCrawler(nil)
	feed := &types.Feed{URL: server.URL}

	_, err := c.Crawl(context.Background(), feed)
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}

	// Non-existent URL error
	badFeed := &types.Feed{URL: "http://non-existent-local-url.local"}
	_, err = c.Crawl(context.Background(), badFeed)
	if err == nil {
		t.Fatal("expected network failure error, got nil")
	}

	// Invalid XML parsing error
	serverBadXML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid xml content"))
	}))
	defer serverBadXML.Close()

	feedBadXML := &types.Feed{URL: serverBadXML.URL}
	_, err = c.Crawl(context.Background(), feedBadXML)
	if err == nil {
		t.Fatal("expected parsing error, got nil")
	}
}

func TestParseRetryAfterEdgeCases(t *testing.T) {
	// Empty string
	if d := parseRetryAfter(""); d != nil {
		t.Errorf("expected nil for empty Retry-After, got %v", d)
	}

	// Zero value
	if d := parseRetryAfter("0"); d == nil || *d != 0 {
		t.Errorf("expected 0 duration for 0 value, got %v", d)
	}

	// Negative value
	if d := parseRetryAfter("-10"); d != nil {
		t.Errorf("expected nil for negative value, got %v", d)
	}

	// Invalid string
	if d := parseRetryAfter("invalid-value"); d != nil {
		t.Errorf("expected nil for invalid string, got %v", d)
	}

	// Past date format
	pastTime := time.Now().Add(-10 * time.Minute).Format(http.TimeFormat)
	if d := parseRetryAfter(pastTime); d == nil || *d != 0 {
		t.Errorf("expected 0 duration for past date, got %v", d)
	}
}

func TestResolveItemLink(t *testing.T) {
	// 1. Standard item without extensions.
	itemNormal := &gofeed.Item{
		Link: "http://example.com/normal",
	}
	if link := ResolveItemLink(itemNormal); link != "http://example.com/normal" {
		t.Errorf("expected http://example.com/normal, got %q", link)
	}

	// 2. Item with feedburner:origLink extension (short name).
	itemFBShort := &gofeed.Item{
		Link: "http://feedburner.com/redirect",
		Extensions: map[string]map[string][]ext.Extension{
			"feedburner": {
				"origLink": {
					{Value: "http://example.com/original-short"},
				},
			},
		},
	}
	if link := ResolveItemLink(itemFBShort); link != "http://example.com/original-short" {
		t.Errorf("expected http://example.com/original-short, got %q", link)
	}

	// 3. Item with feedburner:origLink extension (namespace URI).
	itemFBURI := &gofeed.Item{
		Link: "http://feedburner.com/redirect",
		Extensions: map[string]map[string][]ext.Extension{
			"http://rssnamespace.org/feedburner/ext/1.0": {
				"origLink": {
					{Value: "http://example.com/original-uri"},
				},
			},
		},
	}
	if link := ResolveItemLink(itemFBURI); link != "http://example.com/original-uri" {
		t.Errorf("expected http://example.com/original-uri, got %q", link)
	}

	// 4. Nil item.
	if link := ResolveItemLink(nil); link != "" {
		t.Errorf("expected empty string for nil item, got %q", link)
	}
}
