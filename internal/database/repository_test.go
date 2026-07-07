package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"rss2go/internal/types"
)

func setupTestDB(t *testing.T) (*sql.DB, *Repository) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db, NewRepository(db)
}

func TestFeedOperations(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().Round(time.Second).UTC()
	errTimeInit := now.Add(-time.Hour)

	feed := &types.Feed{
		Title:              "Example Feed",
		URL:                "https://example.com/rss",
		NextPollAt:         now,
		PollIntervalSecs:   1800,
		BackoffFactor:      1.5,
		LastErrorTime:      &errTimeInit,
		LastErrorStr:       "Initial Error",
		LastErrorSnippet:   "Connection reset",
		ExtractFullArticle: true,
		ExtractionStrategy: types.StrategySelector,
		CSSSelector:        ".article-body",
	}

	// Negative get check
	_, err := repo.GetFeed(ctx, 9999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	_, err = repo.GetFeedByURL(ctx, "https://nonexistent.com/rss")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}

	// 1. Create
	if err := repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}
	if feed.ID == 0 {
		t.Fatal("expected feed ID to be populated")
	}

	// 2. Get
	fetched, err := repo.GetFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("failed to get feed: %v", err)
	}
	if fetched.Title != feed.Title || fetched.URL != feed.URL || fetched.CSSSelector != feed.CSSSelector {
		t.Errorf("fetched feed mismatch: %+v vs %+v", fetched, feed)
	}
	if fetched.ExtractFullArticle != feed.ExtractFullArticle {
		t.Errorf("fetched extract article mismatch: %v vs %v", fetched.ExtractFullArticle, feed.ExtractFullArticle)
	}
	if fetched.LastErrorTime == nil || !fetched.LastErrorTime.Equal(errTimeInit) {
		t.Errorf("expected initial LastErrorTime %v, got %v", errTimeInit, fetched.LastErrorTime)
	}

	// Get by URL
	fetchedByURL, err := repo.GetFeedByURL(ctx, feed.URL)
	if err != nil {
		t.Fatalf("failed to get feed by URL: %v", err)
	}
	if fetchedByURL.ID != feed.ID {
		t.Errorf("expected feed ID %d, got %d", feed.ID, fetchedByURL.ID)
	}

	// 3. Update
	fetched.Title = "Updated Title"
	errTimeUpdate := now.Add(time.Minute)
	fetched.LastErrorTime = &errTimeUpdate
	fetched.LastErrorStr = "HTTP 500"
	fetched.LastErrorSnippet = "Internal Server Error"
	if err := repo.UpdateFeed(ctx, fetched); err != nil {
		t.Fatalf("failed to update feed: %v", err)
	}

	updated, err := repo.GetFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("failed to get updated feed: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("expected Title to be updated, got: %s", updated.Title)
	}
	if updated.LastErrorStr != "HTTP 500" {
		t.Errorf("expected LastErrorStr to be updated, got: %s", updated.LastErrorStr)
	}
	if updated.LastErrorTime == nil || !updated.LastErrorTime.Equal(errTimeUpdate) {
		t.Errorf("expected LastErrorTime to match: %v, got %v", errTimeUpdate, updated.LastErrorTime)
	}

	// Negative update check
	err = repo.UpdateFeed(ctx, &types.Feed{ID: 9999, Title: "Ghost"})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}

	// 4. List & List Due
	feedsList, err := repo.ListFeeds(ctx)
	if err != nil {
		t.Fatalf("failed to list feeds: %v", err)
	}
	if len(feedsList) != 1 || feedsList[0].ID != feed.ID {
		t.Errorf("expected list to contain feed, got %+v", feedsList)
	}
	if feedsList[0].ExtractFullArticle != updated.ExtractFullArticle || feedsList[0].Title != updated.Title {
		t.Errorf("list feed details mismatch: %+v vs %+v", feedsList[0], updated)
	}

	dueFeeds, err := repo.ListFeedsDue(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatalf("failed to list due feeds: %v", err)
	}
	if len(dueFeeds) != 1 {
		t.Errorf("expected 1 due feed, got %d", len(dueFeeds))
	}
	if dueFeeds[0].ExtractFullArticle != updated.ExtractFullArticle || dueFeeds[0].Title != updated.Title {
		t.Errorf("due feed details mismatch: %+v vs %+v", dueFeeds[0], updated)
	}

	// 5. Delete
	if err := repo.DeleteFeed(ctx, feed.ID); err != nil {
		t.Fatalf("failed to delete feed: %v", err)
	}

	_, err = repo.GetFeed(ctx, feed.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}

	// Negative delete check
	err = repo.DeleteFeed(ctx, 9999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestUserAndSubscriptionOperations(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	// Negative get check
	_, err := repo.GetUser(ctx, 9999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	_, err = repo.GetUserByEmail(ctx, "ghost@example.com")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	err = repo.DeleteUser(ctx, 9999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}

	// Create Feed
	feed := &types.Feed{
		Title:      "Test Feed",
		URL:        "https://test.com/feed",
		NextPollAt: time.Now(),
	}
	if err := repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	// Create User
	user := &types.User{
		Email: "alice@example.com",
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Get User and List Users
	fetchedUser, err := repo.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if fetchedUser.Email != user.Email {
		t.Errorf("user email mismatch: %s vs %s", fetchedUser.Email, user.Email)
	}

	fetchedByEmail, err := repo.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("failed to get user by email: %v", err)
	}
	if fetchedByEmail.ID != user.ID {
		t.Errorf("user ID mismatch: %d vs %d", user.ID, fetchedByEmail.ID)
	}

	usersList, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatalf("failed to list users: %v", err)
	}
	if len(usersList) != 1 || usersList[0].ID != user.ID {
		t.Errorf("expected list to contain user, got %+v", usersList)
	}

	// Subscribe
	if err := repo.Subscribe(ctx, user.ID, feed.ID); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// List subscriptions for user
	feeds, err := repo.ListSubscriptionsForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to list user subscriptions: %v", err)
	}
	if len(feeds) != 1 || feeds[0].ID != feed.ID {
		t.Errorf("expected subscription to contain feed ID %d, got %+v", feed.ID, feeds)
	}

	// List subscriptions for feed
	users, err := repo.ListSubscriptionsForFeed(ctx, feed.ID)
	if err != nil {
		t.Fatalf("failed to list feed subscriptions: %v", err)
	}
	if len(users) != 1 || users[0].ID != user.ID {
		t.Errorf("expected subscription to contain user ID %d, got %+v", user.ID, users)
	}

	// Unsubscribe
	if err := repo.Unsubscribe(ctx, user.ID, feed.ID); err != nil {
		t.Fatalf("failed to unsubscribe: %v", err)
	}

	feeds, err = repo.ListSubscriptionsForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to list subscriptions: %v", err)
	}
	if len(feeds) != 0 {
		t.Errorf("expected 0 subscriptions after unsubscribe, got %d", len(feeds))
	}

	// Negative unsubscribe check
	err = repo.Unsubscribe(ctx, user.ID, feed.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}

	// Delete User
	if err := repo.DeleteUser(ctx, user.ID); err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}
}

func TestSeenItems(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	feed := &types.Feed{
		Title:      "Feed",
		URL:        "http://url",
		NextPollAt: time.Now(),
	}
	if err := repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	// IsSeen initially false
	seen, err := repo.IsItemSeen(ctx, feed.ID, "guid-1")
	if err != nil {
		t.Fatalf("failed to check seen: %v", err)
	}
	if seen {
		t.Error("expected item to not be seen")
	}

	// Mark seen
	if err := repo.MarkItemSeen(ctx, feed.ID, "guid-1"); err != nil {
		t.Fatalf("failed to mark seen: %v", err)
	}

	seen, err = repo.IsItemSeen(ctx, feed.ID, "guid-1")
	if err != nil {
		t.Fatalf("failed to check seen: %v", err)
	}
	if !seen {
		t.Error("expected item to be seen")
	}

	// Add more seen items and test rewind (unmark seen)
	if err := repo.MarkItemSeen(ctx, feed.ID, "guid-2"); err != nil {
		t.Fatalf("failed to mark seen: %v", err)
	}
	if err := repo.MarkItemSeen(ctx, feed.ID, "guid-3"); err != nil {
		t.Fatalf("failed to mark seen: %v", err)
	}

	// Unmark last 2 seen items (guid-2, guid-3)
	if err := repo.UnmarkSeenItems(ctx, feed.ID, 2); err != nil {
		t.Fatalf("failed to unmark seen: %v", err)
	}

	seen1, _ := repo.IsItemSeen(ctx, feed.ID, "guid-1")
	seen2, _ := repo.IsItemSeen(ctx, feed.ID, "guid-2")
	seen3, _ := repo.IsItemSeen(ctx, feed.ID, "guid-3")

	if !seen1 {
		t.Error("expected guid-1 to remain seen")
	}
	if seen2 || seen3 {
		t.Errorf("expected guid-2 and guid-3 to be unseen, got seen2=%v seen3=%v", seen2, seen3)
	}
}

func TestOutboxOperations(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().Round(time.Second).UTC()
	attemptTimeInit := now.Add(-time.Hour)

	item := &types.OutboxItem{
		Subject:       "Test Subject",
		Body:          "<h1>Test Body</h1>",
		Recipients:    []string{"user1@test.com", "user2@test.com"},
		Status:        types.OutboxPending,
		NextAttemptAt: now,
		LastAttemptAt: &attemptTimeInit,
	}

	// Negative get check
	_, err := repo.GetOutboxItem(ctx, 9999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	err = repo.UpdateOutboxItemStatus(ctx, &types.OutboxItem{ID: 9999})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}

	// Enqueue
	if err := repo.EnqueueOutboxItem(ctx, item); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("expected outbox ID to be populated")
	}

	// Get
	fetched, err := repo.GetOutboxItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("failed to get outbox item: %v", err)
	}
	if fetched.Subject != item.Subject || fetched.Body != item.Body {
		t.Errorf("fetched mismatch: %+v vs %+v", fetched, item)
	}
	if len(fetched.Recipients) != 2 || fetched.Recipients[0] != "user1@test.com" || fetched.Recipients[1] != "user2@test.com" {
		t.Errorf("fetched recipients mismatch: %v", fetched.Recipients)
	}
	if fetched.LastAttemptAt == nil || !fetched.LastAttemptAt.Equal(attemptTimeInit) {
		t.Errorf("expected initial LastAttemptAt %v, got %v", attemptTimeInit, fetched.LastAttemptAt)
	}

	// List Pending
	pending, err := repo.ListPendingOutboxItems(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatalf("failed to list pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != item.ID {
		t.Errorf("expected pending list to contain item, got %+v", pending)
	}

	// Update Status
	fetched.Status = types.OutboxDelivered
	fetched.RetryCount = 1
	fetched.LastError = "No error"
	attemptTime := now.Add(time.Second * 5)
	fetched.LastAttemptAt = &attemptTime

	if err := repo.UpdateOutboxItemStatus(ctx, fetched); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	updated, err := repo.GetOutboxItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("failed to get updated item: %v", err)
	}
	if updated.Status != types.OutboxDelivered {
		t.Errorf("expected status to be delivered, got %s", updated.Status)
	}
	if updated.LastAttemptAt == nil || !updated.LastAttemptAt.Equal(attemptTime) {
		t.Errorf("expected attempt time to match %v, got %v", attemptTime, updated.LastAttemptAt)
	}
}

func TestTransactionRollback(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	// Attempt a transactional operations write, but throw error in callback to force rollback
	err := repo.WithTx(ctx, func(txRepo *Repository) error {
		user := &types.User{
			Email: "rollback@example.com",
		}
		if err := txRepo.CreateUser(ctx, user); err != nil {
			return err
		}
		return errors.New("simulated error")
	})

	if err == nil || err.Error() != "simulated error" {
		t.Fatalf("expected simulated error, got: %v", err)
	}

	// Confirm user was not created (rolled back)
	_, err = repo.GetUserByEmail(ctx, "rollback@example.com")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected user to be missing (rolled back), got err: %v", err)
	}
}
