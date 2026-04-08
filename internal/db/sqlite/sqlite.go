package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hobeone/rss2go/internal/models"
	_ "modernc.org/sqlite"
)

type Store struct {
	db     *sql.DB
	logger *slog.Logger
}

// New creates a new SQLite store.
func New(dbPath string, logger *slog.Logger) (*Store, error) {
	// Add PRAGMAs to DSN for modernc.org/sqlite to enable WAL mode and handle concurrent writes
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %s, %w", dsn, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging database: %s, %w", dsn, err)
	}

	// Configure connection pool to prevent connection exhaustion and help with write locks
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &Store{
		db:     db,
		logger: logger.With("component", "database"),
	}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// feedColumns is the canonical column list for feed SELECT queries.
const feedColumns = "id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet, full_article, backoff_until, etag, last_modified, extraction_strategy, extraction_config"

// feedNulls holds nullable scan targets for a feed row and applies them to a Feed.
type feedNulls struct {
	lastPoll           sql.NullTime
	lastErrorTime      sql.NullTime
	backoffUntil       sql.NullTime
	lastErrorCode      sql.NullInt64
	lastErrorSnippet   sql.NullString
	etag               sql.NullString
	lastModified       sql.NullString
	extractionStrategy sql.NullString
	extractionConfig   sql.NullString
}

func (n *feedNulls) apply(f *models.Feed) {
	if n.lastPoll.Valid {
		f.LastPoll = n.lastPoll.Time
	}
	if n.lastErrorTime.Valid {
		f.LastErrorTime = n.lastErrorTime.Time
	}
	if n.backoffUntil.Valid {
		f.BackoffUntil = n.backoffUntil.Time
	}
	if n.lastErrorCode.Valid {
		f.LastErrorCode = int(n.lastErrorCode.Int64)
	}
	if n.lastErrorSnippet.Valid {
		f.LastErrorSnippet = n.lastErrorSnippet.String
	}
	if n.etag.Valid {
		f.ETag = n.etag.String
	}
	if n.lastModified.Valid {
		f.LastModified = n.lastModified.String
	}
	if n.extractionStrategy.Valid {
		f.ExtractionStrategy = n.extractionStrategy.String
	}
	if n.extractionConfig.Valid {
		f.ExtractionConfig = n.extractionConfig.String
	}
}

// scanFeedRow scans a single feed from a *sql.Row (QueryRow result).
// Returns nil, nil when no row is found.
func scanFeedRow(row *sql.Row) (*models.Feed, error) {
	var f models.Feed
	var n feedNulls
	err := row.Scan(&f.ID, &f.URL, &f.Title, &n.lastPoll, &n.lastErrorTime, &n.lastErrorCode, &n.lastErrorSnippet, &f.FullArticle, &n.backoffUntil, &n.etag, &n.lastModified, &n.extractionStrategy, &n.extractionConfig)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	n.apply(&f)
	return &f, nil
}

// scanFeedRows scans one feed from an open *sql.Rows cursor.
func scanFeedRows(rows *sql.Rows) (models.Feed, error) {
	var f models.Feed
	var n feedNulls
	if err := rows.Scan(&f.ID, &f.URL, &f.Title, &n.lastPoll, &n.lastErrorTime, &n.lastErrorCode, &n.lastErrorSnippet, &f.FullArticle, &n.backoffUntil, &n.etag, &n.lastModified, &n.extractionStrategy, &n.extractionConfig); err != nil {
		return models.Feed{}, err
	}
	n.apply(&f)
	return f, nil
}

func (s *Store) GetFeeds(ctx context.Context) ([]models.Feed, error) {
	query := "SELECT " + feedColumns + " FROM feeds"
	s.logger.Debug("executing query", "query", query)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	feeds := make([]models.Feed, 0, 32)
	for rows.Next() {
		f, err := scanFeedRows(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *Store) GetFeedsWithErrors(ctx context.Context) ([]models.Feed, error) {
	query := "SELECT " + feedColumns + " FROM feeds WHERE last_error_time IS NOT NULL OR last_error_code != 0"
	s.logger.Debug("executing query", "query", query)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	feeds := make([]models.Feed, 0, 16)
	for rows.Next() {
		f, err := scanFeedRows(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *Store) GetFeed(ctx context.Context, id int64) (*models.Feed, error) {
	query := "SELECT " + feedColumns + " FROM feeds WHERE id = ?"
	s.logger.Debug("executing query", "query", query, "args", []any{id})
	return scanFeedRow(s.db.QueryRowContext(ctx, query, id))
}

func (s *Store) GetFeedByURL(ctx context.Context, url string) (*models.Feed, error) {
	query := "SELECT " + feedColumns + " FROM feeds WHERE url = ?"
	s.logger.Debug("executing query", "query", query, "args", []any{url})
	return scanFeedRow(s.db.QueryRowContext(ctx, query, url))
}

func (s *Store) AddFeed(ctx context.Context, url string, title string, fullArticle bool, extractionStrategy string, extractionConfig string) (int64, error) {
	query := `INSERT INTO feeds (url, title, full_article, extraction_strategy, extraction_config)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			title=excluded.title,
			full_article=excluded.full_article,
			extraction_strategy=excluded.extraction_strategy,
			extraction_config=excluded.extraction_config`
	s.logger.Debug("executing exec", "query", query, "args", []any{url, title, fullArticle, extractionStrategy, extractionConfig})
	res, err := s.db.ExecContext(ctx, query, url, title, fullArticle, extractionStrategy, extractionConfig)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateFeed(ctx context.Context, id int64, url *string, title *string, fullArticle *bool, extractionStrategy *string, extractionConfig *string) error {
	if url == nil && title == nil && fullArticle == nil && extractionStrategy == nil && extractionConfig == nil {
		return nil
	}

	query := "UPDATE feeds SET "
	var args []any
	if url != nil {
		query += "url = ?, "
		args = append(args, *url)
	}
	if title != nil {
		query += "title = ?, "
		args = append(args, *title)
	}
	if fullArticle != nil {
		query += "full_article = ?, "
		args = append(args, *fullArticle)
	}
	if extractionStrategy != nil {
		query += "extraction_strategy = ?, "
		args = append(args, *extractionStrategy)
	}
	if extractionConfig != nil {
		query += "extraction_config = ?, "
		args = append(args, *extractionConfig)
	}
	query = query[:len(query)-2] // Remove trailing comma and space
	query += " WHERE id = ?"
	args = append(args, id)

	s.logger.Debug("executing exec", "query", query, "args", args)
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) DeleteFeed(ctx context.Context, id int64) error {
	query := "DELETE FROM feeds WHERE id = ?"
	s.logger.Debug("executing exec", "query", query, "args", []any{id})
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

func (s *Store) DeleteFeedByURL(ctx context.Context, url string) error {
	query := "DELETE FROM feeds WHERE url = ?"
	s.logger.Debug("executing exec", "query", query, "args", []any{url})
	_, err := s.db.ExecContext(ctx, query, url)
	return err
}

func (s *Store) UpdateFeedLastPoll(ctx context.Context, id int64, etag string, lastModified string) error {
	query := "UPDATE feeds SET last_poll = ?, etag = ?, last_modified = ? WHERE id = ?"
	now := time.Now()
	s.logger.Debug("executing exec", "query", query, "args", []any{now, etag, lastModified, id})
	_, err := s.db.ExecContext(ctx, query, now, etag, lastModified, id)
	return err
}

func (s *Store) SetFeedError(ctx context.Context, id int64, code int, snippet string) error {
	query := "UPDATE feeds SET last_error_time = ?, last_error_code = ?, last_error_snippet = ? WHERE id = ?"
	s.logger.Debug("executing exec", "query", query, "args", []any{time.Now(), code, snippet, id})
	_, err := s.db.ExecContext(ctx, query, time.Now(), code, snippet, id)
	return err
}

func (s *Store) ClearFeedError(ctx context.Context, id int64) error {
	query := "UPDATE feeds SET last_error_time = NULL, last_error_code = NULL, last_error_snippet = NULL WHERE id = ?"
	s.logger.Debug("executing exec", "query", query, "args", []any{id})
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

func (s *Store) UpdateFeedBackoff(ctx context.Context, id int64, backoffUntil time.Time) error {
	var query string
	var args []any
	if backoffUntil.IsZero() {
		query = "UPDATE feeds SET backoff_until = NULL WHERE id = ?"
		args = []any{id}
	} else {
		query = "UPDATE feeds SET backoff_until = ? WHERE id = ?"
		args = []any{backoffUntil, id}
	}
	s.logger.Debug("executing exec", "query", query, "args", args)
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) AddUser(ctx context.Context, email string) (int64, error) {
	query := "INSERT INTO users (email) VALUES (?) ON CONFLICT(email) DO NOTHING"
	s.logger.Debug("executing exec", "query", query, "args", []any{email})
	res, err := s.db.ExecContext(ctx, query, email)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id != 0 {
		return id, nil
	}
	// ON CONFLICT DO NOTHING returns id=0 when the row already exists; look it up.
	u, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	if u == nil {
		return 0, fmt.Errorf("user %q not found after upsert", email)
	}
	return u.ID, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := "SELECT id, email FROM users WHERE email = ?"
	s.logger.Debug("executing query", "query", query, "args", []any{email})
	var u models.User
	err := s.db.QueryRowContext(ctx, query, email).Scan(&u.ID, &u.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUsersForFeed(ctx context.Context, feedID int64) ([]models.User, error) {
	query := `
		SELECT u.id, u.email 
		FROM users u 
		JOIN subscriptions s ON u.id = s.user_id 
		WHERE s.feed_id = ?`
	s.logger.Debug("executing query", "query", query, "args", []any{feedID})
	rows, err := s.db.QueryContext(ctx, query, feedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]models.User, 0, 8)
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) Subscribe(ctx context.Context, userID int64, feedID int64) error {
	query := "INSERT INTO subscriptions (user_id, feed_id) VALUES (?, ?) ON CONFLICT DO NOTHING"
	s.logger.Debug("executing exec", "query", query, "args", []any{userID, feedID})
	_, err := s.db.ExecContext(ctx, query, userID, feedID)
	return err
}

func (s *Store) Unsubscribe(ctx context.Context, userID int64, feedID int64) error {
	query := "DELETE FROM subscriptions WHERE user_id = ? AND feed_id = ?"
	s.logger.Debug("executing exec", "query", query, "args", []any{userID, feedID})
	_, err := s.db.ExecContext(ctx, query, userID, feedID)
	return err
}

func (s *Store) EnqueueEmail(ctx context.Context, recipients []string, subject, body string) error {
	query := "INSERT INTO outbox (recipients, subject, body) VALUES (?, ?, ?)"
	joined := strings.Join(recipients, "\n")
	s.logger.Debug("executing exec", "query", query)
	_, err := s.db.ExecContext(ctx, query, joined, subject, body)
	return err
}

// ClaimPendingEmail atomically claims one pending outbox row by transitioning
// it to status='delivering'. Returns nil, nil when the queue is empty.
func (s *Store) ClaimPendingEmail(ctx context.Context) (*models.OutboxEntry, error) {
	query := `UPDATE outbox SET status = 'delivering'
WHERE id = (SELECT id FROM outbox WHERE status = 'pending' ORDER BY created_at LIMIT 1)
RETURNING id, recipients, subject, body, created_at`
	s.logger.Debug("executing query", "query", query)
	var e models.OutboxEntry
	var joined string
	var createdAt sql.NullTime
	err := s.db.QueryRowContext(ctx, query).Scan(&e.ID, &joined, &e.Subject, &e.Body, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	e.Recipients = strings.Split(joined, "\n")
	if createdAt.Valid {
		e.CreatedAt = createdAt.Time
	}
	e.Status = "delivering"
	return &e, nil
}

func (s *Store) MarkEmailDelivered(ctx context.Context, id int64) error {
	query := "UPDATE outbox SET status = 'delivered', delivered_at = ? WHERE id = ?"
	s.logger.Debug("executing exec", "query", query, "args", []any{time.Now(), id})
	_, err := s.db.ExecContext(ctx, query, time.Now(), id)
	return err
}

func (s *Store) ResetEmailToPending(ctx context.Context, id int64) error {
	query := "UPDATE outbox SET status = 'pending' WHERE id = ?"
	s.logger.Debug("executing exec", "query", query, "args", []any{id})
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// ResetDeliveringToPending resets any rows stuck in 'delivering' status back
// to 'pending'. Called once on startup to recover from a prior crash.
func (s *Store) ResetDeliveringToPending(ctx context.Context) error {
	query := "UPDATE outbox SET status = 'pending' WHERE status = 'delivering'"
	s.logger.Debug("executing exec", "query", query)
	_, err := s.db.ExecContext(ctx, query)
	return err
}

func (s *Store) IsSeen(ctx context.Context, feedID int64, guid string) (bool, error) {
	query := "SELECT EXISTS(SELECT 1 FROM seen_items WHERE feed_id = ? AND guid = ?)"
	s.logger.Debug("executing query", "query", query, "args", []any{feedID, guid})
	var exists bool
	err := s.db.QueryRowContext(ctx, query, feedID, guid).Scan(&exists)
	return exists, err
}

func (s *Store) MarkSeen(ctx context.Context, feedID int64, guid string) error {
	query := "INSERT INTO seen_items (feed_id, guid) VALUES (?, ?) ON CONFLICT DO NOTHING"
	s.logger.Debug("executing exec", "query", query, "args", []any{feedID, guid})
	_, err := s.db.ExecContext(ctx, query, feedID, guid)
	return err
}

// UnseenRecentItems removes the N most recently seen items for a feed,
// ordered by seen_at DESC. Returns the GUIDs of the deleted rows.
func (s *Store) UnseenRecentItems(ctx context.Context, feedID int64, n int) ([]string, error) {
	query := `DELETE FROM seen_items WHERE feed_id = ? AND guid IN (
		SELECT guid FROM seen_items WHERE feed_id = ? ORDER BY seen_at DESC LIMIT ?
	) RETURNING guid`
	s.logger.Debug("executing query", "query", query, "args", []any{feedID, feedID, n})
	rows, err := s.db.QueryContext(ctx, query, feedID, feedID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guids []string
	for rows.Next() {
		var guid string
		if err := rows.Scan(&guid); err != nil {
			return nil, err
		}
		guids = append(guids, guid)
	}
	return guids, rows.Err()
}

// ResetFeedPoll sets last_poll to NULL so the scheduler treats the feed as
// never-polled and dispatches a crawl on the next resync cycle.
func (s *Store) ResetFeedPoll(ctx context.Context, id int64) error {
	query := "UPDATE feeds SET last_poll = NULL WHERE id = ?"
	s.logger.Debug("executing exec", "query", query, "args", []any{id})
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}
