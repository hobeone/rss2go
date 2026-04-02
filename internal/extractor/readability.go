package extractor

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"time"

	"codeberg.org/readeck/go-readability/v2"
)

// ReadabilityExtractor uses the go-readability library to extract article content.
type ReadabilityExtractor struct{}

func (e *ReadabilityExtractor) Extract(html io.Reader, pageURL string, timeout time.Duration, logger *slog.Logger) (string, error) {
	parsedURL, err := parseURL(pageURL)
	if err != nil {
		return "", err
	}

	article, err := readability.FromReader(html, parsedURL)
	if err != nil {
		return "", fmt.Errorf("readability extraction failed: %w", err)
	}

	logger.Debug("readability extraction complete",
		"title", article.Title(),
		"byline", article.Byline(),
		"site_name", article.SiteName(),
		"language", article.Language(),
		"image_url", article.ImageURL(),
		"excerpt", truncate(article.Excerpt(), 200),
	)

	var buf bytes.Buffer
	if err := article.RenderHTML(&buf); err != nil {
		return "", fmt.Errorf("failed to render article HTML: %w", err)
	}

	logger.Debug("readability render complete", "bytes", buf.Len())
	return buf.String(), nil
}
