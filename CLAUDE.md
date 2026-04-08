# rss2go — Claude Code Instructions

## Required Steps (Every Interaction)

These apply to any code change, no exceptions:

1. **Run tests**: `go test ./...` — all tests must pass
2. **Run go vet**: `go vet ./...` — must report no issues; catches bugs the compiler misses (bad printf verbs, mutex copies, unreachable code, etc.)
3. **Run go fix**: `go fix ./...` — rewrites deprecated API usages; keep the codebase current
4. **Verify build**: `go build ./cmd/rss2go/... ./cmd/scraper/...` — no missing imports or compile errors
5. **Database migrations**: schema changes MUST be a new file in `migrations/` using the next sequence number (e.g. `004_description.sql`). Never modify existing migration files.

## Project Layout

```
cmd/
  rss2go/         # Main daemon + CLI (package main, multi-file)
    main.go       # Root cmd, shared helpers (getStore, getLogger, getFeedID)
    daemon.go     # daemon subcommand + resyncFeeds loop
    feed.go       # feed subcommands (add/del/update/list/test/errors/catchup)
    user.go       # user subcommands (add/subscribe/unsubscribe)
  scraper/        # Standalone HTTP scraper companion service
internal/
  config/         # Viper-based config loading
  crawler/        # HTTP worker pool (CrawlRequest/CrawlResponse channels)
  db/
    db.go         # Store interface (19 methods)
    sqlite/       # SQLite implementation + migrations runner
  extractor/      # go-readability wrapper for full-article extraction
  mailer/         # SMTP/sendmail worker pool
  metrics/        # Prometheus-style counters + optional HTTP endpoint
  models/         # Plain data structs: Feed, User, Subscription, Item
  version/        # Build-time version info
  watcher/        # Core scheduling and feed-response handling
    scheduler.go  # Min-heap scheduler (one goroutine, N feeds)
    watcher.go    # Per-feed response handler and email formatter
migrations/       # Goose SQL migration files (embedded via embed.go)
```

## Architecture Overview

The daemon runs three concurrent components:

- **Crawler pool** — fixed goroutine pool that fetches URLs; results arrive on a shared `Responses()` channel
- **Scheduler** — single goroutine with a min-heap of `{feedID, nextPoll}`; wakes only when the next feed is due, dispatches crawl requests, then sleeps again. Feeds are removed from the heap before dispatch and re-inserted only after the response is fully processed (no duplicate in-flight crawls).
- **Mailer pool** — fixed goroutine pool that sends email via SMTP or sendmail with exponential-backoff retry

The `Scheduler` also runs a response router goroutine that reads from the crawler pool's `Responses()` channel and dispatches each response to the correct `Watcher`. For feed responses, the interval returned by `HandleResponse` is used to reschedule the feed.

A 1-minute resync loop in `daemon.go` polls the database for feed additions, removals, and metadata changes and calls `scheduler.Register`/`Unregister` accordingly.

## Go Conventions for This Codebase

### Interfaces for testability
Internal components communicate through interfaces defined in the consumer package:
- `db.Store` — defined in `internal/db/db.go`; the SQLite struct implements it
- `watcher.CrawlerPool`, `watcher.MailerPool` — defined in `internal/watcher/watcher.go`

When adding a new component dependency, define the minimal interface in the consuming package, not the providing one.

### Context everywhere
All functions that do I/O accept `context.Context` as the first argument. Never store a context in a struct; pass it through call chains.

### Error handling
- Return errors up the call stack; don't swallow them silently
- Use `fmt.Errorf("verb: %w", err)` for wrapping to preserve the error chain
- Log errors with `slog` at the point where they're handled, not where they're returned

### Logging
Use the structured `slog` logger from `log/slog`. Attach component-level context with `logger.With("component", "name")` in constructors (see crawler, scheduler). Never use `fmt.Println` in library code.

### Concurrency
- Channel-based worker pools (crawler, mailer) use buffered channels sized to `poolSize*2`
- Don't hold a mutex while calling `Submit` on a pool — it can block if the buffer is full
- The `Scheduler` uses a `wakeC chan struct{}` with a non-blocking send pattern to interrupt timers without dropping signals

### No CGO
The project uses `modernc.org/sqlite` (pure Go SQLite). Do not introduce dependencies that require CGO.

## Testing Patterns

### Unit tests — use mocks
`watcher_test.go` shows the pattern: implement the `db.Store`, `CrawlerPool`, and `MailerPool` interfaces with `testify/mock` structs. Use `mock.MatchedBy` for flexible argument matching.

```go
store.On("IsSeen", ctx, feedID, "guid").Return(false, nil)
store.AssertExpectations(t)
```

### Integration tests — use real SQLite
`sqlite_test.go` creates a real `test.db` file, runs migrations, and exercises the full store. Clean up with `os.Remove` in a deferred function.

### Test helpers
- `slog.New(slog.DiscardHandler)` for a no-op logger in tests
- Table-driven tests with `t.Run(tt.name, ...)` for multi-case scenarios (see `TestWatcher_FormatItem_ImageWidth`)

## Database Migrations

Goose migrations are embedded in the binary via `embed.go` and `migrations/`. Migration files are numbered sequentially:

```
001_initial.sql
002_full_article.sql
003_add_etag_last_modified.sql
004_add_backoff_until.sql
005_add_outbox.sql
006_extraction_strategy.sql
```

Next migration: `007_*.sql`. Always use `-- +goose Up` / `-- +goose Down` annotations.

## Configuration

Config is loaded via Viper (`internal/config/config.go`). The config file is searched in the current directory as `rss2go.yaml`. All config fields have sensible defaults. Key fields:

| Field | Purpose |
|-------|---------|
| `db_path` | SQLite file path |
| `poll_interval` / `poll_jitter` | Base polling interval + randomization |
| `crawler_pool_size` / `crawler_timeout` | HTTP concurrency |
| `mailer_pool_size` | Email concurrency |
| `max_image_width` | Max px before stripping image dimensions |
| `metrics_addr` | Optional `host:port` for metrics HTTP endpoint |

## Improvement Backlog

See `claude_improvements.md` for tracked improvement proposals. Completed refactors:
- `main.go` split into `daemon.go`, `feed.go`, `user.go`
- Per-goroutine watcher model replaced with `Scheduler` (min-heap)
