package extractor

import (
	"fmt"
	"io"
	"log/slog"
)

// Strategy constants for extraction methods.
const (
	StrategyReadability = "readability"
	StrategySelector    = "selector"
)

// Extractor pulls the main content from an HTML document.
type Extractor interface {
	Extract(html io.Reader, pageURL string, logger *slog.Logger) (string, error)
}

// New returns an Extractor for the given strategy and config.
// An empty strategy defaults to StrategyReadability.
func New(strategy, config string) (Extractor, error) {
	if strategy == "" {
		strategy = StrategyReadability
	}
	switch strategy {
	case StrategyReadability:
		return &ReadabilityExtractor{}, nil
	case StrategySelector:
		if config == "" {
			return nil, fmt.Errorf("selector strategy requires a CSS selector in extraction_config")
		}
		return &SelectorExtractor{Selector: config}, nil
	default:
		return nil, fmt.Errorf("unknown extraction strategy %q", strategy)
	}
}

