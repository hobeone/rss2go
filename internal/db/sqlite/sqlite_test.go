package sqlite

import (
	"context"
	"os"
	"testing"

	"github.com/hobe/rss2go"
	"github.com/stretchr/testify/assert"
)

func TestStore(t *testing.T) {
	dbPath := "test.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	assert.NoError(t, err)
	defer store.Close()

	err = store.Migrate(rss2go.MigrationsFS)
	assert.NoError(t, err)

	ctx := context.Background()

	// Test AddFeed
	id, err := store.AddFeed(ctx, "https://example.com/rss", "Example Feed")
	assert.NoError(t, err)
	assert.NotZero(t, id)

	// Test GetFeeds
	feeds, err := store.GetFeeds(ctx)
	assert.NoError(t, err)
	assert.Len(t, feeds, 1)
	assert.Equal(t, "https://example.com/rss", feeds[0].URL)

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
	err = store.UpdateFeedLastPoll(ctx, id)
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
}

func TestStore_Errors(t *testing.T) {
	dbPath := "test_errors.db"
	store, err := New(dbPath)
	assert.NoError(t, err)
	
	// Close the DB immediately to simulate connection errors
	store.Close()
	os.Remove(dbPath)

	ctx := context.Background()

	_, err = store.GetFeeds(ctx)
	assert.Error(t, err)

	_, err = store.GetFeed(ctx, 1)
	assert.Error(t, err)

	_, err = store.AddFeed(ctx, "url", "title")
	assert.Error(t, err)

	err = store.UpdateFeedLastPoll(ctx, 1)
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
}

func TestNew_Error(t *testing.T) {
	// If it doesn't fail on open, it might fail on ping. SQLite is permissive, but we can try an invalid path format
	// Actually, just pass a directory instead of a file
	_, err := New("/etc")
	assert.Error(t, err)
}


