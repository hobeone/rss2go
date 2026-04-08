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
	FullArticle        bool      `json:"full_article"`
	ExtractionStrategy string    `json:"extraction_strategy"`
	ExtractionConfig   string    `json:"extraction_config"`
	ETag               string    `json:"etag"`
	LastModified     string    `json:"last_modified"`
	BackoffUntil     time.Time `json:"backoff_until"`
}

// User represents a subscriber.
type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// OutboxEntry is a pending or delivered email in the transactional outbox.
type OutboxEntry struct {
	ID          int64     `json:"id"`
	Recipients  []string  `json:"recipients"`
	Subject     string    `json:"subject"`
	Body        string    `json:"body"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	DeliveredAt time.Time `json:"delivered_at"`
}
