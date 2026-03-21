package watcher

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hobe/rss2go/internal/crawler"
	"github.com/hobe/rss2go/internal/db"
	"github.com/hobe/rss2go/internal/mailer"
	"github.com/hobe/rss2go/internal/metrics"
	"github.com/hobe/rss2go/internal/models"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

// CrawlerPool defines the interface for submitting crawl requests.
type CrawlerPool interface {
	Submit(crawler.CrawlRequest)
}

// MailerPool defines the interface for submitting mail requests.
type MailerPool interface {
	Submit(mailer.MailRequest)
}

// Watcher manages the polling of a single feed.
type Watcher struct {
	feed            models.Feed
	store           db.Store
	crawler         CrawlerPool
	mailer          MailerPool
	logger          *slog.Logger
	interval        time.Duration
	jitter          time.Duration
	strictPol       *bluemonday.Policy
	contentPol      *bluemonday.Policy
	currentInterval time.Duration
	mu              sync.Mutex
}

const maxBackoff = 24 * time.Hour

// New creates a new feed watcher.
func New(feed models.Feed, store db.Store, c CrawlerPool, m MailerPool, interval, jitter time.Duration, logger *slog.Logger) *Watcher {

	// Strict policy for titles and subjects (no HTML)
	strictPol := bluemonday.StrictPolicy()

	// Let's build a strict safe HTML policy from scratch or modify UGC.
	// A simpler safe policy:
	contentPol := bluemonday.NewPolicy()
	contentPol.AllowStandardURLs()
	contentPol.AllowAttrs("href").OnElements("a")
	contentPol.RequireNoReferrerOnLinks(true)
	contentPol.RequireParseableURLs(true)
	contentPol.AllowElements("b", "i", "u", "strong", "em", "p", "br", "div", "span", "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "img")
	contentPol.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")

	return &Watcher{
		feed:            feed,
		store:           store,
		crawler:         c,
		mailer:          m,
		logger:          logger.With("feed_id", feed.ID, "url", feed.URL),
		interval:        interval,
		jitter:          jitter,
		strictPol:       strictPol,
		contentPol:      contentPol,
		currentInterval: interval,
	}
}

// Run starts the watcher loop.
func (w *Watcher) Run(ctx context.Context) {
	initialWait := w.getJitter()
	if !w.feed.LastPoll.IsZero() {
		nextScheduledPoll := w.feed.LastPoll.Add(w.interval)
		wait := time.Until(nextScheduledPoll)
		if wait > 0 {
			initialWait += wait
		}
	}

	w.logger.Info("starting watcher", "next_poll", time.Now().Add(initialWait))

	// Initial wait (last poll offset + jitter)
	select {
	case <-time.After(initialWait):
	case <-ctx.Done():
		return
	}

	for {
		w.crawl(ctx)

		w.mu.Lock()
		wait := w.currentInterval
		w.mu.Unlock()

		w.logger.Info("poll triggered", "next_poll", time.Now().Add(wait))

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			w.logger.Info("watcher shutting down")
			return
		}
	}
}

func (w *Watcher) crawl(ctx context.Context) {
	w.logger.Debug("triggering crawl")
	w.crawler.Submit(crawler.CrawlRequest{
		FeedID: w.feed.ID,
		URL:    w.feed.URL,
		Ctx:    ctx,
	})
}

// HandleResponse processes a crawl result.
// This is called by the orchestrator which listens to the crawler pool.
func (w *Watcher) HandleResponse(ctx context.Context, resp crawler.CrawlResponse) {
	atomic.AddUint64(&metrics.FeedsCrawledTotal, 1)
	if resp.Error != nil {
		atomic.AddUint64(&metrics.FeedsCrawledErrors, 1)
		w.logger.Error("crawl failed", "error", resp.Error)

		// Record error in DB
		snippet := ""
		if len(resp.Body) > 0 {
			maxLen := min(len(resp.Body), 500)
			snippet = string(resp.Body[:maxLen])
		} else {
			snippet = resp.Error.Error()
		}

		if err := w.store.UpdateFeedError(ctx, w.feed.ID, resp.StatusCode, snippet); err != nil {
			w.logger.Error("failed to update feed error in DB", "error", err)
		}

		w.mu.Lock()
		w.currentInterval *= 2
		if w.currentInterval > maxBackoff {
			w.currentInterval = maxBackoff
		}
		newInterval := w.currentInterval
		w.mu.Unlock()

		w.logger.Warn("backing off due to error", "new_interval", newInterval)
		return
	}

	w.mu.Lock()
	w.currentInterval = w.interval
	w.mu.Unlock()

	// Clear error in DB on success
	if err := w.store.UpdateFeedError(ctx, w.feed.ID, 0, ""); err != nil {
		w.logger.Error("failed to clear feed error in DB", "error", err)
	}

	fp := gofeed.NewParser()
	feed, err := fp.Parse(bytes.NewReader(resp.Body))
	if err != nil {
		w.logger.Error("failed to parse feed", "error", err)
		return
	}

	users, err := w.store.GetUsersForFeed(ctx, w.feed.ID)
	if err != nil {
		w.logger.Error("failed to get users for feed", "error", err)
		return
	}

	if len(users) == 0 {
		w.logger.Debug("no users subscribed to feed")
		return
	}

	var userEmails []string
	for u := range users {
		userEmails = append(userEmails, users[u].Email)
	}

	newItemsCount := 0
	for item := range feed.Items {
		itm := feed.Items[item]
		guid := itm.GUID
		if guid == "" {
			guid = itm.Link
		}

		seen, err := w.store.IsSeen(ctx, w.feed.ID, guid)
		if err != nil {
			w.logger.Error("failed to check if item is seen", "guid", guid, "error", err)
			continue
		}

		if seen {
			continue
		}

		subject, body := w.FormatItem(feed.Title, itm)

		w.logger.Info("new item found", "title", itm.Title, "guid", guid)
		w.mailer.Submit(mailer.MailRequest{
			To:      userEmails,
			Subject: subject,
			Body:    body,
		})

		if err := w.store.MarkSeen(ctx, w.feed.ID, guid); err != nil {
			w.logger.Error("failed to mark item as seen", "guid", guid, "error", err)
		}
		newItemsCount++
	}

	if err := w.store.UpdateFeedLastPoll(ctx, w.feed.ID); err != nil {
		w.logger.Error("failed to update last poll time", "error", err)
	}

	w.logger.Debug("crawl complete", "new_items", newItemsCount)
}

// FormatItem sanitizes and formats a feed item for an email.
func (w *Watcher) FormatItem(feedTitle string, item *gofeed.Item) (subject, body string) {
	safeTitle := strings.TrimSpace(w.strictPol.Sanitize(item.Title))
	safeFeedTitle := strings.TrimSpace(w.strictPol.Sanitize(feedTitle))
	safeLink := strings.TrimSpace(w.strictPol.Sanitize(item.Link))

	// Use Content if available, otherwise Description.
	// Many feeds use Description for summary and Content for full text.
	content := item.Content
	if content == "" {
		content = item.Description
	}

	// Pre-process to clean content, remove trackers, and handle embedded elements
	content = cleanFeedContent(content)

	safeContent := strings.TrimSpace(w.contentPol.Sanitize(content))

	subject = "[" + safeFeedTitle + "] " + safeTitle
	body = safeContent + "<br><br><a href=\"" + safeLink + "\">Read more</a>"
	return
}

func cleanFeedContent(htmlStr string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr // Fallback to returning original string if parsing fails
	}

	// Replace iframes with links
	doc.Find("iframe").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if exists && src != "" {
			replacement := fmt.Sprintf(`<a href="%s">[Embedded Content: %s]</a>`, html.EscapeString(src), html.EscapeString(src))
			s.ReplaceWithHtml(replacement)
		} else {
			s.Remove()
		}
	})

	// Remove feedsportal tracking links
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && strings.Contains(strings.ToLower(href), "da.feedsportal.com") {
			s.Remove()
		}
	})

	// Remove tracking images
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		width, _ := s.Attr("width")
		height, _ := s.Attr("height")
		src, _ := s.Attr("src")

		// Remove if width or height is 1 or 0
		if width == "1" || width == "0" || height == "1" || height == "0" {
			s.Remove()
			return
		}

		// Remove if URL contains common tracking keywords
		srcLower := strings.ToLower(src)
		if strings.Contains(srcLower, "tracker") || strings.Contains(srcLower, "pixel") || strings.Contains(srcLower, "analytics") {
			s.Remove()
			return
		}
	})

	htmlStr, _ = doc.Find("body").Html()
	return htmlStr
}

func (w *Watcher) getJitter() time.Duration {
	if w.jitter == 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(w.jitter)))
}
