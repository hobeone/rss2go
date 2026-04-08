package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/hobeone/rss2go/internal/models"
	"github.com/hobeone/rss2go/internal/watcher"
	"github.com/stretchr/testify/assert"
)

// testSyncer is a minimal in-memory feedSyncer for testing syncFeedsOnce.
// Register stores the feed and makes it visible to subsequent GetWatcher calls.
// Unregister removes it. All registered/unregistered feeds are recorded for
// assertion.
type testSyncer struct {
	feeds        map[int64]models.Feed
	registered   []models.Feed
	unregistered []int64
}

func newTestSyncer(initial ...models.Feed) *testSyncer {
	s := &testSyncer{feeds: make(map[int64]models.Feed)}
	for _, f := range initial {
		s.feeds[f.ID] = f
	}
	return s
}

func (s *testSyncer) GetWatcher(feedID int64) (*watcher.Watcher, bool) {
	f, ok := s.feeds[feedID]
	if !ok {
		return nil, false
	}
	logger := slog.New(slog.DiscardHandler)
	return watcher.New(f, nil, nil, nil, time.Hour, 0, 600, logger), true
}

func (s *testSyncer) Register(feed models.Feed) {
	s.feeds[feed.ID] = feed
	s.registered = append(s.registered, feed)
}

func (s *testSyncer) Unregister(feedID int64) {
	delete(s.feeds, feedID)
	s.unregistered = append(s.unregistered, feedID)
}

func (s *testSyncer) GetActiveFeedIDs() []int64 {
	ids := make([]int64, 0, len(s.feeds))
	for id := range s.feeds {
		ids = append(ids, id)
	}
	return ids
}

// testStore is a minimal db.Store that returns a fixed feed list.
type testStore struct {
	feeds []models.Feed
	err   error
}

func (s *testStore) GetFeeds(_ context.Context) ([]models.Feed, error) {
	return s.feeds, s.err
}

// Stub out the rest of the db.Store interface.
func (s *testStore) GetFeedsWithErrors(_ context.Context) ([]models.Feed, error) { return nil, nil }
func (s *testStore) GetFeed(_ context.Context, _ int64) (*models.Feed, error)    { return nil, nil }
func (s *testStore) GetFeedByURL(_ context.Context, _ string) (*models.Feed, error) {
	return nil, nil
}
func (s *testStore) AddFeed(_ context.Context, _, _ string, _ bool, _, _ string) (int64, error) {
	return 0, nil
}
func (s *testStore) UpdateFeed(_ context.Context, _ int64, _ *string, _ *string, _ *bool, _ *string, _ *string) error {
	return nil
}
func (s *testStore) DeleteFeed(_ context.Context, _ int64) error        { return nil }
func (s *testStore) DeleteFeedByURL(_ context.Context, _ string) error  { return nil }
func (s *testStore) UpdateFeedLastPoll(_ context.Context, _ int64, _, _ string) error { return nil }
func (s *testStore) SetFeedError(_ context.Context, _ int64, _ int, _ string) error   { return nil }
func (s *testStore) ClearFeedError(_ context.Context, _ int64) error                  { return nil }
func (s *testStore) UpdateFeedBackoff(_ context.Context, _ int64, _ time.Time) error  { return nil }
func (s *testStore) AddUser(_ context.Context, _ string) (int64, error)               { return 0, nil }
func (s *testStore) GetUserByEmail(_ context.Context, _ string) (*models.User, error) {
	return nil, nil
}
func (s *testStore) GetUsersForFeed(_ context.Context, _ int64) ([]models.User, error) {
	return nil, nil
}
func (s *testStore) Subscribe(_ context.Context, _, _ int64) error          { return nil }
func (s *testStore) Unsubscribe(_ context.Context, _, _ int64) error        { return nil }
func (s *testStore) IsSeen(_ context.Context, _ int64, _ string) (bool, error) { return false, nil }
func (s *testStore) MarkSeen(_ context.Context, _ int64, _ string) error    { return nil }
func (s *testStore) UnseenRecentItems(_ context.Context, _ int64, _ int) ([]string, error) {
	return nil, nil
}
func (s *testStore) ResetFeedPoll(_ context.Context, _ int64) error { return nil }
func (s *testStore) EnqueueEmail(_ context.Context, _ []string, _, _ string) error { return nil }
func (s *testStore) ClaimPendingEmail(_ context.Context) (*models.OutboxEntry, error) {
	return nil, nil
}
func (s *testStore) MarkEmailDelivered(_ context.Context, _ int64) error    { return nil }
func (s *testStore) ResetEmailToPending(_ context.Context, _ int64) error   { return nil }
func (s *testStore) ResetDeliveringToPending(_ context.Context) error       { return nil }
func (s *testStore) Close() error                                           { return nil }

// ── tests ──────────────────────────────────────────────────────────────────

func TestSyncFeedsOnce_RegistersNewFeed(t *testing.T) {
	newFeed := models.Feed{ID: 1, URL: "http://example.com/feed.rss", Title: "New"}
	store := &testStore{feeds: []models.Feed{newFeed}}
	syncer := newTestSyncer() // starts empty

	syncFeedsOnce(context.Background(), store, syncer, slog.New(slog.DiscardHandler))

	assert.Len(t, syncer.registered, 1)
	assert.Equal(t, newFeed.ID, syncer.registered[0].ID)
	assert.Empty(t, syncer.unregistered)
}

func TestSyncFeedsOnce_UnregistersRemovedFeed(t *testing.T) {
	existing := models.Feed{ID: 1, URL: "http://example.com/feed.rss"}
	store := &testStore{feeds: []models.Feed{}} // DB is now empty
	syncer := newTestSyncer(existing)            // syncer still has the feed

	syncFeedsOnce(context.Background(), store, syncer, slog.New(slog.DiscardHandler))

	assert.Empty(t, syncer.registered)
	assert.Contains(t, syncer.unregistered, int64(1))
}

func TestSyncFeedsOnce_ReregistersOnMetadataChange(t *testing.T) {
	original := models.Feed{ID: 1, URL: "http://example.com/feed.rss", Title: "Old Title"}
	updated := models.Feed{ID: 1, URL: "http://example.com/feed.rss", Title: "New Title"}

	store := &testStore{feeds: []models.Feed{updated}}
	syncer := newTestSyncer(original)

	syncFeedsOnce(context.Background(), store, syncer, slog.New(slog.DiscardHandler))

	assert.Len(t, syncer.registered, 1, "changed feed should be re-registered")
	assert.Equal(t, "New Title", syncer.registered[0].Title)
	assert.Empty(t, syncer.unregistered)
}

func TestSyncFeedsOnce_NoOpWhenUnchanged(t *testing.T) {
	feed := models.Feed{ID: 1, URL: "http://example.com/feed.rss", Title: "Same"}
	store := &testStore{feeds: []models.Feed{feed}}
	syncer := newTestSyncer(feed) // already registered with identical data

	syncFeedsOnce(context.Background(), store, syncer, slog.New(slog.DiscardHandler))

	assert.Empty(t, syncer.registered, "unchanged feed should not be re-registered")
	assert.Empty(t, syncer.unregistered)
}

func TestSyncFeedsOnce_DBErrorIsLoggedAndSkipped(t *testing.T) {
	store := &testStore{err: errors.New("db unavailable")}
	syncer := newTestSyncer()

	// Must not panic; no register/unregister should occur.
	assert.NotPanics(t, func() {
		syncFeedsOnce(context.Background(), store, syncer, slog.New(slog.DiscardHandler))
	})

	assert.Empty(t, syncer.registered)
	assert.Empty(t, syncer.unregistered)
}

func TestSyncFeedsOnce_ExtractionStrategyChange(t *testing.T) {
	original := models.Feed{ID: 1, URL: "http://example.com/feed.rss", ExtractionStrategy: "readability"}
	updated := models.Feed{ID: 1, URL: "http://example.com/feed.rss", ExtractionStrategy: "selector", ExtractionConfig: ".article"}

	store := &testStore{feeds: []models.Feed{updated}}
	syncer := newTestSyncer(original)

	syncFeedsOnce(context.Background(), store, syncer, slog.New(slog.DiscardHandler))

	assert.Len(t, syncer.registered, 1, "extraction strategy change should trigger re-registration")
	assert.Equal(t, "selector", syncer.registered[0].ExtractionStrategy)
}
