package watcher

import (
	"context"
	"log/slog"
	"sync"

	"github.com/hobe/rss2go/internal/crawler"
)

// Registry manages multiple watchers and routes crawl responses.
type Registry struct {
	watchers map[int64]*Watcher
	mu       sync.RWMutex
	crawler  *crawler.Pool
	logger   *slog.Logger
}

// NewRegistry creates a new watcher registry.
func NewRegistry(c *crawler.Pool, logger *slog.Logger) *Registry {
	return &Registry{
		watchers: make(map[int64]*Watcher),
		crawler:  c,
		logger:   logger.With("component", "registry"),
	}
}

// Register adds a watcher to the registry.
func (r *Registry) Register(w *Watcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.watchers[w.feed.ID] = w
}

// Unregister removes a watcher from the registry.
func (r *Registry) Unregister(feedID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.watchers, feedID)
}

// GetWatcher retrieves a watcher by feed ID.
func (r *Registry) GetWatcher(feedID int64) (*Watcher, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.watchers[feedID]
	return w, ok
}

// GetActiveFeedIDs returns a list of all currently registered feed IDs.
func (r *Registry) GetActiveFeedIDs() []int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]int64, 0, len(r.watchers))
	for id := range r.watchers {
		ids = append(ids, id)
	}
	return ids
}

// Start starts the response router loop.
func (r *Registry) Start(ctx context.Context) {
	r.logger.Info("starting response router")
	for {
		select {
		case resp, ok := <-r.crawler.Responses():
			if !ok {
				return
			}
			r.mu.RLock()
			w, ok := r.watchers[resp.FeedID]
			r.mu.RUnlock()

			if ok {
				go w.HandleResponse(ctx, resp)
			} else {
				r.logger.Warn("received response for unknown feed", "feed_id", resp.FeedID)
			}
		case <-ctx.Done():
			r.logger.Info("response router shutting down")
			return
		}
	}
}
