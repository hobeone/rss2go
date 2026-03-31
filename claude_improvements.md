# rss2go — Code Improvement Notes

Findings from a post-v2 codebase review. Ordered roughly by impact.

---

## Concurrency / Correctness

### 1. Scheduler response goroutines are untracked on shutdown

`scheduler.go:routeResponses` spawns an untracked goroutine per crawl response:

```go
go func(w *Watcher, resp crawler.CrawlResponse) {
    interval, isFeed := w.HandleResponse(ctx, resp)
    if isFeed {
        s.reschedFeed(resp.FeedID, interval)
    }
}(w, resp)
```

When `ctx` is cancelled, `Run()` returns immediately but these goroutines may still
be executing `HandleResponse` (DB writes, mailer submits). There is no WaitGroup to
drain them before the caller's `defer store.Close()` fires.

**Fix**: Add a `sync.WaitGroup` to the Scheduler. `Add(1)` before launching each goroutine,
`Done()` at the end. Add a `Wait()` call after the main loop exits in `Run()`.

---

### 2. `sendmail` goroutine can block forever

`mailer.go:sendSendmail` spawns a goroutine to write to stdin, then calls `cmd.Run()`.
If `cmd.Run()` returns an error before the process reads stdin, the write goroutine
blocks waiting for the pipe to drain — and `<-errChan` is never reached because the
function has already returned the Run error.

```go
go func() { ...; errChan <- werr }()
if err := cmd.Run(); err != nil {
    return fmt.Errorf("sendmail command failed: %w", err)  // goroutine leaks
}
if err := <-errChan; err != nil { ... }
```

**Fix**: Always drain `errChan` before returning, or close stdin before `cmd.Run()`.
The simplest fix: `defer func() { <-errChan }()` at the top, so both exit paths
consume the channel.

---

### 3. `drainOutbox` ignores shutdown context

All DB calls in `drainOutbox` use `context.Background()`. When the daemon receives
SIGINT, the shutdown context is cancelled, but a draining worker can still block on
a slow DB write, delaying `pool.Close()`.

**Fix**: Thread the context through — either accept `ctx` in `drainOutbox`, or store
the pool-level context set in `NewPool` (cancelled on `Close`).

---

## Idiomatic Go

### 4. Index-range loop instead of value-range

Two identical blocks in `watcher.go` use the index-range form when the value is what's needed:

```go
// lines ~203, ~281
for u := range users {
    userEmails = append(userEmails, users[u].Email)
}
```

Idiomatic Go:

```go
for _, u := range users {
    userEmails = append(userEmails, u.Email)
}
```

---

### 5. Duplicate user-email extraction

The pattern above appears twice (in `handleFeedResponse` and `handleItemResponse`).
Extract once:

```go
func (w *Watcher) getUserEmails(ctx context.Context) ([]string, error) {
    users, err := w.store.GetUsersForFeed(ctx, w.feed.ID)
    if err != nil {
        return nil, err
    }
    emails := make([]string, len(users))
    for i, u := range users {
        emails[i] = u.Email
    }
    return emails, nil
}
```

Pre-allocating with `make([]string, len(users))` also avoids the repeated `append`
growth.

---

### 6. CLI commands repeat the same three-line setup

Seven command functions (`runAddFeed`, `runDelFeed`, `runUpdateFeed`, `runListFeeds`,
`runListErrors`, `runCatchup`, `runAddUser`, `runSubscribe`, `runUnsubscribe`) all
open with:

```go
cfg, err := config.Load(cfgFile)
if err != nil { return err }
logger := getLogger(cfg)
store, err := getStore(logger)
if err != nil { return err }
defer store.Close()
```

A single helper eliminates ~50 lines of boilerplate:

```go
func setup() (*config.Config, *slog.Logger, *sqlite.Store, error) {
    cfg, err := config.Load(cfgFile)
    if err != nil { return nil, nil, nil, err }
    logger := getLogger(cfg)
    store, err := getStore(logger)
    if err != nil { return nil, nil, nil, err }
    return cfg, logger, store, nil
}
```

---

## Missing Pre-allocation

### 7. Slice appends without capacity hint in sqlite.go

`GetFeeds`, `GetFeedsWithErrors`, and `GetUsersForFeed` all append to a nil slice
inside `rows.Next()` loops. Each append past the current capacity triggers a copy.

Since the number of rows isn't known before scanning, the pragmatic fix is a
reasonable starting capacity:

```go
feeds := make([]models.Feed, 0, 32)   // GetFeeds
users := make([]models.User, 0, 8)    // GetUsersForFeed
```

Alternatively, `rows.Rows` has no `len`, but a `SELECT COUNT(*)` first (or
`LIMIT`-aware pagination) would allow exact pre-allocation.

---

## API Clarity

### 8. `UpdateFeedError(id, 0, "")` is a hidden clear operation

The Store interface uses the same method to both set and clear the error state:

```go
store.UpdateFeedError(ctx, id, 500, "Internal Server Error")  // set
store.UpdateFeedError(ctx, id, 0, "")                          // clear ← not obvious
```

The clear semantic is only discovered by reading the SQL. Two methods would be self-documenting:

```go
SetFeedError(ctx context.Context, id int64, code int, snippet string) error
ClearFeedError(ctx context.Context, id int64) error
```

---

### 9. Mock `ClaimPendingEmail` panics on nil return

The testify mock in `watcher_test.go`:

```go
func (m *mockStore) ClaimPendingEmail(ctx context.Context) (*models.OutboxEntry, error) {
    args := m.Called(ctx)
    return args.Get(0).(*models.OutboxEntry), args.Error(1)
}
```

If a test sets up `.Return(nil, nil)`, `args.Get(0)` is a `nil` interface value and
the type assertion `.(* models.OutboxEntry)` panics.

**Fix**:

```go
entry, _ := args.Get(0).(*models.OutboxEntry)
return entry, args.Error(1)
```

The comma-ok form returns a nil pointer (not a panic) when the underlying value is nil.
The same pattern applies to other mock methods that return pointer types
(`GetFeed`, `GetFeedByURL`, `GetUserByEmail`).

---

## Minor

### 10. `nolint:errcheck` on `closeSMTP` can be replaced

```go
//nolint:errcheck
func (p *Pool) closeSMTP() {
    if p.smtpConn != nil {
        p.smtpConn.Close() // #nosec G104
```

The `Close()` error on a `gomail.SendCloser` is almost always a no-op (flushing
already-sent messages). Log it at debug level instead of suppressing it entirely:

```go
if err := p.smtpConn.Close(); err != nil {
    p.logger.Debug("SMTP close error", "error", err)
}
```

This removes the linter suppression and gives visibility without noisy production logs.
