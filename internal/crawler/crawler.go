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
	FeedID int64
	Body   []byte
	Error  error
}

// Pool is a pool of workers for fetching RSS feeds.
type Pool struct {
	requests  chan CrawlRequest
	responses chan CrawlResponse
	client    *http.Client
	logger    *slog.Logger
}

// NewPool creates a new crawler pool.
func NewPool(size int, timeout time.Duration, logger *slog.Logger) *Pool {
	p := &Pool{
		requests:  make(chan CrawlRequest, size*2),
		responses: make(chan CrawlResponse, size*2),
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger.With("component", "crawler"),
	}

	for i := 0; i < size; i++ {
		go p.worker(i)
	}

	return p
}

func (p *Pool) worker(id int) {
	p.logger.Debug("starting worker", "worker_id", id)
	for req := range p.requests {
		p.logger.Debug("crawling feed", "feed_id", req.FeedID, "url", req.URL)
		body, err := p.fetch(req.Ctx, req.URL)
		p.responses <- CrawlResponse{
			FeedID: req.FeedID,
			Body:   body,
			Error:  err,
		}
	}
}

func (p *Pool) fetch(ctx context.Context, url string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "rss2go/2.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, text/xml;q=0.9, */*;q=0.8")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
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
