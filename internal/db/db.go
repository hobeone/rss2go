package db

import (
	"context"
	"time"

	"github.com/hobeone/rss2go/internal/models"
)

// Store defines the methods needed to persist data for rss2go.
type Store interface {
	// Feed operations
	GetFeeds(ctx context.Context) ([]models.Feed, error)
	GetFeedsWithErrors(ctx context.Context) ([]models.Feed, error)
	GetFeed(ctx context.Context, id int64) (*models.Feed, error)
	GetFeedByURL(ctx context.Context, url string) (*models.Feed, error)
	AddFeed(ctx context.Context, url string, title string, fullArticle bool, extractionStrategy string, extractionConfig string) (int64, error)
	UpdateFeed(ctx context.Context, id int64, url *string, title *string, fullArticle *bool, extractionStrategy *string, extractionConfig *string) error
	DeleteFeed(ctx context.Context, id int64) error
	DeleteFeedByURL(ctx context.Context, url string) error
	UpdateFeedLastPoll(ctx context.Context, id int64, etag string, lastModified string) error
	SetFeedError(ctx context.Context, id int64, code int, snippet string) error
	ClearFeedError(ctx context.Context, id int64) error
	UpdateFeedBackoff(ctx context.Context, id int64, backoffUntil time.Time) error

	// User operations
	AddUser(ctx context.Context, email string) (int64, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUsersForFeed(ctx context.Context, feedID int64) ([]models.User, error)

	// Subscription operations
	Subscribe(ctx context.Context, userID int64, feedID int64) error
	Unsubscribe(ctx context.Context, userID int64, feedID int64) error

	// Item operations
	IsSeen(ctx context.Context, feedID int64, guid string) (bool, error)
	MarkSeen(ctx context.Context, feedID int64, guid string) error

	// Outbox operations
	EnqueueEmail(ctx context.Context, recipients []string, subject, body string) error
	ClaimPendingEmail(ctx context.Context) (*models.OutboxEntry, error)
	MarkEmailDelivered(ctx context.Context, id int64) error
	ResetEmailToPending(ctx context.Context, id int64) error
	ResetDeliveringToPending(ctx context.Context) error

	// Lifecycle
	Close() error
}
