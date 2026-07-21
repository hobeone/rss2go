# rss2go

`rss2go` is a self-hosted syndication aggregator and notifier daemon. It periodically polls structured web feeds (such as RSS or Atom) and emails new items to subscribed recipients.

## Architecture

- `cmd/rss2go/`: Entry point and CLI command routing.
- `frontend/`: Svelte 5 + Vite + Vanilla CSS SPA dashboard.
- `internal/`: Core aggregator, scheduler, crawler, database, outbox, server, and sidecar packages.

## Building & Running

- **Build:** `go build ./...`
- **Run:** `./rss2go <flags>`
- **Test (unit):** `go test ./...`
- **Test (race):** `go test -race ./...`
- **Lint:** `go vet ./...` and `golangci-lint run ./...`

## Development Standards

Any AI agent or developer working on this codebase **must** follow these mandates.

### Tooling Setup

```bash
# Install goimports if not present
go install golang.org/x/tools/cmd/goimports@latest

# Install golangci-lint if not present (see https://golangci-lint.run/welcome/install/)
```

### Per-File Workflow (after every .go file edit)

```bash
goimports -w <file>   # format + resolve imports
go fix ./...          # adopt new language features automatically
go build ./...        # verify it compiles
```

### Quality Gate (before every commit)

```bash
goimports -w .
go fix ./...
go vet ./...
go test -race ./...
golangci-lint run ./...
```

All five must pass. Do not commit with failing tests, vet errors, or lint warnings.

### Coding Standards

- **Idioms:** "Accept interfaces, return structs." Define interfaces at the consumer side.
- **Context:** Every blocking or cancellable operation **must** accept `context.Context` as the first parameter.
- **Logging:** Use standard library `log/slog`. Every subsystem struct must accept a `*slog.Logger` in its constructor (defaulting to `slog.Default().With("component", "<name>")` if `nil`). Prefer structured key-value pairs (`url`, `id`, `err`, `duration`) over free-text formatting. Use `Debug` for operational tracing, `Info` for state transitions, `Warn` for fallbacks, and `Error` for unrecoverable step failures.
- **Errors:** Wrap with `fmt.Errorf("component: ...: %w", err)`. Never use `%v` for errors that will be inspected.
- **No hacks:** No `init()` for setup. No `panic` for control flow. No `time.Sleep` in tests — use channels or `sync.WaitGroup`.
- **Standard library first:** Prefer `slices`, `maps`, `errors.Is/As`, `min`/`max` builtins over custom helpers or reflection.

### Concurrency & Locking

- **Never hold a mutex during I/O.** Snapshot under the lock, release, then do I/O.
- **Always `defer mu.Unlock()`.** Only exception: intentional snapshot-then-release, marked with `// --- no lock held below this line ---`.
- **Every `select` must watch `ctx.Done()`.** Goroutines blocked without a context escape route leak forever.
- **Use `sync.Once` or `CompareAndSwap` for idempotent shutdown.** Prevents double-close panics.

### Benchmarking & Profiling

All performance-sensitive packages **must** maintain benchmark suites using modern Go 1.24+ `b.Loop()` to guarantee statistical correctness and prevent dead code elimination.

```bash
# Run all benchmarks in a package
go test -bench=. -benchmem ./pkg/...

# Run benchmarks with statistical rigor (10 runs)
go test -bench=. -benchmem -count=10 ./pkg/...

# Statistically compare baseline vs optimized runs (go install golang.org/x/perf/cmd/benchstat@latest)
benchstat baseline.txt optimized.txt
```

To analyze CPU bottlenecks and heap memory allocations, generate and inspect profiling data directly from your benchmarks:

```bash
# Generate profiles from benchmarks
go test -bench=BenchmarkMyFunc -cpuprofile=cpu.prof ./pkg/mypackage
go test -bench=BenchmarkMyFunc -memprofile=mem.prof ./pkg/mypackage

# Audit profiles
go tool pprof cpu.prof
go tool pprof -alloc_objects mem.prof
```

### Commit Convention

All commits must follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/):

```
<type>[optional scope]: <description>
```

| Type | When to use |
|------|-------------|
| `feat` | New user-visible capability |
| `fix` | Bug patch |
| `perf` | Performance improvement with benchmark evidence |
| `refactor` | Code restructuring, no behavior change |
| `test` | Adding or improving tests |
| `docs` | Documentation only |
| `chore` | Build, CI, dependency updates |

Append `!` or add `BREAKING CHANGE:` footer for any public API or wire-format change.
