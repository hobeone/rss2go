package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hobeone/rss2go/internal/config"
	"github.com/hobeone/rss2go/internal/crawler"
	"github.com/hobeone/rss2go/internal/db"
	"github.com/hobeone/rss2go/internal/mailer"
	"github.com/hobeone/rss2go/internal/metrics"
	"github.com/hobeone/rss2go/internal/models"
	"github.com/hobeone/rss2go/internal/watcher"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the rss2go daemon",
	RunE:  runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}

func runDaemon(_ *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}

	logger := getLogger(cfg)

	store, err := getStore(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	m := &metrics.Set{}

	cPool := crawler.NewPool(cfg.CrawlerPoolSize, cfg.CrawlerTimeout, logger)
	defer cPool.Close()

	mPool := mailer.NewPool(cfg.MailerPoolSize, cfg, m, store, logger)
	defer mPool.Close()

	metrics.StartStatsLoop(ctx, m, logger)
	metrics.Start(ctx, cfg, m, logger)

	scheduler := watcher.NewScheduler(cPool, mPool, store, m, cfg.PollInterval, cfg.PollJitter, cfg.MaxImageWidth, logger)

	feeds, err := store.GetFeeds(ctx)
	if err != nil {
		return fmt.Errorf("failed to load feeds: %w", err)
	}

	for _, f := range feeds {
		scheduler.Register(f)
	}

	logger.Info("daemon started", "feeds_count", len(feeds))

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		resyncFeeds(ctx, store, scheduler, logger)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		scheduler.Run(ctx)
	}()

	<-ctx.Done()
	logger.Info("shutting down, waiting for active jobs to finish")
	wg.Wait()

	return nil
}

// feedSyncer is the subset of watcher.Scheduler methods used by resyncFeeds.
// Defined here so the sync logic can be tested without a real Scheduler.
type feedSyncer interface {
	GetWatcher(feedID int64) (*watcher.Watcher, bool)
	Register(feed models.Feed)
	Unregister(feedID int64)
	GetActiveFeedIDs() []int64
}

// resyncFeeds runs a ticker loop that detects feeds added, removed, or updated
// in the database and adjusts the running set of watchers accordingly.
func resyncFeeds(ctx context.Context, store db.Store, s feedSyncer, logger *slog.Logger) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncFeedsOnce(ctx, store, s, logger)
		}
	}
}

// syncFeedsOnce performs a single reconciliation pass: registers new feeds,
// re-registers changed feeds, and unregisters feeds removed from the database.
func syncFeedsOnce(ctx context.Context, store db.Store, s feedSyncer, logger *slog.Logger) {
	currentFeeds, err := store.GetFeeds(ctx)
	if err != nil {
		logger.Error("failed to sync feeds", "error", err)
		return
	}

	dbFeedsMap := make(map[int64]models.Feed)
	for _, f := range currentFeeds {
		dbFeedsMap[f.ID] = f
		if w, ok := s.GetWatcher(f.ID); !ok {
			logger.Info("sync: registering new feed", "feed_id", f.ID, "url", f.URL)
			s.Register(f)
		} else {
			current := w.Feed()
			if feedMetadataChanged(current, f) {
				logger.Info("sync: feed metadata changed, re-registering", "feed_id", f.ID)
				s.Register(f)
			}
		}
	}

	for _, id := range s.GetActiveFeedIDs() {
		if _, exists := dbFeedsMap[id]; !exists {
			logger.Info("sync: unregistering removed feed", "feed_id", id)
			s.Unregister(id)
		}
	}
}

// feedMetadataChanged reports whether any scheduler-relevant field differs
// between the running watcher's copy and the database copy of a feed.
// Add new fields here whenever models.Feed gains a field that affects
// crawl or email behaviour.
func feedMetadataChanged(running, db models.Feed) bool {
	return running.URL != db.URL ||
		running.Title != db.Title ||
		running.FullArticle != db.FullArticle ||
		running.ExtractionStrategy != db.ExtractionStrategy ||
		running.ExtractionConfig != db.ExtractionConfig
}
