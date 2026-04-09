package crawler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// MaxResponseBodySize is the maximum size of a feed response body (10MB).
	MaxResponseBodySize = 10 * 1024 * 1024
)

// RequestType defines the type of crawl request.
type RequestType int

const (
	RequestTypeFeed RequestType = iota
	RequestTypeItem
)

// CrawlRequest represents a request to fetch a feed or an item.
type CrawlRequest struct {
	FeedID       int64
	URL          string
	Type         RequestType
	ItemGUID     string // To correlate with the original item
	Ctx          context.Context
	ETag         string
	LastModified string
}

// CrawlResponse represents the result of a fetch.
type CrawlResponse struct {
	FeedID       int64
	Type         RequestType
	ItemGUID     string
	StatusCode   int
	Body         []byte
	Error        error
	ETag         string
	LastModified string
	RetryAfter   time.Duration // non-zero when the server sent a Retry-After header (429/503)
}

// Pool is a pool of workers for fetching RSS feeds.
type Pool struct {
	requests  chan CrawlRequest
	responses chan CrawlResponse
	client    *http.Client
	timeout   time.Duration
	logger    *slog.Logger
	wg        sync.WaitGroup
}

// NewPool creates a new crawler pool.
func NewPool(size int, timeout time.Duration, logger *slog.Logger) *Pool {
	p := &Pool{
		requests:  make(chan CrawlRequest, size*2),
		responses: make(chan CrawlResponse, size*2),
		client:    &http.Client{},
		timeout:   timeout,
		logger:    logger.With("component", "crawler"),
	}

	for i := range size {
		p.wg.Add(1)
		go p.worker(i)
	}

	return p
}

func (p *Pool) worker(id int) {
	defer p.wg.Done()
	p.logger.Debug("starting worker", "worker_id", id)
	for req := range p.requests {
		p.logger.Debug("crawling request", "feed_id", req.FeedID, "type", req.Type, "url", req.URL)
		body, code, etag, lastModified, retryAfter, err := p.fetch(req.Ctx, req.URL, req.ETag, req.LastModified)
		p.responses <- CrawlResponse{
			FeedID:       req.FeedID,
			Type:         req.Type,
			ItemGUID:     req.ItemGUID,
			StatusCode:   code,
			Body:         body,
			Error:        err,
			ETag:         etag,
			LastModified: lastModified,
			RetryAfter:   retryAfter,
		}
	}
}

func (p *Pool) fetch(ctx context.Context, url string, etag string, lastModified string) ([]byte, int, string, string, time.Duration, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Create a sub-context with the pool's configured timeout
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, "", "", 0, err
	}

	req.Header.Set("User-Agent", "rss2go/2.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, text/xml;q=0.9, */*;q=0.8")

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	p.logger.Debug("sending request", "url", url, "timeout", p.timeout)
	start := time.Now()
	// #nosec G704 -- intentional SSRF for crawling RSS feeds
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			p.logger.Debug("request timed out", "url", url, "duration", time.Since(start))
		} else {
			p.logger.Debug("request failed", "url", url, "error", err, "duration", time.Since(start))
		}
		return nil, 0, "", "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	duration := time.Since(start)
	p.logger.Debug("response received",
		"url", url,
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"duration", duration,
	)

	if resp.StatusCode == http.StatusNotModified {
		p.logger.Debug("feed not modified", "url", url)
		return nil, resp.StatusCode, "", "", 0, nil
	}

	// Parse Retry-After on rate-limit and service-unavailable responses.
	var retryAfter time.Duration
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
		if retryAfter > 0 {
			p.logger.Warn("rate limited by server", "url", url, "retry_after", retryAfter)
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize+1))
	if err != nil {
		return nil, resp.StatusCode, "", "", retryAfter, err
	}

	if len(body) > MaxResponseBodySize {
		p.logger.Error("response body exceeded limit", "url", url, "limit", MaxResponseBodySize)
		body = body[:MaxResponseBodySize]
	}

	if resp.StatusCode != http.StatusOK {
		return body, resp.StatusCode, "", "", retryAfter, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	p.logger.Debug("read complete", "url", url, "bytes", len(body))
	return body, resp.StatusCode, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), 0, nil
}

// parseRetryAfter parses the value of a Retry-After HTTP header, which may be
// either an integer number of seconds or an HTTP-date (RFC 7231 §7.1.3).
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	// Integer seconds form: "Retry-After: 120"
	if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil {
		if secs > 0 {
			return time.Duration(secs) * time.Second
		}
		return 0
	}
	// HTTP-date form: "Retry-After: Fri, 01 Jan 2027 00:00:00 GMT"
	if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// Submit sends a crawl request to the pool.
func (p *Pool) Submit(req CrawlRequest) {
	p.requests <- req
}

// Responses returns the channel where crawl results are sent.
func (p *Pool) Responses() <-chan CrawlResponse {
	return p.responses
}

// Close closes the pool.
func (p *Pool) Close() {
	close(p.requests)
	p.wg.Wait()
	close(p.responses)
}
