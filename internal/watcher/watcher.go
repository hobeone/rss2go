package watcher

import (
	"bytes"
	"context"
	"log/slog"
	"math/rand"
	"sync/atomic"
	"time"

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
	feed     models.Feed
	store    db.Store
	crawler  CrawlerPool
	mailer   MailerPool
	logger   *slog.Logger
	interval time.Duration
	jitter   time.Duration
}

// New creates a new feed watcher.
func New(feed models.Feed, store db.Store, c CrawlerPool, m MailerPool, interval, jitter time.Duration, logger *slog.Logger) *Watcher {
	return &Watcher{
		feed:     feed,
		store:    store,
		crawler:  c,
		mailer:   m,
		logger:   logger.With("feed_id", feed.ID, "url", feed.URL),
		interval: interval,
		jitter:   jitter,
	}
}

// Run starts the watcher loop.
func (w *Watcher) Run(ctx context.Context) {
	w.logger.Info("starting watcher")

	// Initial jitter to avoid thundering herd on startup
	select {
	case <-time.After(w.getJitter()):
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial crawl
	w.crawl(ctx)

	for {
		select {
		case <-ticker.C:
			w.crawl(ctx)
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

		w.logger.Info("new item found", "title", item.Title, "guid", guid)
		w.mailer.Submit(mailer.MailRequest{
			To:      userEmails,
			Subject: "[" + feed.Title + "] " + item.Title,
			Body:    item.Description + "<br><br><a href=\"" + item.Link + "\">Read more</a>",
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
