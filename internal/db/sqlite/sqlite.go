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

func (s *Store) GetFeeds(ctx context.Context) ([]models.Feed, error) {
	query := "SELECT id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet, full_article, backoff_until FROM feeds"
	s.logger.Debug("executing query", "query", query)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	feeds := make([]models.Feed, 0, 32)
	for rows.Next() {
		var f models.Feed
		var lastPoll, lastErrorTime, backoffUntil sql.NullTime
		var lastErrorCode sql.NullInt64
		var lastErrorSnippet sql.NullString
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &lastErrorTime, &lastErrorCode, &lastErrorSnippet, &f.FullArticle, &backoffUntil); err != nil {
			return nil, err
		}
		if lastPoll.Valid {
			f.LastPoll = lastPoll.Time
		}
		if lastErrorTime.Valid {
			f.LastErrorTime = lastErrorTime.Time
		}
		if lastErrorCode.Valid {
			f.LastErrorCode = int(lastErrorCode.Int64)
		}
		if lastErrorSnippet.Valid {
			f.LastErrorSnippet = lastErrorSnippet.String
		}
		if backoffUntil.Valid {
			f.BackoffUntil = backoffUntil.Time
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
}

func (s *Store) GetFeedsWithErrors(ctx context.Context) ([]models.Feed, error) {
	query := "SELECT id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet, full_article, etag, last_modified FROM feeds WHERE last_error_time IS NOT NULL OR last_error_code != 0"
	s.logger.Debug("executing query", "query", query)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	feeds := make([]models.Feed, 0, 16)
	for rows.Next() {
		var f models.Feed
		var lastPoll, lastErrorTime sql.NullTime
		var lastErrorCode sql.NullInt64
		var lastErrorSnippet, etag, lastModified sql.NullString
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &lastErrorTime, &lastErrorCode, &lastErrorSnippet, &f.FullArticle, &etag, &lastModified); err != nil {
			return nil, err
		}
		if lastPoll.Valid {
			f.LastPoll = lastPoll.Time
		}
		if lastErrorTime.Valid {
			f.LastErrorTime = lastErrorTime.Time
		}
		if lastErrorCode.Valid {
			f.LastErrorCode = int(lastErrorCode.Int64)
		}
		if lastErrorSnippet.Valid {
			f.LastErrorSnippet = lastErrorSnippet.String
		}
		if etag.Valid {
			f.ETag = etag.String
		}
		if lastModified.Valid {
			f.LastModified = lastModified.String
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
}

func (s *Store) GetFeed(ctx context.Context, id int64) (*models.Feed, error) {
	query := "SELECT id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet, full_article, backoff_until FROM feeds WHERE id = ?"
	s.logger.Debug("executing query", "query", query, "args", []any{id})
	var f models.Feed
	var lastPoll, lastErrorTime, backoffUntil sql.NullTime
	var lastErrorCode sql.NullInt64
	var lastErrorSnippet sql.NullString
	err := s.db.QueryRowContext(ctx, query, id).
		Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &lastErrorTime, &lastErrorCode, &lastErrorSnippet, &f.FullArticle, &backoffUntil)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastPoll.Valid {
		f.LastPoll = lastPoll.Time
	}
	if lastErrorTime.Valid {
		f.LastErrorTime = lastErrorTime.Time
	}
	if lastErrorCode.Valid {
		f.LastErrorCode = int(lastErrorCode.Int64)
	}
	if lastErrorSnippet.Valid {
		f.LastErrorSnippet = lastErrorSnippet.String
	}
	if backoffUntil.Valid {
		f.BackoffUntil = backoffUntil.Time
	}
	return &f, nil
}

func (s *Store) GetFeedByURL(ctx context.Context, url string) (*models.Feed, error) {
	query := "SELECT id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet, full_article, etag, last_modified FROM feeds WHERE url = ?"
	s.logger.Debug("executing query", "query", query, "args", []any{url})
	var f models.Feed
	var lastPoll, lastErrorTime sql.NullTime
	var lastErrorCode sql.NullInt64
	var lastErrorSnippet, etag, lastModified sql.NullString
	err := s.db.QueryRowContext(ctx, query, url).
		Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &lastErrorTime, &lastErrorCode, &lastErrorSnippet, &f.FullArticle, &etag, &lastModified)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastPoll.Valid {
		f.LastPoll = lastPoll.Time
	}
	if lastErrorTime.Valid {
		f.LastErrorTime = lastErrorTime.Time
	}
	if lastErrorCode.Valid {
		f.LastErrorCode = int(lastErrorCode.Int64)
	}
	if lastErrorSnippet.Valid {
		f.LastErrorSnippet = lastErrorSnippet.String
	}
	if etag.Valid {
		f.ETag = etag.String
	}
	if lastModified.Valid {
		f.LastModified = lastModified.String
	}
	return &f, nil
}

func (s *Store) AddFeed(ctx context.Context, url string, title string, fullArticle bool) (int64, error) {
	query := "INSERT INTO feeds (url, title, full_article) VALUES (?, ?, ?) ON CONFLICT(url) DO UPDATE SET title=excluded.title, full_article=excluded.full_article"
	s.logger.Debug("executing exec", "query", query, "args", []any{url, title, fullArticle})
	res, err := s.db.ExecContext(ctx, query, url, title, fullArticle)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateFeed(ctx context.Context, id int64, url *string, title *string, fullArticle *bool) error {
	if url == nil && title == nil && fullArticle == nil {
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
	id, _ := res.LastInsertId()
	return id, nil
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
	return users, nil
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
