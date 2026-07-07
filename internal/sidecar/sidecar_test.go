package sidecar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const mockHTML = `<!doctype html>
<html>
  <head><title>Test News</title></head>
  <body>
    <div class="story">
      <h2 class="title">Article One</h2>
      <a class="url" href="/story-one">Read One</a>
      <p class="summary">Summary text one.</p>
    </div>
    <div class="story">
      <h2 class="title">Article Two</h2>
      <a class="url" href="http://external.site/story-two">Read Two</a>
      <p class="summary">Summary text two.</p>
    </div>
    <div class="story">
      <h2 class="title"></h2>
      <a class="url" href="/story-three">Empty Title</a>
    </div>
    <div class="story">
      <h2 class="title">No Link Article</h2>
      <a class="url" href="">No URL</a>
    </div>
  </body>
</html>`

func TestSidecarScrapeSuccess(t *testing.T) {
	// 1. Start local mock target server returning HTML
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, mockHTML)
	}))
	defer targetServer.Close()

	// 2. Create sidecar server and query it
	s := NewServer("127.0.0.1:0", nil, nil)
	handler := http.HandlerFunc(s.handleScrape)

	req := httptest.NewRequest("GET", fmt.Sprintf("/scrape?url=%s&item=.story&title=.title&link=.url&description=.summary", targetServer.URL), nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Check headers
	if !strings.Contains(resp.Header.Get("Content-Type"), "application/xml") {
		t.Errorf("expected Content-Type application/xml, got %q", resp.Header.Get("Content-Type"))
	}

	// Verify XML contents
	if !strings.Contains(bodyStr, "<rss version=\"2.0\">") {
		t.Errorf("expected rss version 2.0 tag")
	}
	if !strings.Contains(bodyStr, "<title>Article One</title>") {
		t.Errorf("expected title 'Article One'")
	}
	// Verify relative link was resolved to absolute targetServer.URL
	expectedLink := fmt.Sprintf("<link>%s/story-one</link>", targetServer.URL)
	if !strings.Contains(bodyStr, expectedLink) {
		t.Errorf("expected resolved relative link %q, not found in %q", expectedLink, bodyStr)
	}
	// Verify absolute link remained absolute
	if !strings.Contains(bodyStr, "<link>http://external.site/story-two</link>") {
		t.Errorf("expected external link remained absolute")
	}
	// Verify description was populated
	if !strings.Contains(bodyStr, "<description>Summary text one.</description>") {
		t.Errorf("expected description text")
	}
	// Verify malformed items (empty title or empty link) were skipped
	if strings.Contains(bodyStr, "Empty Title") || strings.Contains(bodyStr, "No Link Article") {
		t.Errorf("expected malformed elements to be skipped")
	}
}

func TestSidecarScrapeValidationErrors(t *testing.T) {
	s := NewServer("127.0.0.1:0", nil, nil)
	handler := http.HandlerFunc(s.handleScrape)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
	}{
		{
			name:           "Missing target URL",
			query:          "/scrape?item=.story&title=.title&link=.url",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Missing item selector",
			query:          "/scrape?url=http://local&title=.title&link=.url",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid target URL format",
			query:          "/scrape?url=invalid-url&item=.story&title=.title&link=.url",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.query, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			resp := w.Result()
			_ = resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestSidecarScrapeTargetErrors(t *testing.T) {
	// 1. Mock Client returning custom errors or non-200 responses
	s := NewServer("127.0.0.1:0", &http.Client{
		Transport: &mockErrorTransport{},
	}, nil)
	handler := http.HandlerFunc(s.handleScrape)

	req := httptest.NewRequest("GET", "/scrape?url=http://erroring-site.com&item=.story&title=.title&link=.url", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected StatusBadGateway 502 on dial failure, got %d", resp.StatusCode)
	}

	// 2. Mock client returning non-200 Status
	sNon200 := NewServer("127.0.0.1:0", &http.Client{
		Transport: &non200Transport{},
	}, nil)
	handlerNon200 := http.HandlerFunc(sNon200.handleScrape)

	req2 := httptest.NewRequest("GET", "/scrape?url=http://non200.com&item=.story&title=.title&link=.url", nil)
	w2 := httptest.NewRecorder()
	handlerNon200.ServeHTTP(w2, req2)

	resp2 := w2.Result()
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusBadGateway {
		t.Errorf("expected StatusBadGateway on non-200 target response, got %d", resp2.StatusCode)
	}
}

func TestSidecarServerStartStop(t *testing.T) {
	s := NewServer("127.0.0.1:0", nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	var runErr error
	var wg sync.WaitGroup
	var startExited bool
	var mu sync.Mutex

	wg.Go(func() {
		err := s.Start(ctx)
		mu.Lock()
		runErr = err
		startExited = true
		mu.Unlock()
	})

	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	exited := startExited
	errVal := runErr
	mu.Unlock()

	if exited {
		t.Errorf("expected server to block, but exited early: %v", errVal)
	}

	cancel()
	wg.Wait()

	mu.Lock()
	errVal = runErr
	mu.Unlock()

	if errVal != nil {
		t.Errorf("expected clean server shutdown, got error: %v", errVal)
	}
}

func TestSidecarServerStartError(t *testing.T) {
	s := NewServer("999.999.999.999:9999", nil, nil)
	ctx := context.Background()

	err := s.Start(ctx)
	if err == nil {
		t.Error("expected server to fail to bind on invalid address, but returned nil")
	}
}

type mockErrorTransport struct{}

func (t *mockErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("mock network connection failed")
}

type non200Transport struct{}

func (t *non200Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Status:     "404 Not Found",
		Body:       io.NopCloser(bytes.NewReader([]byte("not found"))),
	}, nil
}

func TestSidecarNewServerDefaultClient(t *testing.T) {
	s := NewServer("127.0.0.1:0", nil, nil)
	if s.client == nil {
		t.Error("expected client to be initialized")
	}
	if s.client.Timeout != 30*time.Second {
		t.Errorf("expected client timeout 30s, got %v", s.client.Timeout)
	}
}
