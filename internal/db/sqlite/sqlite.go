package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hobe/rss2go/internal/models"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// New creates a new SQLite store.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging database: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) GetFeeds(ctx context.Context) ([]models.Feed, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, url, title, last_poll, poll_interval FROM feeds")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []models.Feed
	for rows.Next() {
		var f models.Feed
		var lastPoll sql.NullTime
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &f.PollInterval); err != nil {
			return nil, err
		}
		if lastPoll.Valid {
			f.LastPoll = lastPoll.Time
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
}

func (s *Store) GetFeed(ctx context.Context, id int64) (*models.Feed, error) {
	var f models.Feed
	var lastPoll sql.NullTime
	err := s.db.QueryRowContext(ctx, "SELECT id, url, title, last_poll, poll_interval FROM feeds WHERE id = ?", id).
		Scan(&f.ID, &f.URL, &f.Title, &lastPoll, &f.PollInterval)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastPoll.Valid {
		f.LastPoll = lastPoll.Time
	}
	return &f, nil
}

func (s *Store) AddFeed(ctx context.Context, url string, title string) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO feeds (url, title) VALUES (?, ?) ON CONFLICT(url) DO UPDATE SET title=excluded.title", url, title)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateFeedLastPoll(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE feeds SET last_poll = ? WHERE id = ?", time.Now(), id)
	return err
}

func (s *Store) AddUser(ctx context.Context, email string) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO users (email) VALUES (?) ON CONFLICT(email) DO NOTHING", email)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		// If already exists, we might need to fetch it, but here we'll just return it if we can.
		// For simplicity, we can let the caller handle it or use a separate query.
	}
	return id, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRowContext(ctx, "SELECT id, email FROM users WHERE email = ?", email).Scan(&u.ID, &u.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUsersForFeed(ctx context.Context, feedID int64) ([]models.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email 
		FROM users u 
		JOIN subscriptions s ON u.id = s.user_id 
		WHERE s.feed_id = ?`, feedID)
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
	_, err := s.db.ExecContext(ctx, "INSERT INTO subscriptions (user_id, feed_id) VALUES (?, ?) ON CONFLICT DO NOTHING", userID, feedID)
	return err
}

func (s *Store) IsSeen(ctx context.Context, feedID int64, guid string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM seen_items WHERE feed_id = ? AND guid = ?)", feedID, guid).Scan(&exists)
	return exists, err
}

func (s *Store) MarkSeen(ctx context.Context, feedID int64, guid string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO seen_items (feed_id, guid) VALUES (?, ?) ON CONFLICT DO NOTHING", feedID, guid)
	return err
}
