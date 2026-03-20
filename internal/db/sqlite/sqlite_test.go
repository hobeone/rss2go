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
}
