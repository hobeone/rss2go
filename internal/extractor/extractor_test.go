package extractor

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

var discardLogger = slog.New(slog.DiscardHandler)

func TestExtract(t *testing.T) {
	html := `
<html>
<head><title>Test Article</title></head>
<body>
<h1>Main Title</h1>
<div class="content">
<p>This is the main content of the article.</p>
<p>More interesting text here.</p>
</div>
<div class="footer">Ads and stuff</div>
</body>
</html>
`
	ext, err := New(StrategyReadability, "")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	content, err := ext.Extract(strings.NewReader(html), "http://example.com/article", 5*time.Second, discardLogger)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if !strings.Contains(content, "This is the main content") {
		t.Errorf("extracted content missing main text, got: %s", content)
	}
	if strings.Contains(content, "Ads and stuff") {
		t.Errorf("extracted content should not contain footer ads, got: %s", content)
	}
}

func TestExtract_InvalidURL(t *testing.T) {
	ext, err := New(StrategyReadability, "")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	_, err = ext.Extract(strings.NewReader("<html></html>"), "::invalid-url::", 5*time.Second, discardLogger)
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

func TestNew_UnknownStrategy(t *testing.T) {
	_, err := New("magic", "")
	if err == nil {
		t.Error("expected error for unknown strategy, got nil")
	}
}

func TestNew_SelectorMissingConfig(t *testing.T) {
	_, err := New(StrategySelector, "")
	if err == nil {
		t.Error("expected error for selector strategy with empty config, got nil")
	}
}

func TestSelectorExtractor(t *testing.T) {
	html := `
<html><body>
<div class="sidebar">Noise</div>
<article class="post">
<p>This is the article body.</p>
</article>
</body></html>
`
	ext, err := New(StrategySelector, "article.post")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	content, err := ext.Extract(strings.NewReader(html), "http://example.com/", 5*time.Second, discardLogger)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if !strings.Contains(content, "This is the article body") {
		t.Errorf("expected article body, got: %s", content)
	}
	if strings.Contains(content, "Noise") {
		t.Errorf("should not contain sidebar noise, got: %s", content)
	}
}

func TestSelectorExtractor_NoMatch(t *testing.T) {
	ext, err := New(StrategySelector, ".nonexistent")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	_, err = ext.Extract(strings.NewReader("<html><body><p>text</p></body></html>"), "http://example.com/", 5*time.Second, discardLogger)
	if err == nil {
		t.Error("expected error for non-matching selector, got nil")
	}
}
