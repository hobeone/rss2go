package crawler

import (
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
		w.Write([]byte("mock rss content"))
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
