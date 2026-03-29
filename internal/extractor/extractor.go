package extractor

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"time"

	"codeberg.org/readeck/go-readability/v2"
)

// Extract pulls the main content from an HTML document.
func Extract(html io.Reader, pageURL string, timeout time.Duration) (string, error) {
	parsedURL, err := url.Parse(pageURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	article, err := readability.FromReader(html, parsedURL)
	if err != nil {
		return "", fmt.Errorf("readability extraction failed: %w", err)
	}

	var buf bytes.Buffer
	if err := article.RenderHTML(&buf); err != nil {
		return "", fmt.Errorf("failed to render article HTML: %w", err)
	}

	return buf.String(), nil
}
