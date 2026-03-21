# rss2go: Technical Specification & Implementation Guide

This document provides a comprehensive specification for the `rss2go` daemon, a modern RSS-to-Email aggregator. It serves as a blueprint for understanding the system's architecture, functional requirements, and the handling of various edge cases.

---

## 1. Overview
`rss2go` is a Go-based daemon that periodically polls RSS/Atom feeds and emails new items to subscribed users. It is designed for high reliability, security, and low resource usage.

---

## 2. Core Architecture

The system follows a decoupled, pool-based architecture using Go channels for communication:

- **Registry**: Orchestrates multiple `Watcher` goroutines.
- **Feed Watchers**: One goroutine per feed. Manages the polling interval, backoff logic, and parses feed items.
- **Crawler Pool**: A shared pool of workers that execute HTTP requests with context-aware timeouts.
- **Mailer Pool**: A shared pool of workers that manages persistent SMTP connections and sends emails.
- **Database (SQLite)**: Stores feed metadata, users, subscriptions, and tracked GUIDs to prevent duplicate emails.

### Data Flow
1. `Watcher` triggers a `CrawlRequest` -> `Crawler Pool`.
2. `Crawler` fetches XML -> returns `CrawlResponse` to `Registry`.
3. `Registry` routes response back to the specific `Watcher`.
4. `Watcher` parses items -> checks DB for seen GUIDs -> submits `MailRequest` for new items -> `Mailer Pool`.
5. `Mailer` sends email via persistent SMTP connection.

---

## 3. Functional Requirements

### 3.1 CLI Commands
- `daemon`: Starts the aggregator. Supports `--log-level` and `--config` flags.
- `add-feed [url] [title]`: Adds a new feed to the system.
- `add-user [email]`: Adds a new subscriber.
- `subscribe [email] [feed-id]`: Links a user to a feed.
- `list-feeds`: Displays all configured feeds and their IDs.
- `test-feed [url] [email]`: Synchronously fetches a feed and sends the *first* item to the email. Uses the same sanitization logic as the daemon.
- `list-errors`: Displays feeds that are currently failing, including HTTP codes and response snippets.
- `catchup [feed-id] [--all]`: Marks all current items in a feed as "seen" without sending emails.

### 3.2 Configuration
- Uses `viper` to load from `rss2go.yaml`, environment variables (`RSS2GO_`), or CLI flags.
- **Mandatory**: The program must exit with a non-zero code if the config file is missing.
- **Settings**: DB path, SMTP credentials, polling intervals, jitter, and pool sizes.

---

## 4. Technical Implementation Details

### 4.1 Persistence (Database)
- **Engine**: Pure-Go SQLite (`modernc.org/sqlite`).
- **Concurrency**: Must use **Write-Ahead Logging (WAL)** mode and a `busy_timeout` (e.g., 5000ms) to prevent "database is locked" errors during high-concurrency writes.
- **Migrations**: Handled via `pressly/goose`, embedded in the binary.
- **Error Tracking**: The `feeds` table must store `last_error_time`, `last_error_code`, and `last_error_snippet` (cleared on successful crawl).

### 4.2 Security & Sanitization
Every feed item undergoes a two-stage sanitization process:
1. **Structural Pass (goquery)**:
    - Replace `<iframe>` tags with `<a>` links to the `src`.
    - Strip anchor tags containing `da.feedsportal.com`.
    - Strip tracking pixels (images with `width` or `height` of 0/1, or URLs containing "tracker", "pixel", "analytics").
2. **Security Pass (bluemonday)**:
    - **Strict Policy**: Applied to titles and links (plain text only).
    - **UGC Policy**: Applied to bodies. Must strip all `style` attributes and ensure `rel="noopener noreferrer"` on links.

### 4.3 Polling & Backoff
- **Initialization**: On startup, calculate `next_poll` based on `last_poll` from DB + `interval` + `jitter`.
- **Jitter**: Apply random jitter to every poll to avoid thundering herds.
- **Exponential Backoff**: If a crawl fails, double the interval up to a maximum (e.g., 24 hours). Reset to base interval on success.

### 4.4 Crawler Logic
- **Context-Aware**: All HTTP requests must use `http.NewRequestWithContext`.
- **Timeouts**: Use `context.WithTimeout` for every request.
- **Logging**: Log status codes, content-type, and response sizes at the `DEBUG` level.

### 4.5 Mailer Logic
- **Persistence**: Maintain a single persistent SMTP connection.
- **Idle Timeout**: Automatically close the SMTP connection after 3 minutes of inactivity.
- **Retries**: Implement exponential backoff for transient SMTP errors (e.g., 5 retries).
- **Multipart Support**: (Recommended) Send both `text/plain` and `text/html` versions.

---

## 5. Observability
- **Logging**: Structured logging using `log/slog`.
- **Metrics**: HTTP endpoint (`/metrics`) exposing:
    - `feeds_crawled_total` (counter)
    - `feeds_crawled_errors` (counter)
    - `emails_sent_total` (counter)

---

## 6. Development Standards
- **Go Version**: 1.22+ (utilizing modern `for range` over integers).
- **Testing**: High coverage (>90%) for `db`, `crawler`, and `watcher`. Use `testify/mock` for interface-based testing.
- **Concurrency**: Pass `context.Context` through all layers for graceful shutdown.
