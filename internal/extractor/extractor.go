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

// SanitizeURL strips Basic Auth credentials (user:pass) and query/fragment parameters from raw URLs for safe logging.
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rawURL
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// Extract fetches the page at targetURL and extracts the content based on strategy.
func (e *Extractor) Extract(ctx context.Context, targetURL string, strategy types.ExtractionStrategy, selector string) (string, error) {
	log := e.log
	if log == nil {
		log = slog.Default().With("component", "extractor")
	}

	safeURL := SanitizeURL(targetURL)
	log.Debug("Starting article extraction", "url", safeURL, "strategy", strategy, "selector", selector)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		log.Debug("Failed creating HTTP request for article extraction", "url", safeURL, "err", err)
		return "", fmt.Errorf("extractor: create request: %w", err)
	}

	req.Header.Set("User-Agent", "rss2go/1.0 (Full Article Extractor)")

	start := time.Now()
	resp, err := e.client.Do(req)
	duration := time.Since(start)
	if err != nil {
		log.Debug("Article fetch failed", "url", safeURL, "duration", duration, "err", err)
		return "", fmt.Errorf("extractor: fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		log.Debug("Article fetch non-200 HTTP status", "url", safeURL, "status", resp.StatusCode)
		return "", fmt.Errorf("extractor: fetch returned HTTP status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml+xml") {
		log.Debug("Article content-type unsupported for extraction", "url", safeURL, "content_type", contentType)
		return "", fmt.Errorf("extractor: unsupported content type %q", contentType)
	}

	return e.ExtractFromReader(resp.Body, targetURL, strategy, selector)
}

// ExtractFromReader extracts content from an HTML reader.
func (e *Extractor) ExtractFromReader(r io.Reader, targetURL string, strategy types.ExtractionStrategy, selector string) (string, error) {
	log := e.log
	if log == nil {
		log = slog.Default().With("component", "extractor")
	}

	safeURL := SanitizeURL(targetURL)

	switch strategy {
	case types.StrategyHeuristic:
		parsedURL, err := url.Parse(targetURL)
		if err != nil {
			log.Debug("Invalid target URL for readability", "url", safeURL, "err", err)
			return "", fmt.Errorf("extractor: invalid target URL: %w", err)
		}
		article, err := readability.FromReader(r, parsedURL)
		if err != nil {
			log.Debug("Readability extraction failed", "url", safeURL, "err", err)
			return "", fmt.Errorf("extractor: readability extraction failed: %w", err)
		}
		var buf bytes.Buffer
		if err := article.RenderHTML(&buf); err != nil {
			log.Debug("Readability HTML render failed", "url", safeURL, "err", err)
			return "", fmt.Errorf("extractor: render article HTML: %w", err)
		}
		extracted := buf.String()
		log.Debug("Readability extraction succeeded", "url", safeURL, "bytes", len(extracted))
		return extracted, nil

	case types.StrategySelector:
		if selector == "" {
			log.Debug("CSS selector empty for selector strategy", "url", safeURL)
			return "", fmt.Errorf("extractor: structural selector strategy requires a non-empty CSS selector")
		}
		doc, err := goquery.NewDocumentFromReader(r)
		if err != nil {
			log.Debug("Failed parsing HTML document for selector extraction", "url", safeURL, "err", err)
			return "", fmt.Errorf("extractor: parse HTML document: %w", err)
		}

		selection := doc.Find(selector)
		if selection.Length() == 0 {
			log.Debug("CSS selector matched no elements", "url", safeURL, "selector", selector)
			return "", fmt.Errorf("extractor: CSS selector %q matched no elements", selector)
		}

		// Return the HTML of the matched elements.
		html, err := selection.Html()
		if err != nil {
			log.Debug("Failed rendering CSS selector HTML match", "url", safeURL, "selector", selector, "err", err)
			return "", fmt.Errorf("extractor: render matched HTML: %w", err)
		}
		log.Debug("CSS selector extraction succeeded", "url", safeURL, "selector", selector, "bytes", len(html))
		return html, nil

	default:
		log.Debug("Unsupported extraction strategy", "url", safeURL, "strategy", strategy)
		return "", fmt.Errorf("extractor: unsupported extraction strategy %q", strategy)
	}
}
