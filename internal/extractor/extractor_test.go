package extractor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rss2go/internal/types"
)

const sampleArticleHTML = `
<!DOCTYPE html>
<html>
<head><title>Sample Article</title></head>
<body>
  <nav><ul><li>Home</li><li>Blog</li></ul></nav>
  <div class="sidebar">Ads Ads Ads</div>
  <article>
    <h1 class="title">My Awesome Article</h1>
    <div class="content">
      <p>This is the main readable content of the article.</p>
      <p>It should be extracted successfully.</p>
    </div>
  </article>
  <footer>Copyright 2026</footer>
</body>
</html>`

func TestExtractCSSSelector(t *testing.T) {
	e := NewExtractor(nil)

	// Happy Path
	r := strings.NewReader(sampleArticleHTML)
	res, err := e.ExtractFromReader(r, "https://example.com", types.StrategySelector, "article .content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(res, "This is the main readable content") {
		t.Errorf("expected extracted content to contain main text, got %q", res)
	}
	if strings.Contains(res, "Ads Ads Ads") || strings.Contains(res, "Home") {
		t.Error("extracted content contains elements outside the selector target")
	}

	// Error path: selector matches nothing
	r = strings.NewReader(sampleArticleHTML)
	_, err = e.ExtractFromReader(r, "https://example.com", types.StrategySelector, ".non-existent-class")
	if err == nil {
		t.Fatal("expected error for non-existent selector, got nil")
	}

	// Error path: empty selector
	r = strings.NewReader(sampleArticleHTML)
	_, err = e.ExtractFromReader(r, "https://example.com", types.StrategySelector, "")
	if err == nil {
		t.Fatal("expected error for empty selector, got nil")
	}
}

func TestExtractHeuristic(t *testing.T) {
	e := NewExtractor(nil)

	r := strings.NewReader(sampleArticleHTML)
	res, err := e.ExtractFromReader(r, "https://example.com/article", types.StrategyHeuristic, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reader Mode heuristic should capture the core text but drop nav/sidebar ads
	if !strings.Contains(res, "This is the main readable content") {
		t.Errorf("expected reader mode to capture main text, got %q", res)
	}
	if strings.Contains(res, "Ads Ads Ads") || strings.Contains(res, "Home") {
		t.Errorf("expected reader mode to strip nav/ads, got %q", res)
	}
}

func TestExtractUnsupportedStrategy(t *testing.T) {
	e := NewExtractor(nil)
	r := strings.NewReader(sampleArticleHTML)
	_, err := e.ExtractFromReader(r, "https://example.com", "unknown", "")
	if err == nil {
		t.Fatal("expected error for unsupported strategy, got nil")
	}
}

func TestExtractNetworkCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleArticleHTML))
	}))
	defer server.Close()

	e := NewExtractor(nil)

	// Happy path
	res, err := e.Extract(context.Background(), server.URL, types.StrategySelector, "article h1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res, "My Awesome Article") {
		t.Errorf("expected heading to match, got %q", res)
	}

	// Bad Content-Type error path
	serverBadType := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"hello": "world"}`))
	}))
	defer serverBadType.Close()

	_, err = e.Extract(context.Background(), serverBadType.URL, types.StrategyHeuristic, "")
	if err == nil {
		t.Fatal("expected error on non-HTML content type, got nil")
	}

	// HTTP 500 status error path
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server500.Close()

	_, err = e.Extract(context.Background(), server500.URL, types.StrategyHeuristic, "")
	if err == nil {
		t.Fatal("expected error on HTTP 500 status, got nil")
	}

	// Network failure path
	_, err = e.Extract(context.Background(), "http://non-existent-local-server.local/page", types.StrategyHeuristic, "")
	if err == nil {
		t.Fatal("expected network failure error, got nil")
	}
}
