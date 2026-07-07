package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"rss2go/internal/types"
)

// DBTX abstraction allows repository methods to run on *sql.DB or *sql.Tx.
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Repository handles persistence operations against the SQLite database.
type Repository struct {
	db DBTX
}

// NewRepository creates a new Repository instance.
func NewRepository(db DBTX) *Repository {
	return &Repository{db: db}
}

// WithTx wraps operations within a single SQL transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(*Repository) error) error {
	db, ok := r.db.(*sql.DB)
	if !ok {
		return fmt.Errorf("database: repository not backed by standard *sql.DB")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: begin tx failed: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	txRepo := NewRepository(tx)
	if err := fn(txRepo); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: commit tx failed: %w", err)
	}

	return nil
}

// ============================================================================
// Feed Operations
// ============================================================================

func (r *Repository) CreateFeed(ctx context.Context, f *types.Feed) error {
	query := `
		INSERT INTO feeds (
			title, url, etag, last_modified, next_poll_at, 
			poll_interval_secs, backoff_factor, last_error_str, 
			last_error_time, last_error_snippet, last_polled_at, extract_full_article, 
			extraction_strategy, css_selector
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	var errTime *time.Time
	if f.LastErrorTime != nil {
		errTime = f.LastErrorTime
	}

	var polledTime *time.Time
	if f.LastPolledAt != nil {
		polledTime = f.LastPolledAt
	}

	extractVal := 0
	if f.ExtractFullArticle {
		extractVal = 1
	}

	res, err := r.db.ExecContext(
		ctx, query,
		f.Title, f.URL, f.ETag, f.LastModified, f.NextPollAt,
		f.PollIntervalSecs, f.BackoffFactor, f.LastErrorStr,
		errTime, f.LastErrorSnippet, polledTime, extractVal,
		string(f.ExtractionStrategy), f.CSSSelector,
	)
	if err != nil {
		return fmt.Errorf("repository: create feed: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("repository: get feed insert id: %w", err)
	}
	f.ID = id
	return nil
}

func (r *Repository) GetFeed(ctx context.Context, id int64) (*types.Feed, error) {
	query := `
		SELECT 
			id, title, url, etag, last_modified, next_poll_at, 
			poll_interval_secs, backoff_factor, last_error_str, 
			last_error_time, last_error_snippet, last_polled_at, extract_full_article, 
			extraction_strategy, css_selector, created_at, updated_at
		FROM feeds
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanFeed(row)
}

func (r *Repository) GetFeedByURL(ctx context.Context, url string) (*types.Feed, error) {
	query := `
		SELECT 
			id, title, url, etag, last_modified, next_poll_at, 
			poll_interval_secs, backoff_factor, last_error_str, 
			last_error_time, last_error_snippet, last_polled_at, extract_full_article, 
			extraction_strategy, css_selector, created_at, updated_at
		FROM feeds
		WHERE url = ?
	`
	row := r.db.QueryRowContext(ctx, query, url)
	return scanFeed(row)
}

func (r *Repository) UpdateFeed(ctx context.Context, f *types.Feed) error {
	query := `
		UPDATE feeds SET 
			title = ?, url = ?, etag = ?, last_modified = ?, next_poll_at = ?, 
			poll_interval_secs = ?, backoff_factor = ?, last_error_str = ?, 
			last_error_time = ?, last_error_snippet = ?, last_polled_at = ?, extract_full_article = ?, 
			extraction_strategy = ?, css_selector = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	extractVal := 0
	if f.ExtractFullArticle {
		extractVal = 1
	}

	var polledTime *time.Time
	if f.LastPolledAt != nil {
		polledTime = f.LastPolledAt
	}

	res, err := r.db.ExecContext(
		ctx, query,
		f.Title, f.URL, f.ETag, f.LastModified, f.NextPollAt,
		f.PollIntervalSecs, f.BackoffFactor, f.LastErrorStr,
		f.LastErrorTime, f.LastErrorSnippet, polledTime, extractVal,
		string(f.ExtractionStrategy), f.CSSSelector,
		f.ID,
	)
	if err != nil {
		return fmt.Errorf("repository: update feed: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository: check rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteFeed(ctx context.Context, id int64) error {
	query := `DELETE FROM feeds WHERE id = ?`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("repository: delete feed: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository: check rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListFeeds(ctx context.Context) ([]*types.Feed, error) {
	query := `
		SELECT 
			id, title, url, etag, last_modified, next_poll_at, 
			poll_interval_secs, backoff_factor, last_error_str, 
			last_error_time, last_error_snippet, last_polled_at, extract_full_article, 
			extraction_strategy, css_selector, created_at, updated_at
		FROM feeds
		ORDER BY title ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("repository: list feeds: %w", err)
	}
	defer func() { _ = rows.Close() }()

	feeds := []*types.Feed{}
	for rows.Next() {
		f, err := scanFeedRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repository: scan feed row: %w", err)
		}
		feeds = append(feeds, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: rows error: %w", err)
	}

	return feeds, nil
}

func (r *Repository) ListFeedsDue(ctx context.Context, now time.Time) ([]*types.Feed, error) {
	query := `
		SELECT 
			id, title, url, etag, last_modified, next_poll_at, 
			poll_interval_secs, backoff_factor, last_error_str, 
			last_error_time, last_error_snippet, last_polled_at, extract_full_article, 
			extraction_strategy, css_selector, created_at, updated_at
		FROM feeds
		WHERE next_poll_at <= ?
		ORDER BY next_poll_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, now)
	if err != nil {
		return nil, fmt.Errorf("repository: list due feeds: %w", err)
	}
	defer func() { _ = rows.Close() }()

	feeds := []*types.Feed{}
	for rows.Next() {
		f, err := scanFeedRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repository: scan feed row: %w", err)
		}
		feeds = append(feeds, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: rows error: %w", err)
	}

	return feeds, nil
}

// ============================================================================
// User Operations
// ============================================================================

func (r *Repository) CreateUser(ctx context.Context, u *types.User) error {
	query := `INSERT INTO users (email) VALUES (?)`
	res, err := r.db.ExecContext(ctx, query, u.Email)
	if err != nil {
		return fmt.Errorf("repository: create user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("repository: get user insert id: %w", err)
	}
	u.ID = id
	return nil
}

func (r *Repository) GetUser(ctx context.Context, id int64) (*types.User, error) {
	query := `SELECT id, email, created_at FROM users WHERE id = ?`
	row := r.db.QueryRowContext(ctx, query, id)
	var u types.User
	if err := row.Scan(&u.ID, &u.Email, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("repository: get user: %w", err)
	}
	ids, err := r.getSubscribedFeedIDs(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("repository: get user subscription ids: %w", err)
	}
	u.SubscribedFeedIDs = ids
	return &u, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	query := `SELECT id, email, created_at FROM users WHERE email = ?`
	row := r.db.QueryRowContext(ctx, query, email)
	var u types.User
	if err := row.Scan(&u.ID, &u.Email, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("repository: get user by email: %w", err)
	}
	ids, err := r.getSubscribedFeedIDs(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("repository: get user subscription ids: %w", err)
	}
	u.SubscribedFeedIDs = ids
	return &u, nil
}

func (r *Repository) DeleteUser(ctx context.Context, id int64) error {
	query := `DELETE FROM users WHERE id = ?`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("repository: delete user: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository: check rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (r *Repository) getSubscribedFeedIDs(ctx context.Context, userID int64) ([]int64, error) {
	query := `SELECT feed_id FROM subscriptions WHERE user_id = ?`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []int64{}
	}
	return ids, nil
}

func (r *Repository) ListUsers(ctx context.Context) ([]*types.User, error) {
	query := `SELECT id, email, created_at FROM users ORDER BY email ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("repository: list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := []*types.User{}
	for rows.Next() {
		var u types.User
		if err := rows.Scan(&u.ID, &u.Email, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("repository: scan user: %w", err)
		}
		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: rows error: %w", err)
	}
	_ = rows.Close()

	for _, u := range users {
		ids, err := r.getSubscribedFeedIDs(ctx, u.ID)
		if err != nil {
			return nil, fmt.Errorf("repository: get user subscription ids: %w", err)
		}
		u.SubscribedFeedIDs = ids
	}

	return users, nil
}

// ============================================================================
// Subscription Operations
// ============================================================================

func (r *Repository) Subscribe(ctx context.Context, userID, feedID int64) error {
	query := `INSERT INTO subscriptions (user_id, feed_id) VALUES (?, ?) ON CONFLICT DO NOTHING`
	_, err := r.db.ExecContext(ctx, query, userID, feedID)
	if err != nil {
		return fmt.Errorf("repository: subscribe: %w", err)
	}
	return nil
}

func (r *Repository) Unsubscribe(ctx context.Context, userID, feedID int64) error {
	query := `DELETE FROM subscriptions WHERE user_id = ? AND feed_id = ?`
	res, err := r.db.ExecContext(ctx, query, userID, feedID)
	if err != nil {
		return fmt.Errorf("repository: unsubscribe: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository: check rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListSubscriptionsForUser(ctx context.Context, userID int64) ([]*types.Feed, error) {
	query := `
		SELECT 
			f.id, f.title, f.url, f.etag, f.last_modified, f.next_poll_at, 
			f.poll_interval_secs, f.backoff_factor, f.last_error_str, 
			f.last_error_time, f.last_error_snippet, f.last_polled_at, f.extract_full_article, 
			f.extraction_strategy, f.css_selector, f.created_at, f.updated_at
		FROM feeds f
		JOIN subscriptions s ON f.id = s.feed_id
		WHERE s.user_id = ?
		ORDER BY f.title ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("repository: list user subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	feeds := []*types.Feed{}
	for rows.Next() {
		f, err := scanFeedRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repository: scan feed row: %w", err)
		}
		feeds = append(feeds, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: rows error: %w", err)
	}

	return feeds, nil
}

func (r *Repository) ListSubscriptionsForFeed(ctx context.Context, feedID int64) ([]*types.User, error) {
	query := `
		SELECT u.id, u.email, u.created_at
		FROM users u
		JOIN subscriptions s ON u.id = s.user_id
		WHERE s.feed_id = ?
		ORDER BY u.email ASC
	`
	rows, err := r.db.QueryContext(ctx, query, feedID)
	if err != nil {
		return nil, fmt.Errorf("repository: list feed subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := []*types.User{}
	for rows.Next() {
		var u types.User
		if err := rows.Scan(&u.ID, &u.Email, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("repository: scan user: %w", err)
		}
		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: rows error: %w", err)
	}
	_ = rows.Close()

	for _, u := range users {
		ids, err := r.getSubscribedFeedIDs(ctx, u.ID)
		if err != nil {
			return nil, fmt.Errorf("repository: get user subscription ids: %w", err)
		}
		u.SubscribedFeedIDs = ids
	}

	return users, nil
}

// ============================================================================
// Seen Items Operations
// ============================================================================

func (r *Repository) MarkItemSeen(ctx context.Context, feedID int64, guid string) error {
	query := `INSERT INTO seen_items (feed_id, guid) VALUES (?, ?) ON CONFLICT DO NOTHING`
	_, err := r.db.ExecContext(ctx, query, feedID, guid)
	if err != nil {
		return fmt.Errorf("repository: mark item seen: %w", err)
	}
	return nil
}

func (r *Repository) IsItemSeen(ctx context.Context, feedID int64, guid string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM seen_items WHERE feed_id = ? AND guid = ?)`
	var exists int
	err := r.db.QueryRowContext(ctx, query, feedID, guid).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("repository: check item seen: %w", err)
	}
	return exists == 1, nil
}

func (r *Repository) UnmarkSeenItems(ctx context.Context, feedID int64, limit int) error {
	query := `
		DELETE FROM seen_items 
		WHERE feed_id = ? 
		AND guid IN (
			SELECT guid FROM seen_items 
			WHERE feed_id = ? 
			ORDER BY rowid DESC 
			LIMIT ?
		)
	`
	_, err := r.db.ExecContext(ctx, query, feedID, feedID, limit)
	if err != nil {
		return fmt.Errorf("repository: unmark seen items: %w", err)
	}
	return nil
}

// ============================================================================
// Outbox Queue Operations
// ============================================================================

func (r *Repository) EnqueueOutboxItem(ctx context.Context, item *types.OutboxItem) error {
	query := `
		INSERT INTO outbox (
			subject, body, status, retry_count, next_attempt_at, 
			last_attempt_at, last_error
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	var lastAttempt *time.Time
	if item.LastAttemptAt != nil {
		lastAttempt = item.LastAttemptAt
	}

	res, err := r.db.ExecContext(
		ctx, query,
		item.Subject, item.Body, string(item.Status), item.RetryCount,
		item.NextAttemptAt, lastAttempt, item.LastError,
	)
	if err != nil {
		return fmt.Errorf("repository: enqueue outbox item: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("repository: get outbox insert id: %w", err)
	}
	item.ID = id

	recipientQuery := `INSERT INTO outbox_recipients (outbox_id, email) VALUES (?, ?)`
	for _, email := range item.Recipients {
		_, err = r.db.ExecContext(ctx, recipientQuery, item.ID, email)
		if err != nil {
			return fmt.Errorf("repository: insert recipient: %w", err)
		}
	}

	return nil
}

func (r *Repository) GetOutboxItem(ctx context.Context, id int64) (*types.OutboxItem, error) {
	query := `
		SELECT id, subject, body, status, retry_count, next_attempt_at, last_attempt_at, last_error, created_at 
		FROM outbox 
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	var item types.OutboxItem
	var statusStr string
	var lastAttempt sql.NullTime

	err := row.Scan(
		&item.ID, &item.Subject, &item.Body, &statusStr, &item.RetryCount,
		&item.NextAttemptAt, &lastAttempt, &item.LastError, &item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("repository: get outbox item: %w", err)
	}
	item.Status = types.OutboxStatus(statusStr)
	if lastAttempt.Valid {
		item.LastAttemptAt = &lastAttempt.Time
	}

	// Fetch recipients
	recipQuery := `SELECT email FROM outbox_recipients WHERE outbox_id = ? ORDER BY email ASC`
	rows, err := r.db.QueryContext(ctx, recipQuery, id)
	if err != nil {
		return nil, fmt.Errorf("repository: get outbox recipients: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("repository: scan recipient: %w", err)
		}
		item.Recipients = append(item.Recipients, email)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: rows error: %w", err)
	}

	return &item, nil
}

func (r *Repository) ListPendingOutboxItems(ctx context.Context, now time.Time) ([]*types.OutboxItem, error) {
	query := `
		SELECT id, subject, body, status, retry_count, next_attempt_at, last_attempt_at, last_error, created_at 
		FROM outbox 
		WHERE status IN ('pending', 'failed') AND next_attempt_at <= ? 
		ORDER BY next_attempt_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, now)
	if err != nil {
		return nil, fmt.Errorf("repository: list pending outbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []*types.OutboxItem{}
	for rows.Next() {
		var item types.OutboxItem
		var statusStr string
		var lastAttempt sql.NullTime

		err := rows.Scan(
			&item.ID, &item.Subject, &item.Body, &statusStr, &item.RetryCount,
			&item.NextAttemptAt, &lastAttempt, &item.LastError, &item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("repository: scan outbox row: %w", err)
		}
		item.Status = types.OutboxStatus(statusStr)
		if lastAttempt.Valid {
			item.LastAttemptAt = &lastAttempt.Time
		}
		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: rows error: %w", err)
	}

	// Fetch recipients for all items
	for _, item := range items {
		recipQuery := `SELECT email FROM outbox_recipients WHERE outbox_id = ? ORDER BY email ASC`
		recipRows, err := r.db.QueryContext(ctx, recipQuery, item.ID)
		if err != nil {
			return nil, fmt.Errorf("repository: get outbox recipients: %w", err)
		}

		for recipRows.Next() {
			var email string
			if err := recipRows.Scan(&email); err != nil {
				_ = recipRows.Close()
				return nil, fmt.Errorf("repository: scan recipient: %w", err)
			}
			item.Recipients = append(item.Recipients, email)
		}
		_ = recipRows.Close()

		if err := recipRows.Err(); err != nil {
			return nil, fmt.Errorf("repository: rows error: %w", err)
		}
	}

	return items, nil
}

func (r *Repository) UpdateOutboxItemStatus(ctx context.Context, item *types.OutboxItem) error {
	query := `
		UPDATE outbox SET 
			status = ?, retry_count = ?, next_attempt_at = ?, 
			last_attempt_at = ?, last_error = ? 
		WHERE id = ?
	`
	var lastAttempt *time.Time
	if item.LastAttemptAt != nil {
		lastAttempt = item.LastAttemptAt
	}

	res, err := r.db.ExecContext(
		ctx, query,
		string(item.Status), item.RetryCount, item.NextAttemptAt,
		lastAttempt, item.LastError, item.ID,
	)
	if err != nil {
		return fmt.Errorf("repository: update outbox status: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository: check rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListOutboxItems(ctx context.Context, limit int) ([]*types.OutboxItem, error) {
	query := `
		SELECT id, subject, body, status, retry_count, next_attempt_at, last_attempt_at, last_error, created_at 
		FROM outbox 
		ORDER BY id DESC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("repository: list outbox items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []*types.OutboxItem{}
	for rows.Next() {
		var item types.OutboxItem
		var statusStr string
		var lastAttempt sql.NullTime

		err := rows.Scan(
			&item.ID, &item.Subject, &item.Body, &statusStr, &item.RetryCount,
			&item.NextAttemptAt, &lastAttempt, &item.LastError, &item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("repository: scan outbox row: %w", err)
		}
		item.Status = types.OutboxStatus(statusStr)
		if lastAttempt.Valid {
			item.LastAttemptAt = &lastAttempt.Time
		}
		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: rows error: %w", err)
	}

	// Fetch recipients for all items
	for _, item := range items {
		recipQuery := `SELECT email FROM outbox_recipients WHERE outbox_id = ? ORDER BY email ASC`
		recipRows, err := r.db.QueryContext(ctx, recipQuery, item.ID)
		if err != nil {
			return nil, fmt.Errorf("repository: get outbox recipients: %w", err)
		}

		for recipRows.Next() {
			var email string
			if err := recipRows.Scan(&email); err != nil {
				_ = recipRows.Close()
				return nil, fmt.Errorf("repository: scan recipient: %w", err)
			}
			item.Recipients = append(item.Recipients, email)
		}
		_ = recipRows.Close()

		if err := recipRows.Err(); err != nil {
			return nil, fmt.Errorf("repository: recipients rows error: %w", err)
		}
	}

	return items, nil
}

func (r *Repository) GetStats(ctx context.Context) (*types.DBStats, error) {
	var stats types.DBStats
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feeds").Scan(&stats.TotalFeeds)
	if err != nil {
		return nil, fmt.Errorf("repository: get stats total feeds: %w", err)
	}
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("repository: get stats total users: %w", err)
	}
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM outbox WHERE status = 'pending'").Scan(&stats.OutboxPending)
	if err != nil {
		return nil, fmt.Errorf("repository: get stats outbox pending: %w", err)
	}
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM outbox WHERE status = 'failed'").Scan(&stats.OutboxFailed)
	if err != nil {
		return nil, fmt.Errorf("repository: get stats outbox failed: %w", err)
	}
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM outbox WHERE status = 'delivered'").Scan(&stats.OutboxDelivered)
	if err != nil {
		return nil, fmt.Errorf("repository: get stats outbox delivered: %w", err)
	}
	return &stats, nil
}

// ============================================================================
// Internal Helper Functions
// ============================================================================

func scanFeed(row *sql.Row) (*types.Feed, error) {
	var f types.Feed
	var errTime sql.NullTime
	var polledTime sql.NullTime
	var extractVal int
	var strategyStr string

	err := row.Scan(
		&f.ID, &f.Title, &f.URL, &f.ETag, &f.LastModified, &f.NextPollAt,
		&f.PollIntervalSecs, &f.BackoffFactor, &f.LastErrorStr,
		&errTime, &f.LastErrorSnippet, &polledTime, &extractVal,
		&strategyStr, &f.CSSSelector, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("repository: scan feed: %w", err)
	}

	f.ExtractFullArticle = extractVal == 1
	f.ExtractionStrategy = types.ExtractionStrategy(strategyStr)
	if errTime.Valid {
		f.LastErrorTime = &errTime.Time
	}
	if polledTime.Valid {
		f.LastPolledAt = &polledTime.Time
	}

	return &f, nil
}

func scanFeedRow(rows *sql.Rows) (*types.Feed, error) {
	var f types.Feed
	var errTime sql.NullTime
	var polledTime sql.NullTime
	var extractVal int
	var strategyStr string

	err := rows.Scan(
		&f.ID, &f.Title, &f.URL, &f.ETag, &f.LastModified, &f.NextPollAt,
		&f.PollIntervalSecs, &f.BackoffFactor, &f.LastErrorStr,
		&errTime, &f.LastErrorSnippet, &polledTime, &extractVal,
		&strategyStr, &f.CSSSelector, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("repository: scan feed row: %w", err)
	}

	f.ExtractFullArticle = extractVal == 1
	f.ExtractionStrategy = types.ExtractionStrategy(strategyStr)
	if errTime.Valid {
		f.LastErrorTime = &errTime.Time
	}
	if polledTime.Valid {
		f.LastPolledAt = &polledTime.Time
	}

	return &f, nil
}
