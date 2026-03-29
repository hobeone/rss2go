package extractor

import (
	"strings"
	"testing"
	"time"
)

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
	pageURL := "http://example.com/article"
	content, err := Extract(strings.NewReader(html), pageURL, 5*time.Second)
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
	_, err := Extract(strings.NewReader("<html></html>"), "::invalid-url::", 5*time.Second)
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}
