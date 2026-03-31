package crawler

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPool(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("mock rss content")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer ts.Close()

	logger := slog.New(slog.DiscardHandler)
	p := NewPool(2, 5*time.Second, logger)
	defer p.Close()

	req := CrawlRequest{
		FeedID: 1,
		URL:    ts.URL,
	}
	p.Submit(req)

	resp := <-p.Responses()
	assert.NoError(t, resp.Error)
	assert.Equal(t, int64(1), resp.FeedID)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "mock rss content", string(resp.Body))
}

func TestPool_RateLimited(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		retryAfterHdr  string
		wantRetryAfter time.Duration
	}{
		{
			name:           "429 with integer seconds",
			statusCode:     http.StatusTooManyRequests,
			retryAfterHdr:  "60",
			wantRetryAfter: 60 * time.Second,
		},
		{
			name:           "503 with integer seconds",
			statusCode:     http.StatusServiceUnavailable,
			retryAfterHdr:  "30",
			wantRetryAfter: 30 * time.Second,
		},
		{
			name:           "429 with no Retry-After header",
			statusCode:     http.StatusTooManyRequests,
			retryAfterHdr:  "",
			wantRetryAfter: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.retryAfterHdr != "" {
					w.Header().Set("Retry-After", tt.retryAfterHdr)
				}
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, "rate limited")
			}))
			defer ts.Close()

			logger := slog.New(slog.DiscardHandler)
			p := NewPool(1, 5*time.Second, logger)
			defer p.Close()

			p.Submit(CrawlRequest{FeedID: 1, URL: ts.URL})

			resp := <-p.Responses()
			assert.Error(t, resp.Error)
			assert.Equal(t, tt.statusCode, resp.StatusCode)
			assert.Equal(t, tt.wantRetryAfter, resp.RetryAfter)
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)

	tests := []struct {
		header  string
		wantMin time.Duration // lower bound (0 means expect 0)
		wantMax time.Duration // upper bound
	}{
		{"", 0, 0},
		{"0", 0, 0},
		{"-1", 0, 0},
		{"120", 120 * time.Second, 120 * time.Second},
		{" 45 ", 45 * time.Second, 45 * time.Second},
		{"not-a-date", 0, 0},
		{future, 110 * time.Second, 130 * time.Second}, // ~2 min, allow clock skew
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("header=%q", tt.header), func(t *testing.T) {
			got := parseRetryAfter(tt.header)
			if tt.wantMin == 0 && tt.wantMax == 0 {
				assert.Equal(t, time.Duration(0), got)
			} else {
				assert.GreaterOrEqual(t, got, tt.wantMin)
				assert.LessOrEqual(t, got, tt.wantMax)
			}
		})
	}
}

func TestPool_SizeLimit(t *testing.T) {
	content := make([]byte, MaxResponseBodySize+100)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(content); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer ts.Close()

	logger := slog.New(slog.DiscardHandler)
	p := NewPool(1, 5*time.Second, logger)
	defer p.Close()

	req := CrawlRequest{
		FeedID: 1,
		URL:    ts.URL,
	}
	p.Submit(req)

	resp := <-p.Responses()
	assert.NoError(t, resp.Error)
	assert.Equal(t, MaxResponseBodySize, len(resp.Body))
}
