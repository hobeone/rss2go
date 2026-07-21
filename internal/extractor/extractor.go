package extractor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rss2go/internal/types"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
)

// Extractor manages fetching remote destination articles and extracting their primary content.
type Extractor struct {
	client *http.Client
	log    *slog.Logger
}

// NewExtractor creates a new Extractor instance.
// If client is nil, http.DefaultClient is used. If log is nil, slog.Default() is used.
func NewExtractor(client *http.Client, log ...*slog.Logger) *Extractor {
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default().With("component", "extractor")
	}
	return &Extractor{client: client, log: l}
}

// Extract fetches the page at targetURL and extracts the content based on strategy.
func (e *Extractor) Extract(ctx context.Context, targetURL string, strategy types.ExtractionStrategy, selector string) (string, error) {
	e.log.Debug("Starting article extraction", "url", targetURL, "strategy", strategy, "selector", selector)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		e.log.Debug("Failed creating HTTP request for article extraction", "url", targetURL, "err", err)
		return "", fmt.Errorf("extractor: create request: %w", err)
	}

	req.Header.Set("User-Agent", "rss2go/1.0 (Full Article Extractor)")

	start := time.Now()
	resp, err := e.client.Do(req)
	duration := time.Since(start)
	if err != nil {
		e.log.Debug("Article fetch failed", "url", targetURL, "duration", duration, "err", err)
		return "", fmt.Errorf("extractor: fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		e.log.Debug("Article fetch non-200 HTTP status", "url", targetURL, "status", resp.StatusCode)
		return "", fmt.Errorf("extractor: fetch returned HTTP status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml+xml") {
		e.log.Debug("Article content-type unsupported for extraction", "url", targetURL, "content_type", contentType)
		return "", fmt.Errorf("extractor: unsupported content type %q", contentType)
	}

	return e.ExtractFromReader(resp.Body, targetURL, strategy, selector)
}

// ExtractFromReader extracts content from an HTML reader.
func (e *Extractor) ExtractFromReader(r io.Reader, targetURL string, strategy types.ExtractionStrategy, selector string) (string, error) {
	switch strategy {
	case types.StrategyHeuristic:
		parsedURL, err := url.Parse(targetURL)
		if err != nil {
			e.log.Debug("Invalid target URL for readability", "url", targetURL, "err", err)
			return "", fmt.Errorf("extractor: invalid target URL: %w", err)
		}
		article, err := readability.FromReader(r, parsedURL)
		if err != nil {
			e.log.Debug("Readability extraction failed", "url", targetURL, "err", err)
			return "", fmt.Errorf("extractor: readability extraction failed: %w", err)
		}
		var buf bytes.Buffer
		if err := article.RenderHTML(&buf); err != nil {
			e.log.Debug("Readability HTML render failed", "url", targetURL, "err", err)
			return "", fmt.Errorf("extractor: render article HTML: %w", err)
		}
		extracted := buf.String()
		e.log.Debug("Readability extraction succeeded", "url", targetURL, "bytes", len(extracted))
		return extracted, nil

	case types.StrategySelector:
		if selector == "" {
			e.log.Debug("CSS selector empty for selector strategy", "url", targetURL)
			return "", fmt.Errorf("extractor: structural selector strategy requires a non-empty CSS selector")
		}
		doc, err := goquery.NewDocumentFromReader(r)
		if err != nil {
			e.log.Debug("Failed parsing HTML document for selector extraction", "url", targetURL, "err", err)
			return "", fmt.Errorf("extractor: parse HTML document: %w", err)
		}

		selection := doc.Find(selector)
		if selection.Length() == 0 {
			e.log.Debug("CSS selector matched no elements", "url", targetURL, "selector", selector)
			return "", fmt.Errorf("extractor: CSS selector %q matched no elements", selector)
		}

		// Return the HTML of the matched elements.
		html, err := selection.Html()
		if err != nil {
			e.log.Debug("Failed rendering CSS selector HTML match", "url", targetURL, "selector", selector, "err", err)
			return "", fmt.Errorf("extractor: render matched HTML: %w", err)
		}
		e.log.Debug("CSS selector extraction succeeded", "url", targetURL, "selector", selector, "bytes", len(html))
		return html, nil

	default:
		e.log.Debug("Unsupported extraction strategy", "url", targetURL, "strategy", strategy)
		return "", fmt.Errorf("extractor: unsupported extraction strategy %q", strategy)
	}
}
