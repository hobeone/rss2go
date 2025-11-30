# Modernization Plan

This document outlines suggested steps to further modernize the rss2go codebase.

## Completed
- [x] Remove obsolete CI (`wercker.yml`) and init scripts (`upstart`).
- [x] Introduce `context.Context` to `crawler` package.
- [x] Fix resource leaks (context cancellation) and potential infinite loops in `FeedCrawler`.

## Recommended Next Steps

### 1. Logging Migration
**Goal:** Replace `github.com/sirupsen/logrus` with the standard library `log/slog` (Go 1.21+).
**Benefit:** Removes external dependency, better performance, standard API.
**Strategy:**
- Update `log/log.go` to setup a `*slog.Logger`.
- Replace `logrus.FieldLogger` with `*slog.Logger` in structs (`FeedWatcher`, etc.).
- Rewrite log calls: `logrus.Infof("msg %s", arg)` -> `slog.Info("msg", "key", arg)`.

### 2. Database Migrations
**Goal:** Replace `github.com/hobeone/gomigrate` with a widely supported tool like `golang-migrate/migrate` or `pressly/goose`.
**Benefit:** Better support, CLI tools, standard format.

### 3. CLI Library
**Goal:** Consider migrating from `kingpin` (maintenance mode) to `cobra`.
**Benefit:** Industry standard, better completion support, active development.

### 4. Database Driver
**Goal:** Update `github.com/mattn/go-sqlite3` and consider `modernc.org/sqlite` (pure Go) if CGO is a concern.
**Current Status:** `go.mod` has a recent version, so this is lower priority.

### 5. CI/CD
**Goal:** Expand `.github/workflows/go.yml` to include linting (`golangci-lint`) and test coverage.

### 6. Error Handling
**Goal:** Ensure errors are wrapped using `%w` for better traceability.
