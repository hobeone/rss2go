package extractor

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
}

// NewExtractor creates a new Extractor instance.
// If client is nil, http.DefaultClient is used.
func NewExtractor(client *http.Client) *Extractor {
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	return &Extractor{client: client}
}

// Extract fetches the page at targetURL and extracts the content based on strategy.
func (e *Extractor) Extract(ctx context.Context, targetURL string, strategy types.ExtractionStrategy, selector string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("extractor: create request: %w", err)
	}

	req.Header.Set("User-Agent", "rss2go/1.0 (Full Article Extractor)")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("extractor: fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("extractor: fetch returned HTTP status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml+xml") {
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
			return "", fmt.Errorf("extractor: invalid target URL: %w", err)
		}
		article, err := readability.FromReader(r, parsedURL)
		if err != nil {
			return "", fmt.Errorf("extractor: readability extraction failed: %w", err)
		}
		var buf bytes.Buffer
		if err := article.RenderHTML(&buf); err != nil {
			return "", fmt.Errorf("extractor: render article HTML: %w", err)
		}
		return buf.String(), nil

	case types.StrategySelector:
		if selector == "" {
			return "", fmt.Errorf("extractor: structural selector strategy requires a non-empty CSS selector")
		}
		doc, err := goquery.NewDocumentFromReader(r)
		if err != nil {
			return "", fmt.Errorf("extractor: parse HTML document: %w", err)
		}

		selection := doc.Find(selector)
		if selection.Length() == 0 {
			return "", fmt.Errorf("extractor: CSS selector %q matched no elements", selector)
		}

		// Return the HTML of the matched elements.
		html, err := selection.Html()
		if err != nil {
			return "", fmt.Errorf("extractor: render matched HTML: %w", err)
		}
		return html, nil

	default:
		return "", fmt.Errorf("extractor: unsupported extraction strategy %q", strategy)
	}
}
