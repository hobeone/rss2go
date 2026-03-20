package watcher

import (
	"bytes"
	"context"
	"log/slog"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"github.com/hobe/rss2go/internal/crawler"
	"github.com/hobe/rss2go/internal/db"
	"github.com/hobe/rss2go/internal/mailer"
	"github.com/hobe/rss2go/internal/metrics"
	"github.com/hobe/rss2go/internal/models"
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
	feed       models.Feed
	store      db.Store
	crawler    CrawlerPool
	mailer     MailerPool
	logger     *slog.Logger
	interval   time.Duration
	jitter     time.Duration
	strictPol  *bluemonday.Policy
	contentPol *bluemonday.Policy
}

// New creates a new feed watcher.
func New(feed models.Feed, store db.Store, c CrawlerPool, m MailerPool, interval, jitter time.Duration, logger *slog.Logger) *Watcher {
	
	// Strict policy for titles and subjects (no HTML)
	strictPol := bluemonday.StrictPolicy()

	// Content policy for email bodies based on UGC, but strictly stripped of images and styles
	contentPol := bluemonday.UGCPolicy()
	
	// Remove images to prevent tracking pixels and inappropriate content
	contentPol.AllowElements() // Reset elements for images (there is no direct 'deny' element, so we just don't allow it if it was allowed)
	// Actually bluemonday.UGCPolicy allows images. To remove them, we can't easily "remove" an allowed element.
	// Wait, we CAN just build a custom policy or use UGC and then `contentPol.RequireNoReferrerOnLinks(true)`
	// Let's build a strict safe HTML policy from scratch or modify UGC.
	// A simpler safe policy:
	contentPol = bluemonday.NewPolicy()
	contentPol.AllowStandardURLs()
	contentPol.AllowAttrs("href").OnElements("a")
	contentPol.RequireNoReferrerOnLinks(true)
	contentPol.RequireParseableURLs(true)
	contentPol.AllowElements("b", "i", "u", "strong", "em", "p", "br", "div", "span", "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote")

	return &Watcher{
		feed:       feed,
		store:      store,
		crawler:    c,
		mailer:     m,
		logger:     logger.With("feed_id", feed.ID, "url", feed.URL),
		interval:   interval,
		jitter:     jitter,
		strictPol:  strictPol,
		contentPol: contentPol,
	}
}

// Run starts the watcher loop.
func (w *Watcher) Run(ctx context.Context) {
	jitter := w.getJitter()
	w.logger.Info("starting watcher", "next_poll", time.Now().Add(jitter))

	// Initial jitter to avoid thundering herd on startup
	select {
	case <-time.After(jitter):
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial crawl
	w.crawl(ctx)
	w.logger.Info("initial poll triggered", "next_poll", time.Now().Add(w.interval))

	for {
		select {
		case <-ticker.C:
			w.crawl(ctx)
			w.logger.Info("poll triggered", "next_poll", time.Now().Add(w.interval))
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
	})
}

// HandleResponse processes a crawl result.
// This is called by the orchestrator which listens to the crawler pool.
func (w *Watcher) HandleResponse(ctx context.Context, resp crawler.CrawlResponse) {
	atomic.AddUint64(&metrics.FeedsCrawledTotal, 1)
	if resp.Error != nil {
		atomic.AddUint64(&metrics.FeedsCrawledErrors, 1)
		w.logger.Error("crawl failed", "error", resp.Error)
		return
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
	for _, u := range users {
		userEmails = append(userEmails, u.Email)
	}

	newItemsCount := 0
	for _, item := range feed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		seen, err := w.store.IsSeen(ctx, w.feed.ID, guid)
		if err != nil {
			w.logger.Error("failed to check if item is seen", "guid", guid, "error", err)
			continue
		}

		if seen {
			continue
		}

		safeTitle := strings.TrimSpace(w.strictPol.Sanitize(item.Title))
		safeFeedTitle := strings.TrimSpace(w.strictPol.Sanitize(feed.Title))
		safeLink := strings.TrimSpace(w.strictPol.Sanitize(item.Link))

		// Use Content if available, otherwise Description. 
		// If both are available and different, we can include both or just Content.
		// Many feeds use Description for summary and Content for full text.
		content := item.Content
		if content == "" {
			content = item.Description
		}
		safeContent := strings.TrimSpace(w.contentPol.Sanitize(content))

		w.logger.Info("new item found", "title", safeTitle, "guid", guid)
		w.mailer.Submit(mailer.MailRequest{
			To:      userEmails,
			Subject: "[" + safeFeedTitle + "] " + safeTitle,
			Body:    safeContent + "<br><br><a href=\"" + safeLink + "\">Read more</a>",
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

func (w *Watcher) getJitter() time.Duration {
	if w.jitter == 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(w.jitter)))
}
