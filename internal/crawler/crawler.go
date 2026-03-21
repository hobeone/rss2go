package crawler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// CrawlRequest represents a request to fetch a feed.
type CrawlRequest struct {
	FeedID int64
	URL    string
	Ctx    context.Context
}

// CrawlResponse represents the result of a fetch.
type CrawlResponse struct {
	FeedID     int64
	StatusCode int
	Body       []byte
	Error      error
}

// Pool is a pool of workers for fetching RSS feeds.
type Pool struct {
	requests  chan CrawlRequest
	responses chan CrawlResponse
	client    *http.Client
	timeout   time.Duration
	logger    *slog.Logger
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
		go p.worker(i)
	}

	return p
}

func (p *Pool) worker(id int) {
	p.logger.Debug("starting worker", "worker_id", id)
	for req := range p.requests {
		p.logger.Debug("crawling feed", "feed_id", req.FeedID, "url", req.URL)
		body, code, err := p.fetch(req.Ctx, req.URL)
		p.responses <- CrawlResponse{
			FeedID:     req.FeedID,
			StatusCode: code,
			Body:       body,
			Error:      err,
		}
	}
}

func (p *Pool) fetch(ctx context.Context, url string) ([]byte, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Create a sub-context with the pool's configured timeout
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("User-Agent", "rss2go/2.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, text/xml;q=0.9, */*;q=0.8")

	p.logger.Debug("sending request", "url", url, "timeout", p.timeout)
	start := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			p.logger.Debug("request timed out", "url", url, "duration", time.Since(start))
		} else {
			p.logger.Debug("request failed", "url", url, "error", err, "duration", time.Since(start))
		}
		return nil, 0, err
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	p.logger.Debug("response received",
		"url", url,
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"duration", duration,
	)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode != http.StatusOK {
		return body, resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	p.logger.Debug("read complete", "url", url, "bytes", len(body))
	return body, resp.StatusCode, nil
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
}
