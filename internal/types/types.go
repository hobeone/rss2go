package types

import (
	"time"
)

// ExtractionStrategy defines how full article content is extracted.
type ExtractionStrategy string

const (
	StrategySummary   ExtractionStrategy = "summary"   // Fallback to feed item summary/excerpt
	StrategyHeuristic ExtractionStrategy = "heuristic" // Automated reader mode heuristic
	StrategySelector  ExtractionStrategy = "selector"  // Target structural element via CSS selector
	StrategyCss       ExtractionStrategy = "css"       // Alias for StrategySelector (legacy UI value)
)

// Feed represents a tracked RSS/Atom feed source.
type Feed struct {
	ID                 int64              `json:"id"`
	Title              string             `json:"title"`
	URL                string             `json:"url"`
	ETag               string             `json:"etag"`
	LastModified       string             `json:"last_modified"`
	NextPollAt         time.Time          `json:"next_poll_at"`
	PollIntervalSecs   int                `json:"poll_interval_secs"`
	BackoffFactor      float64            `json:"backoff_factor"`
	LastErrorStr       string             `json:"last_error_str,omitempty"`
	LastErrorTime      *time.Time         `json:"last_error_time,omitempty"`
	LastErrorSnippet   string             `json:"last_error_snippet,omitempty"`
	LastPolledAt       *time.Time         `json:"last_polled_at,omitempty"`
	ExtractFullArticle bool               `json:"extract_full_article"`
	ExtractionStrategy ExtractionStrategy `json:"extraction_strategy"`
	CSSSelector        string             `json:"css_selector"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// User represents a recipient of email notifications.
type User struct {
	ID                int64     `json:"id"`
	Email             string    `json:"email"`
	SubscribedFeedIDs []int64   `json:"subscribed_feed_ids"`
	CreatedAt         time.Time `json:"created_at"`
}

// Subscription represents a mapping between a User and a Feed.
type Subscription struct {
	UserID int64 `json:"user_id"`
	FeedID int64 `json:"feed_id"`
}

// SeenItem tracks which feed items have already been processed/emailed.
type SeenItem struct {
	FeedID int64     `json:"feed_id"`
	GUID   string    `json:"guid"`
	SeenAt time.Time `json:"seen_at"`
}

// OutboxStatus defines the state of a pending email.
type OutboxStatus string

const (
	OutboxPending    OutboxStatus = "pending"
	OutboxDelivering OutboxStatus = "delivering"
	OutboxDelivered  OutboxStatus = "delivered"
	OutboxFailed     OutboxStatus = "failed"
)

// OutboxItem represents a message queued for SMTP/sendmail dispatch.
type OutboxItem struct {
	ID            int64        `json:"id"`
	Subject       string       `json:"subject"`
	Body          string       `json:"body"`
	Recipients    []string     `json:"recipients"`
	Status        OutboxStatus `json:"status"`
	RetryCount    int          `json:"retry_count"`
	NextAttemptAt time.Time    `json:"next_attempt_at"`
	LastAttemptAt *time.Time   `json:"last_attempt_at,omitempty"`
	LastError     string       `json:"last_error,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

// DBStats holds high-level telemetry and status counters.
type DBStats struct {
	TotalFeeds      int `json:"total_feeds"`
	TotalUsers      int `json:"total_users"`
	OutboxPending   int `json:"outbox_pending"`
	OutboxFailed    int `json:"outbox_failed"`
	OutboxDelivered int `json:"outbox_delivered"`
}
