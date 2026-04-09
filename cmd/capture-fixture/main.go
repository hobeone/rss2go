// capture-fixture fetches a URL and saves the HTML + metadata into
// internal/extractor/testdata/<name>/ so it can be used as a repeatable
// test fixture for extractor strategy development.
//
// Usage:
//
//	go run ./cmd/capture-fixture -url https://example.com/article -name my_site
//	go run ./cmd/capture-fixture -url https://example.com/article -name my_site \
//	    -strategy selector -config "article.post-content" \
//	    -expect "some text that should appear in the extracted content"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type meta struct {
	URL              string   `json:"url"`
	CapturedAt       string   `json:"captured_at"`
	Strategy         string   `json:"strategy"`
	Config           string   `json:"config"`
	ExpectedContains []string `json:"expected_contains"`
}

func run() error {
	rawURL := flag.String("url", "", "URL to fetch (required)")
	name := flag.String("name", "", "Fixture directory name, e.g. ars_technica (required)")
	strategy := flag.String("strategy", "readability", "Extraction strategy: readability or selector")
	config := flag.String("config", "", "Strategy config, e.g. CSS selector for 'selector' strategy")
	expect := flag.String("expect", "", "Comma-separated strings that should appear in extracted output")
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP request timeout")
	flag.Parse()

	if *rawURL == "" || *name == "" {
		return fmt.Errorf("usage: capture-fixture -url <URL> -name <name> [-strategy selector] [-config '.selector'] [-expect 'text1,text2']")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Fetch the page
	client := &http.Client{Timeout: *timeout}
	logger.Info("fetching", "url", *rawURL)
	resp, err := client.Get(*rawURL)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body failed: %w", err)
	}

	// Target directory relative to repo root: internal/extractor/testdata/<name>
	// The tool is designed to be run from the repo root.
	outDir := filepath.Join("internal", "extractor", "testdata", *name)
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// Save article.html
	htmlPath := filepath.Join(outDir, "article.html")
	if err := os.WriteFile(htmlPath, body, 0o600); err != nil {
		return fmt.Errorf("write article.html failed: %w", err)
	}
	logger.Info("saved HTML", "path", htmlPath, "bytes", len(body))

	// Build expected_contains list
	var expectedContains []string
	if *expect != "" {
		for s := range strings.SplitSeq(*expect, ",") {
			if t := strings.TrimSpace(s); t != "" {
				expectedContains = append(expectedContains, t)
			}
		}
	}

	// Save meta.json
	m := meta{
		URL:              *rawURL,
		CapturedAt:       time.Now().UTC().Format(time.RFC3339),
		Strategy:         *strategy,
		Config:           *config,
		ExpectedContains: expectedContains,
	}
	metaBytes, _ := json.MarshalIndent(m, "", "  ")
	metaPath := filepath.Join(outDir, "meta.json")
	if err := os.WriteFile(metaPath, append(metaBytes, '\n'), 0o600); err != nil {
		return fmt.Errorf("write meta.json failed: %w", err)
	}
	logger.Info("saved meta", "path", metaPath)

	fmt.Printf("\nFixture saved to %s\n", outDir)
	fmt.Printf("Edit %s to add expected_contains strings, then run:\n", metaPath)
	fmt.Printf("  go test ./internal/extractor/ -run TestFixtures/%s -v\n", *name)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
