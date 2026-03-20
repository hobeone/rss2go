package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hobe/rss2go"
	"github.com/hobe/rss2go/internal/config"
	"github.com/hobe/rss2go/internal/crawler"
	"github.com/hobe/rss2go/internal/db/sqlite"
	"github.com/hobe/rss2go/internal/mailer"
	"github.com/hobe/rss2go/internal/metrics"
	"github.com/hobe/rss2go/internal/models"
	"github.com/hobe/rss2go/internal/watcher"
	"github.com/mmcdole/gofeed"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "rss2go",
		Short: "rss2go is an RSS-to-Email daemon",
	}

	daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Start the rss2go daemon",
		RunE:  runDaemon,
	}

	addFeedCmd = &cobra.Command{
		Use:   "add-feed [url] [title]",
		Short: "Add a new RSS feed",
		Args:  cobra.ExactArgs(2),
		RunE:  runAddFeed,
	}

	addUserCmd = &cobra.Command{
		Use:   "add-user [email]",
		Short: "Add a new user",
		Args:  cobra.ExactArgs(1),
		RunE:  runAddUser,
	}

	subscribeCmd = &cobra.Command{
		Use:   "subscribe [email] [feed-id]",
		Short: "Subscribe a user to a feed",
		Args:  cobra.ExactArgs(2),
		RunE:  runSubscribe,
	}

	listFeedsCmd = &cobra.Command{
		Use:   "list-feeds",
		Short: "List all RSS feeds",
		RunE:  runListFeeds,
	}

	testFeedCmd = &cobra.Command{
		Use:   "test-feed [url] [email]",
		Short: "Test a feed by sending its first item to an email",
		Args:  cobra.ExactArgs(2),
		RunE:  runTestFeed,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./rss2go.yaml)")
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(addFeedCmd)
	rootCmd.AddCommand(addUserCmd)
	rootCmd.AddCommand(subscribeCmd)
	rootCmd.AddCommand(listFeedsCmd)
	rootCmd.AddCommand(testFeedCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
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
	var feedID int64
	if _, err := fmt.Sscanf(args[1], "%d", &feedID); err != nil {
		return fmt.Errorf("invalid feed ID: %s", args[1])
	}

	user, err := store.GetUserByEmail(context.Background(), email)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found: %s", email)
	}

	if err := store.Subscribe(context.Background(), user.ID, feedID); err != nil {
		return err
	}
	fmt.Printf("Subscribed %s to feed ID %d\n", email, feedID)
	return nil
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
