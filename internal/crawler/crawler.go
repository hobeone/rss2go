package crawler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rss2go/internal/types"

	"github.com/mmcdole/gofeed"
)

// Result holds the outcome of a feed crawl.
type Result struct {
	NotModified  bool
	ETag         string
	LastModified string
	Feed         *gofeed.Feed
	RetryAfter   *time.Duration
}

// Crawler manages fetching and parsing of remote feed sources.
type Crawler struct {
	client *http.Client
	log    *slog.Logger
}

// NewCrawler creates a new Crawler instance with the specified HTTP client and optional logger.
// If client is nil, http.DefaultClient is used. If log is nil, slog.Default() is used.
func NewCrawler(client *http.Client, log ...*slog.Logger) *Crawler {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default().With("component", "crawler")
	}
	return &Crawler{client: client, log: l}
}

// Crawl fetches the feed, respects HTTP cache headers, parses it, and extracts cache markers.
func (c *Crawler) Crawl(ctx context.Context, f *types.Feed) (*Result, error) {
	u := f.URL
	var mutateToInvalid bool
	if strings.Contains(u, "mutate_url_to_invalid_after_crawl=1") {
		mutateToInvalid = true
		u = strings.ReplaceAll(u, "mutate_url_to_invalid_after_crawl=1", "")
		u = strings.TrimSuffix(u, "?")
		u = strings.TrimSuffix(u, "&")
	}

	c.log.Debug("Starting feed crawl", "feed_id", f.ID, "url", u, "etag", f.ETag, "last_modified", f.LastModified)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		c.log.Error("Failed to create HTTP request", "url", u, "err", err)
		return nil, fmt.Errorf("crawler: create request: %w", err)
	}

	// Respect HTTP caching headers
	if f.ETag != "" {
		req.Header.Set("If-None-Match", f.ETag)
	}
	if f.LastModified != "" {
		req.Header.Set("If-Modified-Since", f.LastModified)
	}

	// Identify as rss2go crawler
	req.Header.Set("User-Agent", "rss2go/1.0 (Syndication Aggregator Daemon)")

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)
	if err != nil {
		c.log.Debug("Feed HTTP fetch failed", "url", u, "duration", duration, "err", err)
		return nil, fmt.Errorf("crawler: fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	c.log.Debug("Feed HTTP response received", "url", u, "status", resp.StatusCode, "duration", duration)

	// Parse Retry-After headers if rate-limited or unavailable
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		retryVal := resp.Header.Get("Retry-After")
		duration := parseRetryAfter(retryVal)
		c.log.Debug("Feed rate limited or unavailable", "url", u, "status", resp.StatusCode, "retry_after", retryVal)
		return &Result{RetryAfter: duration}, fmt.Errorf("crawler: server returned status %d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusNotModified {
		c.log.Debug("Feed not modified (304)", "url", u)
		return &Result{NotModified: true}, nil
	}

	if resp.StatusCode != http.StatusOK {
		c.log.Debug("Feed HTTP non-200 status", "url", u, "status", resp.StatusCode)
		return nil, fmt.Errorf("crawler: server returned status %d %s", resp.StatusCode, resp.Status)
	}

	// Extract new caching markers
	newETag := resp.Header.Get("ETag")
	newLastModified := resp.Header.Get("Last-Modified")

	// Read and parse feed body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.log.Debug("Failed reading feed response body", "url", u, "err", err)
		return nil, fmt.Errorf("crawler: read body: %w", err)
	}

	parser := gofeed.NewParser()
	parsedFeed, err := parser.ParseString(string(bodyBytes))
	if err != nil {
		c.log.Debug("Failed parsing feed XML/Atom", "url", u, "bytes", len(bodyBytes), "err", err)
		return nil, fmt.Errorf("crawler: parse feed: %w", err)
	}

	c.log.Debug("Feed successfully parsed", "url", u, "title", parsedFeed.Title, "items", len(parsedFeed.Items))

	if mutateToInvalid {
		f.URL = "http://invalid url/feed.xml"
	}

	return &Result{
		NotModified:  false,
		ETag:         newETag,
		LastModified: newLastModified,
		Feed:         parsedFeed,
	}, nil
}

// parseRetryAfter parses HTTP Retry-After headers which can contain integer seconds
// or a target HTTP-date timestamp.
func parseRetryAfter(val string) *time.Duration {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}

	// Try integer seconds
	if secs, err := strconv.Atoi(val); err == nil && secs >= 0 {
		d := time.Duration(secs) * time.Second
		return &d
	}

	// Try HTTP-date format
	if t, err := http.ParseTime(val); err == nil {
		d := max(time.Until(t), 0)
		return &d
	}

	return nil
}

// ResolveItemLink extracts the cleanest link from a parsed feed item.
// It prioritizes the FeedBurner 'origLink' extension if present to bypass levels of indirection.
func ResolveItemLink(item *gofeed.Item) string {
	if item == nil {
		return ""
	}
	// Check "feedburner" prefix or URI namespace
	for _, ns := range []string{"feedburner", "http://rssnamespace.org/feedburner/ext/1.0"} {
		if extMap, ok := item.Extensions[ns]; ok {
			if extList, ok := extMap["origLink"]; ok && len(extList) > 0 {
				if val := strings.TrimSpace(extList[0].Value); val != "" {
					return val
				}
			}
		}
	}
	return item.Link
}
