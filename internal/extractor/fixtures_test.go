package extractor

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureMeta is the schema for testdata/<name>/meta.json.
type fixtureMeta struct {
	URL              string   `json:"url"`
	CapturedAt       string   `json:"captured_at"`
	Strategy         string   `json:"strategy"`
	Config           string   `json:"config"`
	ExpectedContains []string `json:"expected_contains"`
}

// TestFixtures loads every testdata/<name>/ directory, constructs the
// appropriate extractor, runs it against the saved article.html, and asserts
// that each expected_contains string appears in the output.
//
// To add a new fixture: run the capture tool
//
//	go run ./cmd/capture-fixture -url <URL> -name <name> [-strategy selector] [-config ".css-selector"]
//
// then verify the saved meta.json and article.html look correct.
func TestFixtures(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to read testdata dir: %v", err)
	}

	//logger := slog.New(slog.DiscardHandler)
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug, // Enable debug logs
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("testdata", name)

			// Load meta.json
			metaBytes, err := os.ReadFile(filepath.Join(dir, "meta.json"))
			if err != nil {
				t.Fatalf("missing meta.json: %v", err)
			}
			var meta fixtureMeta
			if err := json.Unmarshal(metaBytes, &meta); err != nil {
				t.Fatalf("invalid meta.json: %v", err)
			}

			// Load article.html
			htmlBytes, err := os.ReadFile(filepath.Join(dir, "article.html"))
			if err != nil {
				t.Fatalf("missing article.html: %v", err)
			}

			// Build extractor
			ext, err := New(meta.Strategy, meta.Config)
			if err != nil {
				t.Fatalf("failed to build extractor: %v", err)
			}

			content, err := ext.Extract(strings.NewReader(string(htmlBytes)), meta.URL, 5*time.Second, logger)
			if err != nil {
				t.Fatalf("extraction failed: %v", err)
			}

			if content == "" {
				t.Fatal("extraction returned empty content")
			}

			for _, want := range meta.ExpectedContains {
				if !strings.Contains(content, want) {
					t.Errorf("expected %q in extracted content\ngot:\n%s", want, content)
				}
			}
		})
	}
}
