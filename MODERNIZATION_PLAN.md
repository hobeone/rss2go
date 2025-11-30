# Modernization Plan

This document outlines suggested steps to further modernize the rss2go codebase.

## Completed
- [x] Remove obsolete CI (`wercker.yml`) and init scripts (`upstart`).
- [x] Introduce `context.Context` to `crawler` package.
- [x] Fix resource leaks (context cancellation) and potential infinite loops in `FeedCrawler`.
- [x] Logging Migration: Replaced `logrus` with `log/slog`.

## Recommended Next Steps

### 1. Database Migrations
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
