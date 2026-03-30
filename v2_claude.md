# rss2go v2: Proposed Improvements

Generated from codebase analysis against RSS2GO_SPEC.md.

---

## High Impact

### 1. Replace per-goroutine Watcher model with a scheduler

Currently, each feed spawns its own goroutine (one Watcher per feed). At 1000+ feeds this still works, but a priority-queue scheduler (min-heap ordered by `next_poll`) would handle 10K+ feeds with a fixed goroutine pool instead. The registry+watcher model in `watcher/registry.go` and `watcher/watcher.go` would be refactored into a scheduler that drains a heap.

### 2. DB-backed outbox for emails

Currently `MailRequest` objects live only in a buffered channel — a crash loses undelivered emails. A v2 `outbox` table (`id, feed_id, user_id, subject, body, status, created_at`) would let the mailer pool drain persisted jobs and mark them delivered, achieving at-least-once semantics with no added code complexity.

### 3. Migrate `cmd/rss2go/main.go` to proper package structure

At 746 lines, `main.go` does too much. The daemon orchestration, resync loop, all CRUD commands, and `test-feed` logic should be split into `cmd/rss2go/daemon.go`, `cmd/rss2go/feed.go`, `cmd/rss2go/user.go`, etc. This is a maintainability blocker before any large feature work — the current CRUD commands and daemon orchestration share globals that make unit testing nearly impossible.

### 4. HTTP management API

Add a thin HTTP API (alongside the existing `/metrics` endpoint) for feed/user/subscription CRUD. This unlocks headless management, webhooks, and a potential future web UI without duplicating CLI logic — the CLI commands can call the same service layer.

---

## Medium Impact

### 5. Per-user email digest mode

Add a `subscriptions.digest` column (`boolean`) and a `subscriptions.digest_interval` column. Instead of mailing every item as it arrives, batch them into a single daily/hourly digest per user per feed. This is the most-requested feature in similar tools.

### 6. Retry-After / rate limit awareness in the crawler

`crawler.go` currently ignores `429 Too Many Requests` and `Retry-After` headers. Respecting these in `handleFeedResponse` (storing the retry time on the feed row) would prevent feed bans and is increasingly necessary as feeds add rate limits.

### 7. Content-hash deduplication fallback

`seen_items` uses GUID as the key. Some feeds rotate GUIDs on edits, causing re-delivery. A `content_hash` column (SHA-256 of title+link) as a secondary dedup key in `IsSeen`/`MarkSeen` would eliminate this class of duplicates.

### 8. Persist backoff state across restarts

The current exponential backoff lives only in watcher memory (`watcher.go` local state). A restart resets it, causing an immediate retry storm on startup. Adding a `backoff_until` column to the `feeds` table would survive process restarts and preserve backoff state.

---

## Lower Impact / Quality of Life

### 9. Configurable DB connection pool

`sqlite.go` hard-codes `db.SetMaxOpenConns(25)`. This should be a config key (`db_max_connections`) — useful for tuning on low-RAM VPS deployments.

### 10. Feed validation on `add-feed`

`add-feed` inserts the URL without checking it's a valid feed. A synchronous fetch+parse at add time (reusing the crawler logic) would provide immediate feedback and prevent silent misconfiguration.

### 11. Pending item TTL cleanup in watcher

`watcher.go` maintains a `pendingItems` map for full-article extractions. There's no timeout or eviction — a hung HTTP request leaks an entry forever. A TTL-based cleanup (e.g. prune entries older than `2 * crawlerTimeout`) would close this leak.

### 12. Replace `gorilla/feeds` in scraper with stdlib XML

The scraper only uses `gorilla/feeds` to render RSS XML. That entire dependency could be replaced with a 30-line struct + `encoding/xml`, removing a transitive dependency chain.

### 13. Structured `list-errors` output

`list-errors` currently prints to stdout with `fmt.Printf`. Adding `--json` and `--format` flags (table/json) would make it scriptable for monitoring integrations.
