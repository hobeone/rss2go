package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
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

func runDaemon(cmd *cobra.Command, args []string) error {
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

	cPool := crawler.NewPool(cfg.CrawlerPoolSize, cfg.CrawlerTimeout, logger)
	defer cPool.Close()

	mPool := mailer.NewPool(cfg.MailerPoolSize, cfg, store, logger)
	defer mPool.Close()

	metrics.StartStatsLoop(ctx, logger)
	metrics.Start(ctx, cfg, logger)

	scheduler := watcher.NewScheduler(cPool, mPool, store, cfg.PollInterval, cfg.PollJitter, cfg.MaxImageWidth, logger)

	feeds, err := store.GetFeeds(ctx)
	if err != nil {
		return fmt.Errorf("failed to load feeds: %w", err)
	}

	for _, f := range feeds {
		scheduler.Register(f)
	}

	logger.Info("daemon started", "feeds_count", len(feeds))

	go resyncFeeds(ctx, store, scheduler, logger)
	go scheduler.Run(ctx)

	<-ctx.Done()
	logger.Info("shutting down")

	return nil
}

// resyncFeeds runs a ticker loop that detects feeds added, removed, or updated
// in the database and adjusts the running set of watchers accordingly.
func resyncFeeds(
	ctx context.Context,
	store db.Store,
	scheduler *watcher.Scheduler,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentFeeds, err := store.GetFeeds(ctx)
			if err != nil {
				logger.Error("failed to sync feeds", "error", err)
				continue
			}

			dbFeedsMap := make(map[int64]models.Feed)
			for _, f := range currentFeeds {
				dbFeedsMap[f.ID] = f
				if w, ok := scheduler.GetWatcher(f.ID); !ok {
					logger.Info("sync: registering new feed", "feed_id", f.ID, "url", f.URL)
					scheduler.Register(f)
				} else {
					current := w.Feed()
					if feedMetadataChanged(current, f) {
						logger.Info("sync: feed metadata changed, re-registering", "feed_id", f.ID)
						scheduler.Register(f) // Register replaces the existing watcher
					}
				}
			}

			for _, id := range scheduler.GetActiveFeedIDs() {
				if _, exists := dbFeedsMap[id]; !exists {
					logger.Info("sync: unregistering removed feed", "feed_id", id)
					scheduler.Unregister(id)
				}
			}
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
