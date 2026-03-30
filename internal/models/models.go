package models

import "time"

// Feed represents an RSS feed to be polled.
type Feed struct {
	ID             int64     `json:"id"`
	URL            string    `json:"url"`
	Title          string    `json:"title"`
	LastPoll       time.Time `json:"last_poll"`
	LastErrorTime  time.Time `json:"last_error_time"`
	LastErrorCode  int       `json:"last_error_code"`
	LastErrorSnippet string    `json:"last_error_snippet"`
	FullArticle      bool      `json:"full_article"`
	ETag             string    `json:"etag"`
	LastModified     string    `json:"last_modified"`
}

// User represents a subscriber.
type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// Subscription links a user to a feed.
type Subscription struct {
	UserID int64 `json:"user_id"`
	FeedID int64 `json:"feed_id"`
}

// Item represents a single RSS entry.
type Item struct {
	FeedID      int64     `json:"feed_id"`
	GUID        string    `json:"guid"`
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	Description string    `json:"description"`
	PublishedAt time.Time `json:"published_at"`
}
