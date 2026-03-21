package db

import (
	"context"

	"github.com/hobe/rss2go/internal/models"
)

// Store defines the methods needed to persist data for rss2go.
type Store interface {
	// Feed operations
	GetFeeds(ctx context.Context) ([]models.Feed, error)
	GetFeed(ctx context.Context, id int64) (*models.Feed, error)
	AddFeed(ctx context.Context, url string, title string) (int64, error)
	UpdateFeedLastPoll(ctx context.Context, id int64) error
	UpdateFeedError(ctx context.Context, id int64, code int, snippet string) error

	// User operations
	AddUser(ctx context.Context, email string) (int64, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUsersForFeed(ctx context.Context, feedID int64) ([]models.User, error)

	// Subscription operations
	Subscribe(ctx context.Context, userID int64, feedID int64) error

	// Item operations
	IsSeen(ctx context.Context, feedID int64, guid string) (bool, error)
	MarkSeen(ctx context.Context, feedID int64, guid string) error

	// Lifecycle
	Close() error
}
