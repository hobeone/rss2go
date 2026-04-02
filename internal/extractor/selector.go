package extractor

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// SelectorExtractor uses a CSS selector to extract a specific element from the page.
type SelectorExtractor struct {
	Selector string
}

func (e *SelectorExtractor) Extract(html io.Reader, pageURL string, timeout time.Duration, logger *slog.Logger) (string, error) {
	doc, err := goquery.NewDocumentFromReader(html)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	sel := doc.Find(e.Selector)
	if sel.Length() == 0 {
		return "", fmt.Errorf("selector %q matched no elements", e.Selector)
	}

	content, err := sel.First().Html()
	if err != nil {
		return "", fmt.Errorf("failed to render selected content: %w", err)
	}

	content = strings.TrimSpace(content)
	logger.Debug("selector extraction complete",
		"selector", e.Selector,
		"matched", sel.Length(),
		"bytes", len(content),
	)
	return content, nil
}
