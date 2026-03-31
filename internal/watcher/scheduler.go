package watcher

import (
	"container/heap"
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hobeone/rss2go/internal/crawler"
	"github.com/hobeone/rss2go/internal/db"
	"github.com/hobeone/rss2go/internal/models"
)

// schedEntry is one slot in the min-heap.
type schedEntry struct {
	feedID   int64
	nextPoll time.Time
	index    int // maintained by the heap for O(log n) Fix/Remove
}

// schedHeap is a min-heap ordered by nextPoll.
type schedHeap []*schedEntry

func (h schedHeap) Len() int           { return len(h) }
func (h schedHeap) Less(i, j int) bool { return h[i].nextPoll.Before(h[j].nextPoll) }
func (h schedHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *schedHeap) Push(x any) {
	e := x.(*schedEntry)
	e.index = len(*h)
	*h = append(*h, e)
}
func (h *schedHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	e.index = -1
	*h = old[:n-1]
	return e
}

// Scheduler manages feed polling with a priority queue instead of one
// goroutine per feed. A single scheduler goroutine sleeps until the next
// feed is due, dispatches crawl requests for all due feeds, then goes back
// to sleep. Feeds are removed from the heap while a crawl is in-flight and
// re-inserted once the response is processed, so at most one crawl per feed
// is ever in-flight.
type Scheduler struct {
	mu            sync.Mutex
	h             schedHeap
	entries       map[int64]*schedEntry // feedID → live heap entry
	watchers      map[int64]*Watcher
	crawlerPool   *crawler.Pool
	mailerPool    MailerPool
	store         db.Store
	interval      time.Duration
	jitter        time.Duration
	maxImageWidth int
	logger        *slog.Logger
	wakeC         chan struct{} // non-blocking signal to interrupt sleep
}

// NewScheduler creates a Scheduler.
func NewScheduler(
	crawlerPool *crawler.Pool,
	mailerPool MailerPool,
	store db.Store,
	interval, jitter time.Duration,
	maxImageWidth int,
	logger *slog.Logger,
) *Scheduler {
	s := &Scheduler{
		h:             make(schedHeap, 0),
		entries:       make(map[int64]*schedEntry),
		watchers:      make(map[int64]*Watcher),
		crawlerPool:   crawlerPool,
		mailerPool:    mailerPool,
		store:         store,
		interval:      interval,
		jitter:        jitter,
		maxImageWidth: maxImageWidth,
		logger:        logger.With("component", "scheduler"),
		wakeC:         make(chan struct{}, 1),
	}
	heap.Init(&s.h)
	return s
}

// Register adds a feed to the scheduler. If a watcher already exists for this
// feed it is replaced (used for metadata updates from the resync loop).
func (s *Scheduler) Register(feed models.Feed) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove any existing heap entry for this feed.
	if e, ok := s.entries[feed.ID]; ok {
		heap.Remove(&s.h, e.index)
		delete(s.entries, feed.ID)
	}

	w := New(feed, s.store, s.crawlerPool, s.mailerPool, s.interval, s.jitter, s.maxImageWidth, s.logger)
	s.watchers[feed.ID] = w

	// Determine the initial poll time, honouring any persisted backoff first.
	// If backoff_until is in the future, use it directly (no jitter — this is
	// a hard "don't retry before" constraint). Otherwise schedule from
	// max(last_poll+interval, now) with the usual per-feed jitter.
	now := time.Now()
	var nextPoll time.Time
	if !feed.BackoffUntil.IsZero() && feed.BackoffUntil.After(now) {
		nextPoll = feed.BackoffUntil
		s.logger.Info("resuming feed under backoff", "feed_id", feed.ID, "backoff_until", feed.BackoffUntil)
	} else {
		nextPoll = now
		if !feed.LastPoll.IsZero() {
			if scheduled := feed.LastPoll.Add(s.interval); scheduled.After(nextPoll) {
				nextPoll = scheduled
			}
		}
		nextPoll = nextPoll.Add(w.getJitter())
	}

	e := &schedEntry{feedID: feed.ID, nextPoll: nextPoll}
	s.entries[feed.ID] = e
	heap.Push(&s.h, e)

	s.wake()
	s.logger.Info("registered feed", "feed_id", feed.ID, "next_poll", nextPoll)
}

// Unregister removes a feed from the scheduler. If a crawl is already in
// flight for this feed, its response will be silently dropped.
func (s *Scheduler) Unregister(feedID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.watchers, feedID)
	if e, ok := s.entries[feedID]; ok {
		heap.Remove(&s.h, e.index)
		delete(s.entries, feedID)
	}
	s.logger.Info("unregistered feed", "feed_id", feedID)
}

// GetWatcher returns the Watcher for a feed (used by the resync loop to
// compare current metadata against the database).
func (s *Scheduler) GetWatcher(feedID int64) (*Watcher, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.watchers[feedID]
	return w, ok
}

// GetActiveFeedIDs returns the IDs of all currently registered feeds.
func (s *Scheduler) GetActiveFeedIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]int64, 0, len(s.watchers))
	for id := range s.watchers {
		ids = append(ids, id)
	}
	return ids
}

// Run starts the scheduler. It blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	s.logger.Info("scheduler started", "feeds", s.h.Len())
	go s.routeResponses(ctx)

	for {
		s.mu.Lock()
		var delay time.Duration
		if s.h.Len() == 0 {
			delay = time.Minute // idle; wake when a feed is registered
		} else {
			delay = time.Until(s.h[0].nextPoll)
		}
		s.mu.Unlock()

		if delay < 0 {
			delay = 0
		}

		select {
		case <-time.After(delay):
			s.dispatchDue(ctx)
		case <-s.wakeC:
			// Heap changed; recalculate delay at top of loop.
		case <-ctx.Done():
			s.logger.Info("scheduler shutting down")
			return
		}
	}
}

// dispatchDue submits crawl requests for all feeds whose nextPoll is now or
// in the past. Each feed is removed from the heap before its crawl fires and
// only re-inserted after the response arrives (see reschedFeed).
func (s *Scheduler) dispatchDue(ctx context.Context) {
	now := time.Now()

	// Collect watchers to dispatch under the lock, then release before calling
	// Submit (which can block if the crawler buffer is full).
	s.mu.Lock()
	var due []*Watcher
	for s.h.Len() > 0 && !s.h[0].nextPoll.After(now) {
		e := heap.Pop(&s.h).(*schedEntry)
		delete(s.entries, e.feedID)
		if w, ok := s.watchers[e.feedID]; ok {
			due = append(due, w)
		}
	}
	s.mu.Unlock()

	for _, w := range due {
		s.logger.Debug("dispatching crawl", "feed_id", w.feed.ID)
		w.crawl(ctx)
	}
}

// reschedFeed re-inserts a feed into the heap after its crawl response has
// been fully processed. Called from the response-routing goroutine.
func (s *Scheduler) reschedFeed(feedID int64, interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.watchers[feedID]; !ok {
		return // unregistered while crawl was in-flight; discard
	}

	// Guard against double-insertion (should not happen but be defensive).
	if e, ok := s.entries[feedID]; ok {
		heap.Remove(&s.h, e.index)
		delete(s.entries, feedID)
	}

	nextPoll := time.Now().Add(interval)
	e := &schedEntry{feedID: feedID, nextPoll: nextPoll}
	s.entries[feedID] = e
	heap.Push(&s.h, e)

	s.wake()
	s.logger.Debug("rescheduled feed", "feed_id", feedID, "next_poll", nextPoll)
}

// routeResponses reads crawl responses and dispatches each to its Watcher.
// For feed responses it also triggers rescheduling once handling completes.
func (s *Scheduler) routeResponses(ctx context.Context) {
	s.logger.Info("response router started")
	for {
		select {
		case resp, ok := <-s.crawlerPool.Responses():
			if !ok {
				return
			}
			s.mu.Lock()
			w, ok := s.watchers[resp.FeedID]
			s.mu.Unlock()

			if !ok {
				s.logger.Warn("received response for unknown feed", "feed_id", resp.FeedID)
				continue
			}

			go func(w *Watcher, resp crawler.CrawlResponse) {
				interval, isFeed := w.HandleResponse(ctx, resp)
				if isFeed {
					s.reschedFeed(resp.FeedID, interval)
				}
			}(w, resp)

		case <-ctx.Done():
			s.logger.Info("response router shutting down")
			return
		}
	}
}

// wake sends a non-blocking signal to the scheduler loop to recalculate its
// sleep duration. Must be called with or without s.mu held (channel is async).
func (s *Scheduler) wake() {
	select {
	case s.wakeC <- struct{}{}:
	default:
	}
}
