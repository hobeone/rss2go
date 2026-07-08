//go:build uitest

package uitest

import (
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"

	"rss2go/internal/crawler"
	"rss2go/internal/database"
	"rss2go/internal/extractor"
	"rss2go/internal/sanitizer"
	"rss2go/internal/scheduler"
	"rss2go/internal/server"
	"rss2go/internal/server/ui"
)

const testPassword = "uitest-password"

type testEnv struct {
	Server    *httptest.Server
	BaseURL   string
	Repo      *database.Repository
	DB        *sql.DB
	Scheduler *scheduler.Scheduler
	PW        *playwright.Playwright
	Browser   playwright.Browser
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	// Verify the embedded UI dist exists.
	if _, err := fs.Stat(ui.Files, "dist/index.html"); err != nil {
		t.Fatal("ui/dist/index.html not found — run 'cd frontend && npm run build' first")
	}

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	repo := database.NewRepository(db)

	cr := crawler.NewCrawler(nil)
	ex := extractor.NewExtractor(nil)
	sa := sanitizer.NewSanitizer(600)

	schedCfg := scheduler.Config{
		PollInterval: 10 * time.Second,
		MaxWorkers:   2,
	}
	sched := scheduler.New(repo, cr, ex, sa, schedCfg, nil)

	srvCfg := server.Config{
		Addr:     "127.0.0.1:0", // Ephemeral port
	}
	srv := server.New(repo, sched, cr, ex, sa, srvCfg, nil)

	handler, err := srv.Handler()
	if err != nil {
		_ = db.Close()
		t.Fatalf("server.Handler: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.HandleFunc("/testfeed.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0">
<channel>
 <title>Mock RSS Feed</title>
 <description>A mock feed for Playwright E2E testing</description>
 <link>http://localhost</link>
 <lastBuildDate>Sat, 27 Jun 2026 09:00:00 GMT</lastBuildDate>
 <item>
  <title>Mock Item 1</title>
  <description>First mock item description</description>
  <link>http://localhost/item1</link>
  <guid>mock-guid-1</guid>
  <pubDate>Sat, 27 Jun 2026 09:00:00 GMT</pubDate>
 </item>
 <item>
  <title>Mock Item 2</title>
  <description>Second mock item description</description>
  <link>http://localhost/item2</link>
  <guid>mock-guid-2</guid>
  <pubDate>Sat, 27 Jun 2026 08:30:00 GMT</pubDate>
 </item>
</channel>
</rss>
`))
	})

	ts := httptest.NewServer(mux)

	// Launch Playwright.
	pw, err := playwright.Run()
	if err != nil {
		ts.Close()
		_ = db.Close()
		t.Fatalf("playwright.Run: %v", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		_ = pw.Stop()
		ts.Close()
		_ = db.Close()
		t.Fatalf("chromium.Launch: %v", err)
	}

	env := &testEnv{
		Server:    ts,
		BaseURL:   ts.URL,
		Repo:      repo,
		DB:        db,
		Scheduler: sched,
		PW:        pw,
		Browser:   browser,
	}

	t.Cleanup(func() {
		_ = browser.Close()
		_ = pw.Stop()
		ts.Close()
		_ = db.Close()
	})

	return env
}

func (e *testEnv) newPage(t *testing.T) playwright.Page {
	t.Helper()
	page, err := e.Browser.NewPage()
	if err != nil {
		t.Fatalf("browser.NewPage: %v", err)
	}
	page.OnDialog(func(dialog playwright.Dialog) {
		t.Logf("[DIALOG EVENT] type=%q message=%q", dialog.Type(), dialog.Message())
		if err := dialog.Accept(); err != nil {
			t.Logf("[DIALOG EVENT] accept failed: %v", err)
		}
	})
	var logs []string
	page.OnConsole(func(msg playwright.ConsoleMessage) {
		logs = append(logs, fmt.Sprintf("%s: %s", msg.Type(), msg.Text()))
	})
	t.Cleanup(func() {
		if t.Failed() && len(logs) > 0 {
			t.Logf("--- Browser Console Logs ---")
			for _, log := range logs {
				t.Logf("[BROWSER] %s", log)
			}
			t.Logf("----------------------------")
		}
		_ = page.Close()
	})
	return page
}

func (e *testEnv) navigate(t *testing.T, page playwright.Page, path string) {
	t.Helper()
	url := fmt.Sprintf("%s%s", e.BaseURL, path)
	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		t.Fatalf("page.Goto(%s): %v", url, err)
	}
}



func screenshotOnFailure(t *testing.T, page playwright.Page) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			_ = os.MkdirAll("screenshots", 0o755)
			path := fmt.Sprintf("screenshots/%s.png", t.Name())
			if _, err := page.Screenshot(playwright.PageScreenshotOptions{
				Path:     playwright.String(path),
				FullPage: playwright.Bool(true),
			}); err != nil {
				t.Logf("screenshot failed: %v", err)
			} else {
				t.Logf("Screenshot saved: %s", path)
			}
			if content, err := page.Content(); err == nil {
				t.Logf("Page HTML on failure:\n%s", content)
			}
		}
	})
}
