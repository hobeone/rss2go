# rss2go — RSS-to-Email Daemon

`rss2go` is a lightweight, modern RSS feed aggregator and notifier. It periodically scrapes RSS/Atom feeds and emails new items to subscribed addresses.

## Features

-   **Modular Architecture**: Decoupled crawler, mailer, and feed watcher pools.
-   **Pure Go Persistence**: Uses SQLite (`modernc.org/sqlite`) for storage—no CGO required.
-   **Dynamic Sync**: Automatically detects added or removed feeds without restarting the daemon.
-   **Structured Logging**: Powered by Go's `log/slog`.
-   **Robust Configuration**: Load settings from YAML, JSON, TOML, or Environment Variables via `spf13/viper`.
-   **Observability**: Prometheus-style `/metrics` endpoint.
-   **Graceful Shutdown**: Context-aware design ensures all workers finish clean on SIGINT/SIGTERM.

---

## 🚀 Installation

Ensure you have [Go 1.24+](https://go.dev/dl/) installed.

```bash
git clone https://github.com/hobeone/rss2go.git
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

---

## 🛠️ Usage

`rss2go` uses a hierarchical command structure.

### 1. Initialize & Start Daemon
```bash
./rss2go daemon
```

### 2. Manage Feeds
```bash
# Add a feed
./rss2go feed add "https://go.dev/blog/feed.atom" "Go Blog"

# List feeds (find IDs)
./rss2go feed list

# Delete a feed
./rss2go feed del 1  # or URL: ./rss2go feed del https://...

# Catch up unread items without mailing
./rss2go feed catchup 1
```

### 3. Manage Users & Subscriptions
```bash
# Add a user
./rss2go user add "subscriber@example.com"

# Subscribe a user to a feed
./rss2go user subscribe "subscriber@example.com" 1

# Unsubscribe a user
./rss2go user unsubscribe "subscriber@example.com" 1
```

---

## 📈 Monitoring

If `metrics_addr` is configured, you can view the internal state:

```bash
curl http://localhost:8080/metrics
```

---

## 🧪 Testing

Run the full test suite to ensure everything is working correctly:

```bash
go test -race ./...
```
