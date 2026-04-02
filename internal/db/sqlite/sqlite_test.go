package sqlite

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hobeone/rss2go"
	"github.com/stretchr/testify/assert"
)

func TestStore(t *testing.T) {
	dbPath := "test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil {
			t.Errorf("failed to remove test db: %v", err)
		}
	}()

	logger := slog.New(slog.DiscardHandler)
	store, err := New(dbPath, logger)
	assert.NoError(t, err)
	defer store.Close()

	err = store.Migrate(rss2go.MigrationsFS)
	assert.NoError(t, err)

	ctx := context.Background()

	// Test AddFeed
	id, err := store.AddFeed(ctx, "https://example.com/rss", "Example Feed", false, "readability", "")
	assert.NoError(t, err)
	assert.NotZero(t, id)

	// Test GetFeeds
	feeds, err := store.GetFeeds(ctx)
	assert.NoError(t, err)
	assert.Len(t, feeds, 1)
	assert.Equal(t, "https://example.com/rss", feeds[0].URL)
	assert.False(t, feeds[0].FullArticle)

	// Test AddUser
	uid, err := store.AddUser(ctx, "user@example.com")
	assert.NoError(t, err)
	assert.NotZero(t, uid)

	// Test Subscribe
	err = store.Subscribe(ctx, uid, id)
	assert.NoError(t, err)

	// Test GetUsersForFeed
	users, err := store.GetUsersForFeed(ctx, id)
	assert.NoError(t, err)
	assert.Len(t, users, 1)
	assert.Equal(t, "user@example.com", users[0].Email)

	// Test MarkSeen and IsSeen
	seen, err := store.IsSeen(ctx, id, "item-123")
	assert.NoError(t, err)
	assert.False(t, seen)

	err = store.MarkSeen(ctx, id, "item-123")
	assert.NoError(t, err)

	seen, err = store.IsSeen(ctx, id, "item-123")
	assert.NoError(t, err)
	assert.True(t, seen)

	// Test GetFeed
	f, err := store.GetFeed(ctx, id)
	assert.NoError(t, err)
	assert.NotNil(t, f)
	assert.Equal(t, "https://example.com/rss", f.URL)

	fNil, err := store.GetFeed(ctx, 999)
	assert.NoError(t, err)
	assert.Nil(t, fNil)

	// Test UpdateFeedLastPoll
	err = store.UpdateFeedLastPoll(ctx, id, "etag", "last_modified")
	assert.NoError(t, err)
	fUpdated, err := store.GetFeed(ctx, id)
	assert.NoError(t, err)
	assert.False(t, fUpdated.LastPoll.IsZero())

	// Test GetUserByEmail
	u, err := store.GetUserByEmail(ctx, "user@example.com")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.Equal(t, uid, u.ID)

	uNil, err := store.GetUserByEmail(ctx, "notfound@example.com")
	assert.NoError(t, err)
	assert.Nil(t, uNil)

	// Test SetFeedError
	err = store.SetFeedError(ctx, id, 500, "Internal Server Error")
	assert.NoError(t, err)
	fErr, err := store.GetFeed(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, 500, fErr.LastErrorCode)
	assert.Equal(t, "Internal Server Error", fErr.LastErrorSnippet)
	assert.False(t, fErr.LastErrorTime.IsZero())

	// Test ClearFeedError
	err = store.ClearFeedError(ctx, id)
	assert.NoError(t, err)
	fClear, err := store.GetFeed(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, 0, fClear.LastErrorCode)
	assert.Equal(t, "", fClear.LastErrorSnippet)
	assert.True(t, fClear.LastErrorTime.IsZero())

	// Test UpdateFeed
	newTitle := "Updated Title"
	err = store.UpdateFeed(ctx, id, nil, &newTitle, nil, nil, nil)
	assert.NoError(t, err)
	fUpdatedTitle, _ := store.GetFeed(ctx, id)
	assert.Equal(t, newTitle, fUpdatedTitle.Title)
	assert.Equal(t, "https://example.com/rss", fUpdatedTitle.URL)

	newURL := "https://example.com/new-rss"
	err = store.UpdateFeed(ctx, id, &newURL, nil, nil, nil, nil)
	assert.NoError(t, err)
	fUpdatedURL, _ := store.GetFeed(ctx, id)
	assert.Equal(t, newURL, fUpdatedURL.URL)
	assert.Equal(t, newTitle, fUpdatedURL.Title)

	fullArticle := true
	err = store.UpdateFeed(ctx, id, nil, nil, &fullArticle, nil, nil)
	assert.NoError(t, err)
	fUpdatedFA, _ := store.GetFeed(ctx, id)
	assert.True(t, fUpdatedFA.FullArticle)

	strategy := "selector"
	selConfig := "article.post-body"
	err = store.UpdateFeed(ctx, id, nil, nil, nil, &strategy, &selConfig)
	assert.NoError(t, err)
	fUpdatedExt, _ := store.GetFeed(ctx, id)
	assert.Equal(t, "selector", fUpdatedExt.ExtractionStrategy)
	assert.Equal(t, "article.post-body", fUpdatedExt.ExtractionConfig)

	// No changes
	err = store.UpdateFeed(ctx, id, nil, nil, nil, nil, nil)
	assert.NoError(t, err)

	// Test UpdateFeedBackoff — set, persist, and clear
	backoffTime := time.Now().Add(30 * time.Minute).Truncate(time.Second)
	err = store.UpdateFeedBackoff(ctx, id, backoffTime)
	assert.NoError(t, err)
	fBackoff, err := store.GetFeed(ctx, id)
	assert.NoError(t, err)
	assert.False(t, fBackoff.BackoffUntil.IsZero())
	assert.WithinDuration(t, backoffTime, fBackoff.BackoffUntil, time.Second)

	// Verify GetFeeds also returns it
	allFeeds, err := store.GetFeeds(ctx)
	assert.NoError(t, err)
	assert.Len(t, allFeeds, 1)
	assert.False(t, allFeeds[0].BackoffUntil.IsZero())

	// Clear the backoff
	err = store.UpdateFeedBackoff(ctx, id, time.Time{})
	assert.NoError(t, err)
	fCleared, err := store.GetFeed(ctx, id)
	assert.NoError(t, err)
	assert.True(t, fCleared.BackoffUntil.IsZero())
}

func TestStore_Outbox(t *testing.T) {
	dbPath := "test_outbox.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil {
			t.Errorf("failed to remove test db: %v", err)
		}
	}()

	logger := slog.New(slog.DiscardHandler)
	store, err := New(dbPath, logger)
	assert.NoError(t, err)
	defer store.Close()

	err = store.Migrate(rss2go.MigrationsFS)
	assert.NoError(t, err)

	ctx := context.Background()

	// Queue is empty initially.
	entry, err := store.ClaimPendingEmail(ctx)
	assert.NoError(t, err)
	assert.Nil(t, entry)

	// Enqueue two emails.
	recipients := []string{"a@example.com", "b@example.com"}
	err = store.EnqueueEmail(ctx, recipients, "Subject 1", "Body 1")
	assert.NoError(t, err)
	err = store.EnqueueEmail(ctx, recipients, "Subject 2", "Body 2")
	assert.NoError(t, err)

	// Claim the first one — should be status 'delivering'.
	e1, err := store.ClaimPendingEmail(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, e1)
	assert.Equal(t, "Subject 1", e1.Subject)
	assert.Equal(t, "delivering", e1.Status)
	assert.Equal(t, recipients, e1.Recipients)

	// Reset back to pending and re-claim.
	err = store.ResetEmailToPending(ctx, e1.ID)
	assert.NoError(t, err)

	// Now there are two pending; first is still e1.
	e1again, err := store.ClaimPendingEmail(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, e1again)
	assert.Equal(t, e1.ID, e1again.ID)

	// Mark it delivered.
	err = store.MarkEmailDelivered(ctx, e1again.ID)
	assert.NoError(t, err)

	// Second email can now be claimed.
	e2, err := store.ClaimPendingEmail(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, e2)
	assert.Equal(t, "Subject 2", e2.Subject)

	// Mark as delivering, then simulate crash recovery via ResetDeliveringToPending.
	err = store.ResetDeliveringToPending(ctx)
	assert.NoError(t, err)

	e2recovered, err := store.ClaimPendingEmail(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, e2recovered)
	assert.Equal(t, e2.ID, e2recovered.ID)

	err = store.MarkEmailDelivered(ctx, e2recovered.ID)
	assert.NoError(t, err)

	// Queue is empty again.
	empty, err := store.ClaimPendingEmail(ctx)
	assert.NoError(t, err)
	assert.Nil(t, empty)
}

func TestStore_Errors(t *testing.T) {
	dbPath := "test_errors.db"
	logger := slog.New(slog.DiscardHandler)
	store, err := New(dbPath, logger)
	assert.NoError(t, err)
	
	// Close the DB immediately to simulate connection errors
	assert.NoError(t, store.Close())
	assert.NoError(t, os.Remove(dbPath))

	ctx := context.Background()

	_, err = store.GetFeeds(ctx)
	assert.Error(t, err)

	_, err = store.GetFeed(ctx, 1)
	assert.Error(t, err)

	_, err = store.AddFeed(ctx, "url", "title", false, "readability", "")
	assert.Error(t, err)

	err = store.UpdateFeedLastPoll(ctx, 1, "", "")
	assert.Error(t, err)

	err = store.SetFeedError(ctx, 1, 500, "snippet")
	assert.Error(t, err)

	_, err = store.AddUser(ctx, "email")
	assert.Error(t, err)

	_, err = store.GetUserByEmail(ctx, "email")
	assert.Error(t, err)

	_, err = store.GetUsersForFeed(ctx, 1)
	assert.Error(t, err)

	err = store.Subscribe(ctx, 1, 1)
	assert.Error(t, err)

	_, err = store.IsSeen(ctx, 1, "guid")
	assert.Error(t, err)

	err = store.MarkSeen(ctx, 1, "guid")
	assert.Error(t, err)

	err = store.EnqueueEmail(ctx, []string{"a@b.com"}, "s", "b")
	assert.Error(t, err)

	_, err = store.ClaimPendingEmail(ctx)
	assert.Error(t, err)

	err = store.MarkEmailDelivered(ctx, 1)
	assert.Error(t, err)

	err = store.ResetEmailToPending(ctx, 1)
	assert.Error(t, err)

	err = store.ResetDeliveringToPending(ctx)
	assert.Error(t, err)
}

func TestNew_Error(t *testing.T) {
	// If it doesn't fail on open, it might fail on ping. SQLite is permissive, but we can try an invalid path format
	// Actually, just pass a directory instead of a file
	logger := slog.New(slog.DiscardHandler)
	_, err := New("/etc", logger)
	assert.Error(t, err)
}
