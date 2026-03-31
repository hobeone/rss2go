# rss2go v2: Proposed Improvements

Generated from codebase analysis against RSS2GO_SPEC.md.

---

## High Impact

### 1. DB-backed outbox for emails

Currently `MailRequest` objects live only in a buffered channel — a crash loses undelivered emails. A v2 `outbox` table (`id, feed_id, user_id, subject, body, status, created_at`) would let the mailer pool drain persisted jobs and mark them delivered, achieving at-least-once semantics with no added code complexity.

### 2. HTTP management API

Add a thin HTTP API (alongside the existing `/metrics` endpoint) for feed/user/subscription CRUD. This unlocks headless management, webhooks, and a potential future web UI without duplicating CLI logic — the CLI commands can call the same service layer.

---

## Medium Impact

### 3. Per-user email digest mode

Add a `subscriptions.digest` column (`boolean`) and a `subscriptions.digest_interval` column. Instead of mailing every item as it arrives, batch them into a single daily/hourly digest per user per feed. This is the most-requested feature in similar tools.

### ~~4. Retry-After / rate limit awareness in the crawler~~ ✓ Done

~~`crawler.go` currently ignores `429 Too Many Requests` and `Retry-After` headers. Respecting these in `handleFeedResponse` (storing the retry time on the feed row) would prevent feed bans and is increasingly necessary as feeds add rate limits.~~

### 5. Content-hash deduplication fallback

`seen_items` uses GUID as the key. Some feeds rotate GUIDs on edits, causing re-delivery. A `content_hash` column (SHA-256 of title+link) as a secondary dedup key in `IsSeen`/`MarkSeen` would eliminate this class of duplicates.

### ~~6. Persist backoff state across restarts~~ ✓ Done

~~The current exponential backoff lives only in watcher memory (`watcher.go` local state). A restart resets it, causing an immediate retry storm on startup. Adding a `backoff_until` column to the `feeds` table would survive process restarts and preserve backoff state.~~

---

## Lower Impact / Quality of Life

### 7. Configurable DB connection pool

`sqlite.go` hard-codes `db.SetMaxOpenConns(25)`. This should be a config key (`db_max_connections`) — useful for tuning on low-RAM VPS deployments.

### 8. Feed validation on `add-feed`

`add-feed` inserts the URL without checking it's a valid feed. A synchronous fetch+parse at add time (reusing the crawler logic) would provide immediate feedback and prevent silent misconfiguration.

### 9. Pending item TTL cleanup in watcher

`watcher.go` maintains a `pendingItems` map for full-article extractions. There's no timeout or eviction — a hung HTTP request leaks an entry forever. A TTL-based cleanup (e.g. prune entries older than `2 * crawlerTimeout`) would close this leak.

### 10. Replace `gorilla/feeds` in scraper with stdlib XML

The scraper only uses `gorilla/feeds` to render RSS XML. That entire dependency could be replaced with a 30-line struct + `encoding/xml`, removing a transitive dependency chain.

### 11. Structured `list-errors` output

`list-errors` currently prints to stdout with `fmt.Printf`. Adding `--json` and `--format` flags (table/json) would make it scriptable for monitoring integrations.
