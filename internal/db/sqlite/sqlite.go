package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging database: %w", err)
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
	query := "SELECT id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet FROM feeds"
	s.logger.Debug("executing query", "query", query)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []models.Feed
	for rows.Next() {
		var f models.Feed
		var lastPoll, lastErrorTime sql.NullTime
		var lastErrorCode sql.NullInt64
		var lastErrorSnippet sql.NullString
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &lastErrorTime, &lastErrorCode, &lastErrorSnippet); err != nil {
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
		feeds = append(feeds, f)
	}
	return feeds, nil
}

func (s *Store) GetFeedsWithErrors(ctx context.Context) ([]models.Feed, error) {
	query := "SELECT id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet FROM feeds WHERE last_error_time IS NOT NULL OR last_error_code != 0"
	s.logger.Debug("executing query", "query", query)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []models.Feed
	for rows.Next() {
		var f models.Feed
		var lastPoll, lastErrorTime sql.NullTime
		var lastErrorCode sql.NullInt64
		var lastErrorSnippet sql.NullString
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &lastErrorTime, &lastErrorCode, &lastErrorSnippet); err != nil {
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
		feeds = append(feeds, f)
	}
	return feeds, nil
}

func (s *Store) GetFeed(ctx context.Context, id int64) (*models.Feed, error) {
	query := "SELECT id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet FROM feeds WHERE id = ?"
	s.logger.Debug("executing query", "query", query, "args", []any{id})
	var f models.Feed
	var lastPoll, lastErrorTime sql.NullTime
	var lastErrorCode sql.NullInt64
	var lastErrorSnippet sql.NullString
	err := s.db.QueryRowContext(ctx, query, id).
		Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &lastErrorTime, &lastErrorCode, &lastErrorSnippet)
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
	return &f, nil
}

func (s *Store) GetFeedByURL(ctx context.Context, url string) (*models.Feed, error) {
	query := "SELECT id, url, title, last_poll, last_error_time, last_error_code, last_error_snippet FROM feeds WHERE url = ?"
	s.logger.Debug("executing query", "query", query, "args", []any{url})
	var f models.Feed
	var lastPoll, lastErrorTime sql.NullTime
	var lastErrorCode sql.NullInt64
	var lastErrorSnippet sql.NullString
	err := s.db.QueryRowContext(ctx, query, url).
		Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &lastErrorTime, &lastErrorCode, &lastErrorSnippet)
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
	return &f, nil
}

func (s *Store) AddFeed(ctx context.Context, url string, title string) (int64, error) {
	query := "INSERT INTO feeds (url, title) VALUES (?, ?) ON CONFLICT(url) DO UPDATE SET title=excluded.title"
	s.logger.Debug("executing exec", "query", query, "args", []any{url, title})
	res, err := s.db.ExecContext(ctx, query, url, title)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
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

func (s *Store) UpdateFeedLastPoll(ctx context.Context, id int64) error {
	query := "UPDATE feeds SET last_poll = ? WHERE id = ?"
	now := time.Now()
	s.logger.Debug("executing exec", "query", query, "args", []any{now, id})
	_, err := s.db.ExecContext(ctx, query, now, id)
	return err
}

func (s *Store) UpdateFeedError(ctx context.Context, id int64, code int, snippet string) error {
	var query string
	var args []any
	if code == 0 && snippet == "" {
		query = "UPDATE feeds SET last_error_time = NULL, last_error_code = NULL, last_error_snippet = NULL WHERE id = ?"
		args = []any{id}
	} else {
		query = "UPDATE feeds SET last_error_time = ?, last_error_code = ?, last_error_snippet = ? WHERE id = ?"
		args = []any{time.Now(), code, snippet, id}
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

	var users []models.User
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
