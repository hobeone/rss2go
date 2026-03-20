# rss2go — RSS-to-Email Daemon

`rss2go` is a lightweight, modern RSS feed aggregator and notifier. It periodically scrapes RSS/Atom feeds and emails new items to subscribed addresses.

## Features

-   **Modular Architecture**: Decoupled crawler, mailer, and feed watcher pools.
-   **Pure Go Persistence**: Uses SQLite (`modernc.org/sqlite`) for storage—no CGO required.
-   **Structured Logging**: Powered by Go 1.22's `log/slog`.
-   **Robust Configuration**: Load settings from YAML, JSON, TOML, or Environment Variables via `spf13/viper`.
-   **Observability**: Simple Prometheus-style `/metrics` endpoint.
-   **Graceful Shutdown**: Context-aware design ensures all workers finish clean on SIGINT/SIGTERM.

---

## 🚀 Installation

Ensure you have [Go 1.22+](https://go.dev/dl/) installed.

```bash
git clone https://github.com/hobe/rss2go.git
cd rss2go
go build -o rss2go ./cmd/rss2go
```

---

## ⚙️ Configuration

`rss2go` looks for a configuration file named `rss2go.yaml` in the current directory by default. You can override this with the `--config` flag.

### Example `rss2go.yaml`

```yaml
# Database location
db_path: "rss2go.db"

# Log level: debug, info, warn, error
log_level: "info"

# Polling settings
poll_interval: "1h"
poll_jitter: "5m"

# Crawler settings
crawler_pool_size: 5
crawler_timeout: "30s"

# Mailer settings (SMTP)
smtp_server: "smtp.gmail.com"
smtp_port: 587
smtp_user: "your-email@gmail.com"
smtp_pass: "your-app-password"
smtp_sender: "rss2go@example.com"
use_tls: true

# Mailer settings (Alternative: Local Sendmail)
# sendmail: "/usr/sbin/sendmail"

# Metrics server
metrics_addr: ":8080"
```

### Environment Variables

All settings can be overridden using environment variables prefixed with `RSS2GO_`:

-   `RSS2GO_SMTP_PASS=mysecret ./rss2go daemon`
-   `RSS2GO_DB_PATH=/var/lib/rss2go.db ./rss2go daemon`

---

## 🛠️ Usage

### 1. Initialize the Database
The database is automatically initialized and migrated when you run any command.

### 2. Add a Feed
```bash
./rss2go add-feed "https://go.dev/blog/feed.atom" "Go Blog"
```

### 3. Add a User
```bash
./rss2go add-user "subscriber@example.com"
```

### 4. Subscribe the User to the Feed
List feeds to find the ID:
```bash
./rss2go list-feeds
```
Then subscribe using the email and feed ID:
```bash
./rss2go subscribe "subscriber@example.com" 1
```

### 5. Start the Daemon
```bash
./rss2go daemon
```

---

## 📈 Monitoring

If `metrics_addr` is configured, you can view the internal state:

```bash
curl http://localhost:8080/metrics
```

**Metrics included:**
-   `feeds_crawled_total`: Total successful crawls.
-   `feeds_crawled_errors`: Total crawl failures.
-   `emails_sent_total`: Total notifications sent.

---

## 🧪 Testing

Run the full test suite to ensure everything is working correctly:

```bash
go test ./...
```
