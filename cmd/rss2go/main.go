package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hobeone/rss2go"
	"github.com/hobeone/rss2go/internal/config"
	"github.com/hobeone/rss2go/internal/crawler"
	"github.com/hobeone/rss2go/internal/db"
	"github.com/hobeone/rss2go/internal/db/sqlite"
	"github.com/hobeone/rss2go/internal/mailer"
	"github.com/hobeone/rss2go/internal/metrics"
	"github.com/hobeone/rss2go/internal/models"
	"github.com/hobeone/rss2go/internal/watcher"
	"github.com/mmcdole/gofeed"
	"github.com/spf13/cobra"
)

var (
	cfgFile  string
	logLevel string
	rootCmd  = &cobra.Command{
		Use:   "rss2go",
		Short: "rss2go is an RSS-to-Email daemon",
	}

	daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Start the rss2go daemon",
		RunE:  runDaemon,
	}

	feedCmd = &cobra.Command{
		Use:   "feed",
		Short: "Manage RSS feeds",
	}

	feedAddCmd = &cobra.Command{
		Use:   "add [url] [title]",
		Short: "Add a new RSS feed",
		Args:  cobra.ExactArgs(2),
		RunE:  runAddFeed,
	}

	feedDelCmd = &cobra.Command{
		Use:   "del [feed-id or url]",
		Short: "Delete an RSS feed",
		Args:  cobra.ExactArgs(1),
		RunE:  runDelFeed,
	}

	feedUpdateCmd = &cobra.Command{
		Use:   "update [feed-id]",
		Short: "Update an RSS feed's URL or title",
		Args:  cobra.ExactArgs(1),
		RunE:  runUpdateFeed,
	}

	feedListCmd = &cobra.Command{
		Use:   "list",
		Short: "List all RSS feeds",
		RunE:  runListFeeds,
	}

	feedTestCmd = &cobra.Command{
		Use:   "test [url] [email]",
		Short: "Test a feed by sending its first item to an email",
		Args:  cobra.ExactArgs(2),
		RunE:  runTestFeed,
	}

	feedErrorsCmd = &cobra.Command{
		Use:   "errors",
		Short: "List feeds with recorded errors",
		RunE:  runListErrors,
	}

	feedCatchupCmd = &cobra.Command{
		Use:   "catchup [feed-id]",
		Short: "Mark all items in a feed (or all feeds) as seen without mailing",
		RunE:  runCatchup,
	}

	userCmd = &cobra.Command{
		Use:   "user",
		Short: "Manage users and subscriptions",
	}

	userAddCmd = &cobra.Command{
		Use:   "add [email]",
		Short: "Add a new user",
		Args:  cobra.ExactArgs(1),
		RunE:  runAddUser,
	}

	userSubscribeCmd = &cobra.Command{
		Use:   "subscribe [email] [feed-id or url]",
		Short: "Subscribe a user to a feed",
		Args:  cobra.ExactArgs(2),
		RunE:  runSubscribe,
	}

	userUnsubscribeCmd = &cobra.Command{
		Use:   "unsubscribe [email] [feed-id or url]",
		Short: "Unsubscribe a user from a feed",
		Args:  cobra.ExactArgs(2),
		RunE:  runUnsubscribe,
	}
)

var (
	catchupAll  bool
	updateURL   string
	updateTitle string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./rss2go.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "override log level (debug, info, warn, error)")

	rootCmd.AddCommand(daemonCmd)

	// Feed commands
	feedCmd.AddCommand(feedAddCmd)
	feedCmd.AddCommand(feedDelCmd)
	feedUpdateCmd.Flags().StringVar(&updateURL, "url", "", "New URL for the feed")
	feedUpdateCmd.Flags().StringVar(&updateTitle, "title", "", "New title for the feed")
	feedCmd.AddCommand(feedUpdateCmd)
	feedCmd.AddCommand(feedListCmd)
	feedCmd.AddCommand(feedTestCmd)
	feedCmd.AddCommand(feedErrorsCmd)

	feedCatchupCmd.Flags().BoolVar(&catchupAll, "all", false, "catchup all feeds")
	feedCmd.AddCommand(feedCatchupCmd)

	rootCmd.AddCommand(feedCmd)

	// User commands
	userCmd.AddCommand(userAddCmd)
	userCmd.AddCommand(userSubscribeCmd)
	userCmd.AddCommand(userUnsubscribeCmd)

	rootCmd.AddCommand(userCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getLogger(cfg *config.Config) *slog.Logger {
	levelStr := cfg.LogLevel
	if logLevel != "" {
		levelStr = logLevel
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

func getStore(logger *slog.Logger) (*sqlite.Store, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, err
	}
	store, err := sqlite.New(cfg.DBPath, logger)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(rss2go.MigrationsFS); err != nil {
		return nil, err
	}
	return store, nil
}

func runDaemon(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}

	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cPool := crawler.NewPool(cfg.CrawlerPoolSize, cfg.CrawlerTimeout, logger)
	defer cPool.Close()

	mPool := mailer.NewPool(cfg.MailerPoolSize, cfg, logger)
	defer mPool.Close()

	metrics.Start(cfg, logger)

	registry := watcher.NewRegistry(cPool, logger)
	go registry.Start(ctx)

	feeds, err := store.GetFeeds(ctx)
	if err != nil {
		return fmt.Errorf("failed to load feeds: %w", err)
	}

	for _, f := range feeds {
		w := watcher.New(f, store, cPool, mPool, cfg.PollInterval, cfg.PollJitter, logger)
		registry.Register(w)
		go w.Run(ctx)
	}

	logger.Info("daemon started", "feeds_count", len(feeds))

	// Resync loop
	go func() {
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

				// Find new or updated feeds
				dbFeedsMap := make(map[int64]models.Feed)
				for _, f := range currentFeeds {
					dbFeedsMap[f.ID] = f
					if w, ok := registry.GetWatcher(f.ID); !ok {
						logger.Info("sync: starting new watcher", "feed_id", f.ID, "url", f.URL)
						w := watcher.New(f, store, cPool, mPool, cfg.PollInterval, cfg.PollJitter, logger)
						registry.Register(w)
						go w.Run(ctx)
					} else {
						// Check if metadata has changed
						currentFeed := w.Feed()
						if currentFeed.URL != f.URL || currentFeed.Title != f.Title {
							logger.Info("sync: feed metadata changed, restarting watcher", "feed_id", f.ID)
							w.Stop()
							registry.Unregister(f.ID)

							newW := watcher.New(f, store, cPool, mPool, cfg.PollInterval, cfg.PollJitter, logger)
							registry.Register(newW)
							go newW.Run(ctx)
						}
					}
				}

				// Find removed feeds
				activeFeedIDs := registry.GetActiveFeedIDs()
				for _, id := range activeFeedIDs {
					if _, exists := dbFeedsMap[id]; !exists {
						logger.Info("sync: stopping removed watcher", "feed_id", id)
						if w, ok := registry.GetWatcher(id); ok {
							w.Stop()
						}
						registry.Unregister(id)
					}
				}
			}
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	return nil
}

func runAddFeed(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	id, err := store.AddFeed(context.Background(), args[0], args[1])
	if err != nil {
		return err
	}
	fmt.Printf("Added feed: %s (ID: %d)\n", args[1], id)
	return nil
}

func runDelFeed(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	arg := args[0]
	var id int64
	// Try parsing as ID first
	if _, err := fmt.Sscanf(arg, "%d", &id); err == nil {
		if err := store.DeleteFeed(context.Background(), id); err != nil {
			return err
		}
		fmt.Printf("Deleted feed with ID: %d\n", id)
	} else {
		// Assume it's a URL
		if err := store.DeleteFeedByURL(context.Background(), arg); err != nil {
			return err
		}
		fmt.Printf("Deleted feed with URL: %s\n", arg)
	}
	return nil
}

func runUpdateFeed(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	var id int64
	if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
		return fmt.Errorf("invalid feed ID: %s", args[0])
	}

	var urlPtr, titlePtr *string
	if cmd.Flags().Changed("url") {
		urlPtr = &updateURL
	}
	if cmd.Flags().Changed("title") {
		titlePtr = &updateTitle
	}

	if urlPtr == nil && titlePtr == nil {
		return fmt.Errorf("at least one of --url or --title must be provided")
	}

	if err := store.UpdateFeed(context.Background(), id, urlPtr, titlePtr); err != nil {
		return err
	}
	fmt.Printf("Updated feed ID: %d\n", id)
	return nil
}

func runAddUser(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	id, err := store.AddUser(context.Background(), args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Added user: %s (ID: %d)\n", args[0], id)
	return nil
}

func runSubscribe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	email := args[0]
	feedArg := args[1]

	ctx := context.Background()
	user, err := store.GetUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found: %s", email)
	}

	feedID, err := getFeedID(ctx, store, feedArg)
	if err != nil {
		return err
	}

	if err := store.Subscribe(ctx, user.ID, feedID); err != nil {
		return err
	}
	fmt.Printf("Subscribed %s to feed ID %d\n", email, feedID)
	return nil
}

func runUnsubscribe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	email := args[0]
	feedArg := args[1]

	ctx := context.Background()
	user, err := store.GetUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found: %s", email)
	}

	feedID, err := getFeedID(ctx, store, feedArg)
	if err != nil {
		return err
	}

	if err := store.Unsubscribe(ctx, user.ID, feedID); err != nil {
		return err
	}
	fmt.Printf("Unsubscribed %s from feed ID %d\n", email, feedID)
	return nil
}

func getFeedID(ctx context.Context, store db.Store, arg string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(arg, "%d", &id); err == nil {
		return id, nil
	}

	// Assume it's a URL
	f, err := store.GetFeedByURL(ctx, arg)
	if err != nil {
		return 0, err
	}
	if f == nil {
		return 0, fmt.Errorf("feed not found with URL: %s", arg)
	}
	return f.ID, nil
}

func runListFeeds(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	feeds, err := store.GetFeeds(context.Background())
	if err != nil {
		return err
	}

	fmt.Printf("%-5s | %-30s | %s\n", "ID", "Title", "URL")
	fmt.Println("------------------------------------------------------------")
	for _, f := range feeds {
		fmt.Printf("%-5d | %-30s | %s\n", f.ID, f.Title, f.URL)
	}
	return nil
}

func runTestFeed(cmd *cobra.Command, args []string) error {
	feedURL := args[0]
	email := args[1]

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}

	logger := getLogger(cfg)

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(feedURL)
	if err != nil {
		return fmt.Errorf("failed to parse feed: %w", err)
	}

	if len(feed.Items) == 0 {
		return fmt.Errorf("feed has no items")
	}

	item := feed.Items[0]

	mPool := mailer.NewPool(1, cfg, logger)
	defer mPool.Close()

	// Use a dummy watcher to use its FormatItem logic
	w := watcher.New(models.Feed{}, nil, nil, nil, 0, 0, logger)
	subject, body := w.FormatItem(feed.Title, item)

	fmt.Printf("Sending first item: %s\n", item.Title)

	err = mPool.Send(mailer.MailRequest{
		To:      []string{email},
		Subject: "[TEST] " + subject,
		Body:    body,
	})
	if err != nil {
		return fmt.Errorf("failed to send test email: %w", err)
	}

	fmt.Println("Test email sent successfully!")

	return nil
}

func runListErrors(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	feeds, err := store.GetFeedsWithErrors(context.Background())
	if err != nil {
		return err
	}

	if len(feeds) == 0 {
		fmt.Println("No feeds with errors found.")
		return nil
	}

	fmt.Println("Feeds with errors:")
	fmt.Println("------------------------------------------------------------")
	for _, f := range feeds {
		fmt.Printf("Feed ID: %d\n", f.ID)
		fmt.Printf("Title:   %s\n", f.Title)
		fmt.Printf("URL:     %s\n", f.URL)
		fmt.Printf("Time:    %s\n", f.LastErrorTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("Code:    %d\n", f.LastErrorCode)
		fmt.Printf("Snippet: %s\n", f.LastErrorSnippet)
		fmt.Println("------------------------------------------------------------")
	}

	return nil
}

func runCatchup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	logger := getLogger(cfg)

	store, err := getStore(logger)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	var feeds []models.Feed

	if catchupAll {
		feeds, err = store.GetFeeds(ctx)
		if err != nil {
			return err
		}
	} else {
		if len(args) == 0 {
			return fmt.Errorf("either feed-id or --all must be provided")
		}
		var feedID int64
		if _, err := fmt.Sscanf(args[0], "%d", &feedID); err != nil {
			return fmt.Errorf("invalid feed ID: %s", args[0])
		}
		f, err := store.GetFeed(ctx, feedID)
		if err != nil {
			return err
		}
		if f == nil {
			return fmt.Errorf("feed not found: %d", feedID)
		}
		feeds = append(feeds, *f)
	}

	fp := gofeed.NewParser()
	for i := range feeds {
		f := feeds[i]
		fmt.Printf("Catching up on feed: %s (%s)\n", f.Title, f.URL)
		parsedFeed, err := fp.ParseURL(f.URL)
		if err != nil {
			fmt.Printf("  Failed to parse feed: %v\n", err)
			continue
		}

		markedCount := 0
		for itm := range parsedFeed.Items {
			item := parsedFeed.Items[itm]
			guid := item.GUID
			if guid == "" {
				guid = item.Link
			}

			seen, err := store.IsSeen(ctx, f.ID, guid)
			if err != nil {
				fmt.Printf("  Failed to check if item is seen: %v\n", err)
				continue
			}
			if seen {
				continue
			}

			if err := store.MarkSeen(ctx, f.ID, guid); err != nil {
				fmt.Printf("  Failed to mark item as seen: %v\n", err)
				continue
			}
			markedCount++
		}

		if err := store.UpdateFeedLastPoll(ctx, f.ID); err != nil {
			fmt.Printf("  Failed to update last poll time: %v\n", err)
		}
		// Also clear errors
		if err := store.UpdateFeedError(ctx, f.ID, 0, ""); err != nil {
			fmt.Printf("  Failed to clear feed error: %v\n", err)
		}

		fmt.Printf("  Done. Marked %d new items as seen.\n", markedCount)
	}

	return nil
}
