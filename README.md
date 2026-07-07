# rss2go

`rss2go` is a self-hosted syndication aggregator and notification daemon. It periodically polls structured web feeds (such as RSS or Atom), runs HTML sanitization, processes full-text article extraction, and emails new items to subscribed recipients. It features an embedded Material 3 SPA dashboard for operators to manage feeds, subscribers, system telemetry, and view live crawl logs.

---

## 🛠️ Building the Application

To compile the daemon, you must first build the frontend static assets so they can be embedded into the Go binary.

### 1. Build the Frontend SPA
Ensure you have Node.js and npm installed.
```bash
# Navigate to the frontend directory
cd frontend

# Install Svelte 5 & Vite dependencies
npm install

# Build the production bundle (outputs directly to the Go server's ui/dist directory)
npm run build

# Return to root directory
cd ..
```

### 2. Compile the Go Daemon
```bash
# Build the binary
go build -o rss2go ./cmd/rss2go
```

---

## 🚀 Running the Daemon CLI

`rss2go` uses standard CLI flags with automatic environment variable fallbacks.

### CLI Flag Configuration Reference

| Flag | Environment Variable | Default Value | Description |
|------|----------------------|---------------|-------------|
| `-db` | `RSS2GO_DB` | `rss2go.db` | Path to the SQLite database file (WAL mode). |
| `-addr` | `RSS2GO_ADDR` | `:8080` | Bind address for the HTTP REST API & Dashboard. |
| `-pass` | `RSS2GO_PASSWORD` | *None* | Password required to unlock the operator panel. If empty, the panel remains open. |
| `-mailer` | `RSS2GO_MAILER` | `sendmail` | Outbox delivery system to use (`smtp`, `sendmail`, or `mock`). |
| `-crawlers` | `RSS2GO_CRAWLERS` | `4` | Maximum concurrent background feed crawler workers. |
| `-smtp-host` | `RSS2GO_SMTP_HOST` | `localhost` | Hostname of the target SMTP server. |
| `-smtp-port` | `RSS2GO_SMTP_PORT` | `587` | Port of the target SMTP server. |
| `-smtp-user` | `RSS2GO_SMTP_USER` | *None* | Username for SMTP Plain authentication. |
| `-smtp-pass` | `RSS2GO_SMTP_PASS` | *None* | Password for SMTP Plain authentication. |
| `-smtp-from` | `RSS2GO_SMTP_FROM` | `rss2go@localhost`| Email address displayed in the `From` header. |
| `-smtp-security` | `RSS2GO_SMTP_SECURITY` | `starttls` | Transport security mode to use (`none`, `starttls`, or `ssl`). |

---

## 💡 Configuration Examples

### 1. Local Development & Testing (Dry-Run Mode)
This configuration opens a dashboard locally on port `8080` without requiring an email server (dispatches logs to the terminal instead of sending emails).
```bash
./rss2go -mailer mock -pass myoperatorpass
```

### 2. Standard Production SMTP (e.g. Gmail / Mailgun)
This launches the daemon with a SQLite database, configures the HTTP server, and routes durable email outbox deliveries through secure SMTP with `STARTTLS`.
```bash
./rss2go \
  -db /var/lib/rss2go/rss2go.db \
  -addr :80 \
  -pass super-secure-admin-pass \
  -mailer smtp \
  -smtp-host smtp.mailgun.org \
  -smtp-port 587 \
  -smtp-user postmaster@mg.example.com \
  -smtp-pass mysecretpassword \
  -smtp-from aggregator@example.com \
  -smtp-security starttls \
  -crawlers 8
```

### 3. Local Sendmail Binary Configuration
Useful if you are running on a server that has a local postfix/sendmail configuration.
```bash
./rss2go -mailer sendmail -smtp-from noreply@myserver.com
```

You can also leverage environment variables for configuration to avoid passing secrets in CLI parameters:
```bash
export RSS2GO_DB="/var/lib/rss2go/rss2go.db"
export RSS2GO_PASSWORD="super-secure-admin-pass"
export RSS2GO_MAILER="smtp"
export RSS2GO_SMTP_HOST="smtp.mailgun.org"
export RSS2GO_SMTP_PASS="mysecretpassword"

./rss2go
```

---

## 🔒 Operator Panel Access

Once the daemon starts, access the dashboard by navigating to the bind address in your browser (e.g., `http://localhost:8080`). 

If a password was configured via `-pass` or `RSS2GO_PASSWORD`, unlock the panel with that password. Inside, you can:
- Register target feed XML endpoints.
- Trigger dry-run crawl reports to test HTML sanitization and CSS selectors.
- Check live Server-Sent Events logs streaming from the scraper.
- Manage recipient email addresses and subscribe them to specific feeds.

---

## ⚡ HTML Scraper Sidecar Subcommand

Some websites do not publish RSS or Atom feeds. `rss2go` has a built-in **Scraper Sidecar** mode that translates HTML websites into standard RSS feeds on-the-fly.

### Running the Sidecar
Run `rss2go sidecar` as a subcommand to launch the scraper HTTP server.

```bash
./rss2go sidecar -addr :8081
```

Or configure via environment variables:
```bash
export RSS2GO_SIDECAR_ADDR=":8081"
./rss2go sidecar
```

### Scraping API Endpoints
The sidecar exposes a `GET /scrape` endpoint accepting query parameters:
- `url`: The target website page to fetch and parse.
- `item`: CSS selector of the repeating elements containing each post.
- `title`: CSS selector for the post's title (evaluated relative to `item`).
- `link`: CSS selector for the post's link `href` attribute (relative to `item`).
- `description`: (Optional) CSS selector for the post's body text (relative to `item`).

### Example: Tracking Hacker News
To track Hacker News, you can register a feed URL inside the `rss2go` operator dashboard that points to your local sidecar server:

```text
http://localhost:8081/scrape?url=https://news.ycombinator.com&item=tr.athing&title=span.titleline>a&link=span.titleline>a
```

The scraper sidecar will:
1. Fetch `https://news.ycombinator.com` on each poll.
2. Resolve CSS selectors to extract title, links, and description.
3. Automatically rewrite relative links (e.g. `item?id=...`) to absolute links (e.g. `https://news.ycombinator.com/item?id=...`).
4. Output a standardized RSS XML format, allowing the main `rss2go` daemon to seamlessly aggregate, sanitize, and email posts.

