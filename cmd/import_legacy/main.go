package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"time"

	"rss2go/internal/database"

	_ "modernc.org/sqlite"
)

func main() {
	srcPath := flag.String("src", "../rss2go/rss2go.db", "Path to source legacy SQLite database")
	dstPath := flag.String("dst", "rss2go.db", "Path to destination modern SQLite database")
	flag.Parse()

	if _, err := os.Stat(*srcPath); os.IsNotExist(err) {
		log.Fatalf("source database file does not exist: %s", *srcPath)
	}

	srcDB, err := sql.Open("sqlite", *srcPath)
	if err != nil {
		log.Fatalf("failed to open source database: %v", err)
	}
	defer func() { _ = srcDB.Close() }()

	dstDB, err := database.Open(*dstPath)
	if err != nil {
		log.Fatalf("failed to open destination database: %v", err)
	}
	defer func() { _ = dstDB.Close() }()

	ctx := context.Background()

	// 1. Migrate Users
	log.Println("Migrating users...")
	usersRows, err := srcDB.QueryContext(ctx, "SELECT id, email FROM users")
	if err != nil {
		log.Fatalf("failed to query source users: %v", err)
	}
	defer func() { _ = usersRows.Close() }()

	userCount := 0
	for usersRows.Next() {
		var id int64
		var email string
		if err := usersRows.Scan(&id, &email); err != nil {
			log.Fatalf("failed to scan user: %v", err)
		}

		_, err = dstDB.ExecContext(ctx,
			"INSERT INTO users (id, email) VALUES (?, ?) ON CONFLICT(email) DO NOTHING",
			id, email,
		)
		if err != nil {
			log.Fatalf("failed to insert user %q: %v", email, err)
		}
		userCount++
	}
	log.Printf("Successfully processed %d users.", userCount)

	// 2. Migrate Feeds
	log.Println("Migrating feeds...")
	feedsRows, err := srcDB.QueryContext(ctx, `
		SELECT 
			id, url, title, last_poll, last_error_time, last_error_snippet, 
			full_article, etag, last_modified, extraction_strategy, extraction_config 
		FROM feeds
	`)
	if err != nil {
		log.Fatalf("failed to query source feeds: %v", err)
	}
	defer func() { _ = feedsRows.Close() }()

	feedCount := 0
	for feedsRows.Next() {
		var id int64
		var urlStr, title string
		var lastPoll, lastErrorTime sql.NullTime
		var lastErrorSnippet sql.NullString
		var fullArticle bool
		var etag, lastModified string
		var strategy, config sql.NullString

		err := feedsRows.Scan(
			&id, &urlStr, &title, &lastPoll, &lastErrorTime, &lastErrorSnippet,
			&fullArticle, &etag, &lastModified, &strategy, &config,
		)
		if err != nil {
			log.Fatalf("failed to scan feed: %v", err)
		}

		// Map extraction strategy
		newStrategy := "heuristic"
		cssSelector := ""
		if strategy.Valid {
			if strategy.String == "selector" {
				newStrategy = "css"
				if config.Valid {
					cssSelector = config.String
				}
			} else {
				newStrategy = "heuristic"
			}
		}

		extractFull := 0
		if fullArticle {
			extractFull = 1
		}

		var lastPolled *time.Time
		if lastPoll.Valid {
			lastPolled = &lastPoll.Time
		}

		var errTime *time.Time
		if lastErrorTime.Valid {
			errTime = &lastErrorTime.Time
		}

		errStr := ""
		if lastErrorSnippet.Valid {
			errStr = lastErrorSnippet.String
		}

		nextPollAt := time.Now()

		_, err = dstDB.ExecContext(ctx, `
			INSERT INTO feeds (
				id, title, url, etag, last_modified, next_poll_at, 
				poll_interval_secs, backoff_factor, last_error_str, 
				last_error_time, last_error_snippet, last_polled_at, 
				extract_full_article, extraction_strategy, css_selector
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(url) DO NOTHING`,
			id, title, urlStr, etag, lastModified, nextPollAt,
			3600, 1.0, errStr,
			errTime, errStr, lastPolled,
			extractFull, newStrategy, cssSelector,
		)
		if err != nil {
			log.Fatalf("failed to insert feed %q: %v", urlStr, err)
		}
		feedCount++
	}
	log.Printf("Successfully processed %d feeds.", feedCount)

	// 3. Migrate Subscriptions (excluding orphaned rows pointing to deleted users/feeds)
	log.Println("Migrating subscriptions...")
	subRows, err := srcDB.QueryContext(ctx, `
		SELECT s.user_id, s.feed_id 
		FROM subscriptions s
		JOIN feeds f ON s.feed_id = f.id
		JOIN users u ON s.user_id = u.id
	`)
	if err != nil {
		log.Fatalf("failed to query source subscriptions: %v", err)
	}
	defer func() { _ = subRows.Close() }()

	subCount := 0
	for subRows.Next() {
		var userID, feedID int64
		if err := subRows.Scan(&userID, &feedID); err != nil {
			log.Fatalf("failed to scan subscription: %v", err)
		}

		_, err = dstDB.ExecContext(ctx,
			"INSERT INTO subscriptions (user_id, feed_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
			userID, feedID,
		)
		if err != nil {
			log.Fatalf("failed to insert subscription (%d, %d): %v", userID, feedID, err)
		}
		subCount++
	}
	log.Printf("Successfully processed %d subscriptions.", subCount)

	// 4. Migrate Seen Items (excluding orphaned rows pointing to deleted feeds)
	log.Println("Migrating seen items...")
	seenRows, err := srcDB.QueryContext(ctx, `
		SELECT s.feed_id, s.guid, s.seen_at 
		FROM seen_items s
		JOIN feeds f ON s.feed_id = f.id
	`)
	if err != nil {
		log.Fatalf("failed to query source seen_items: %v", err)
	}
	defer func() { _ = seenRows.Close() }()

	seenCount := 0
	for seenRows.Next() {
		var feedID int64
		var guid string
		var seenAt time.Time
		if err := seenRows.Scan(&feedID, &guid, &seenAt); err != nil {
			log.Fatalf("failed to scan seen_item: %v", err)
		}

		_, err = dstDB.ExecContext(ctx,
			"INSERT INTO seen_items (feed_id, guid, seen_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING",
			feedID, guid, seenAt,
		)
		if err != nil {
			log.Fatalf("failed to insert seen_item (%d, %s): %v", feedID, guid, err)
		}
		seenCount++
	}
	log.Printf("Successfully processed %d seen_items.", seenCount)

	log.Println("Migration complete!")
}
