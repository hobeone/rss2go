package watcher

import (
	"container/heap"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hobeone/rss2go/internal/crawler"
	"github.com/hobeone/rss2go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestScheduler returns a Scheduler wired with a real crawler pool (size 1)
// and no-op mock mailer/store. The pool is closed when the test ends.
func newTestScheduler(t *testing.T) (*Scheduler, *crawler.Pool) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	cPool := crawler.NewPool(1, 5*time.Second, logger)
	t.Cleanup(func() { cPool.Close() })
	s := NewScheduler(cPool, new(mockMailer), new(mockStore), time.Hour, 0, 600, logger)
	return s, cPool
}

// rssServer starts a test HTTP server that returns a minimal valid RSS feed.
func rssServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title></channel></rss>`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestScheduler_Register(t *testing.T) {
	s, _ := newTestScheduler(t)
	feed := models.Feed{ID: 1, URL: "http://example.com/feed.rss", Title: "Test Feed"}

	s.Register(feed)

	s.mu.Lock()
	assert.Equal(t, 1, s.h.Len(), "heap should have one entry")
	assert.Contains(t, s.entries, int64(1), "entries map should contain feed ID")
	s.mu.Unlock()

	w, ok := s.GetWatcher(1)
	require.True(t, ok)
	assert.Equal(t, feed.ID, w.feed.ID)
}

func TestScheduler_Register_ReplacesExisting(t *testing.T) {
	s, _ := newTestScheduler(t)
	feed := models.Feed{ID: 1, URL: "http://example.com/feed.rss", Title: "Original"}
	s.Register(feed)

	updated := feed
	updated.Title = "Updated"
	s.Register(updated)

	s.mu.Lock()
	heapLen := s.h.Len()
	s.mu.Unlock()

	assert.Equal(t, 1, heapLen, "re-registering should not add a duplicate heap entry")

	w, ok := s.GetWatcher(1)
	require.True(t, ok)
	assert.Equal(t, "Updated", w.feed.Title, "watcher should reflect updated metadata")
}

func TestScheduler_Unregister(t *testing.T) {
	s, _ := newTestScheduler(t)
	s.Register(models.Feed{ID: 1, URL: "http://example.com/feed.rss"})

	s.Unregister(1)

	s.mu.Lock()
	assert.Equal(t, 0, s.h.Len(), "heap should be empty after unregister")
	assert.NotContains(t, s.entries, int64(1))
	assert.NotContains(t, s.watchers, int64(1))
	s.mu.Unlock()

	_, ok := s.GetWatcher(1)
	assert.False(t, ok)
}

func TestScheduler_Unregister_UnknownIsNoop(t *testing.T) {
	s, _ := newTestScheduler(t)
	assert.NotPanics(t, func() { s.Unregister(999) })
}

func TestScheduler_GetActiveFeedIDs(t *testing.T) {
	s, _ := newTestScheduler(t)
	s.Register(models.Feed{ID: 1, URL: "http://example.com/a"})
	s.Register(models.Feed{ID: 2, URL: "http://example.com/b"})
	s.Register(models.Feed{ID: 3, URL: "http://example.com/c"})

	ids := s.GetActiveFeedIDs()
	assert.ElementsMatch(t, []int64{1, 2, 3}, ids)
}

func TestScheduler_HeapOrdering(t *testing.T) {
	s, _ := newTestScheduler(t)

	// feed1: LastPoll 2h ago → scheduled at -1h from now, clamped to now (immediate)
	// feed2: LastPoll just now → scheduled at now+1h (not yet due)
	// With 0 jitter, feed1 should be at heap[0].
	now := time.Now()
	feed1 := models.Feed{ID: 1, URL: "http://example.com/a", LastPoll: now.Add(-2 * time.Hour)}
	feed2 := models.Feed{ID: 2, URL: "http://example.com/b", LastPoll: now}

	s.Register(feed2) // register the later-due feed first
	s.Register(feed1) // the earlier-due feed should bubble to the top

	s.mu.Lock()
	topID := s.h[0].feedID
	s.mu.Unlock()

	assert.Equal(t, int64(1), topID, "feed due sooner should be at heap top regardless of registration order")
}

func TestScheduler_BackoffHonored(t *testing.T) {
	s, _ := newTestScheduler(t)

	backoff := time.Now().Add(30 * time.Minute)
	feed := models.Feed{ID: 1, URL: "http://example.com/feed.rss", BackoffUntil: backoff}
	s.Register(feed)

	// The heap entry's nextPoll must reflect the backoff constraint.
	s.mu.Lock()
	nextPoll := s.h[0].nextPoll
	s.mu.Unlock()
	assert.False(t, nextPoll.Before(backoff), "nextPoll should be at or after BackoffUntil")

	// dispatchDue must not dispatch a feed whose nextPoll is in the future.
	s.dispatchDue(context.Background())

	s.mu.Lock()
	_, stillPresent := s.entries[feed.ID]
	s.mu.Unlock()
	assert.True(t, stillPresent, "backoffed feed should remain in heap, not be dispatched")
}

func TestScheduler_DispatchDue_RemovesFeedFromHeap(t *testing.T) {
	s, cPool := newTestScheduler(t)
	srv := rssServer(t)

	// No LastPoll → nextPoll = now (due immediately).
	feed := models.Feed{ID: 1, URL: srv.URL + "/feed.rss"}
	s.Register(feed)

	s.mu.Lock()
	assert.Equal(t, 1, s.h.Len())
	s.mu.Unlock()

	s.dispatchDue(context.Background())

	// The feed should have been removed from the heap during dispatch.
	s.mu.Lock()
	heapLen := s.h.Len()
	_, inEntries := s.entries[feed.ID]
	s.mu.Unlock()

	assert.Equal(t, 0, heapLen, "dispatched feed should be removed from heap")
	assert.False(t, inEntries, "dispatched feed should be removed from entries map")

	// Drain the response so the pool worker doesn't block on a full buffer.
	select {
	case <-cPool.Responses():
	case <-time.After(5 * time.Second):
		t.Fatal("expected a crawl response within 5 seconds")
	}
}

func TestScheduler_DispatchDue_NotDue(t *testing.T) {
	s, _ := newTestScheduler(t)

	// Feed polled just now → nextPoll = now+1h (not yet due).
	feed := models.Feed{ID: 1, URL: "http://example.com/feed.rss", LastPoll: time.Now()}
	s.Register(feed)

	s.dispatchDue(context.Background())

	s.mu.Lock()
	_, stillPresent := s.entries[feed.ID]
	s.mu.Unlock()

	assert.True(t, stillPresent, "feed not yet due should remain in heap")
}

func TestScheduler_ReschedFeed(t *testing.T) {
	s, _ := newTestScheduler(t)
	feed := models.Feed{ID: 1, URL: "http://example.com/feed.rss"}
	s.Register(feed)

	// Simulate the feed being removed from the heap during dispatch.
	s.mu.Lock()
	heap.Remove(&s.h, s.entries[1].index)
	delete(s.entries, int64(1))
	s.mu.Unlock()

	s.reschedFeed(1, 30*time.Minute)

	s.mu.Lock()
	heapLen := s.h.Len()
	entry, ok := s.entries[int64(1)]
	s.mu.Unlock()

	assert.Equal(t, 1, heapLen, "rescheduled feed should be back in the heap")
	require.True(t, ok, "rescheduled feed should be in entries map")
	assert.True(t, entry.nextPoll.After(time.Now()), "rescheduled feed's nextPoll should be in the future")
}

func TestScheduler_ReschedFeed_UnregisteredIsNoop(t *testing.T) {
	s, _ := newTestScheduler(t)

	// reschedFeed for a feed unregistered while in-flight should be silently dropped.
	assert.NotPanics(t, func() {
		s.reschedFeed(999, time.Minute)
	})

	s.mu.Lock()
	heapLen := s.h.Len()
	s.mu.Unlock()
	assert.Equal(t, 0, heapLen, "unregistered feed should not be re-inserted into heap")
}
