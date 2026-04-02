package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/hobeone/rss2go/internal/config"
	"github.com/hobeone/rss2go/internal/crawler"
	"github.com/hobeone/rss2go/internal/extractor"
	"github.com/hobeone/rss2go/internal/mailer"
	"github.com/hobeone/rss2go/internal/models"
	"github.com/hobeone/rss2go/internal/watcher"
	"github.com/mmcdole/gofeed"
	"github.com/spf13/cobra"
)

var (
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
		Short: "Test a feed by sending its first item to an email or stdout",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runTestFeed,
	}

	feedInfoCmd = &cobra.Command{
		Use:   "info [feed-id or url]",
		Short: "Show detailed information about a feed",
		Args:  cobra.ExactArgs(1),
		RunE:  runFeedInfo,
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
)

var (
	catchupAll              bool
	updateURL               string
	updateTitle             string
	feedFullArticle         bool
	feedExtractionStrategy  string
	feedExtractionConfig    string
	updateFullArticle       bool
	updateExtractionStrategy string
	updateExtractionConfig  string
	testFullArticle         bool
	testExtractionStrategy  string
	testExtractionConfig    string
	testToStdout            bool
)

func init() {
	feedAddCmd.Flags().BoolVar(&feedFullArticle, "full-article", false, "Extract full article content for this feed")
	feedAddCmd.Flags().StringVar(&feedExtractionStrategy, "extraction-strategy", "readability", "Extraction strategy when full-article is set (readability, selector)")
	feedAddCmd.Flags().StringVar(&feedExtractionConfig, "extraction-config", "", "Extraction config (e.g. CSS selector for selector strategy)")
	feedCmd.AddCommand(feedAddCmd)

	feedCmd.AddCommand(feedDelCmd)

	feedUpdateCmd.Flags().StringVar(&updateURL, "url", "", "New URL for the feed")
	feedUpdateCmd.Flags().StringVar(&updateTitle, "title", "", "New title for the feed")
	feedUpdateCmd.Flags().BoolVar(&updateFullArticle, "full-article", false, "Extract full article content for this feed")
	feedUpdateCmd.Flags().StringVar(&updateExtractionStrategy, "extraction-strategy", "", "Extraction strategy (readability, selector)")
	feedUpdateCmd.Flags().StringVar(&updateExtractionConfig, "extraction-config", "", "Extraction config (e.g. CSS selector for selector strategy)")
	feedCmd.AddCommand(feedUpdateCmd)

	feedCmd.AddCommand(feedListCmd)

	feedCmd.AddCommand(feedInfoCmd)

	feedTestCmd.Flags().BoolVar(&testFullArticle, "full-article", false, "Fetch and extract the full article (implied by --extraction-strategy/--extraction-config)")
	feedTestCmd.Flags().StringVar(&testExtractionStrategy, "extraction-strategy", "readability", "Extraction strategy: readability or selector (implies --full-article)")
	feedTestCmd.Flags().StringVar(&testExtractionConfig, "extraction-config", "", "Extraction config, e.g. CSS selector for selector strategy (implies --full-article)")
	feedTestCmd.Flags().BoolVar(&testToStdout, "stdout", false, "Output to stdout instead of mailing")
	feedCmd.AddCommand(feedTestCmd)

	feedCmd.AddCommand(feedErrorsCmd)

	feedCatchupCmd.Flags().BoolVar(&catchupAll, "all", false, "catchup all feeds")
	feedCmd.AddCommand(feedCatchupCmd)

	rootCmd.AddCommand(feedCmd)
}

func runAddFeed(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	id, err := store.AddFeed(context.Background(), args[0], args[1], feedFullArticle, feedExtractionStrategy, feedExtractionConfig)
	if err != nil {
		return err
	}
	fmt.Printf("Added feed: %s (ID: %d, full_article: %v, strategy: %s)\n", args[1], id, feedFullArticle, feedExtractionStrategy)
	return nil
}

func runDelFeed(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	arg := args[0]
	var id int64
	if _, err := fmt.Sscanf(arg, "%d", &id); err == nil {
		if err := store.DeleteFeed(context.Background(), id); err != nil {
			return err
		}
		fmt.Printf("Deleted feed with ID: %d\n", id)
	} else {
		if err := store.DeleteFeedByURL(context.Background(), arg); err != nil {
			return err
		}
		fmt.Printf("Deleted feed with URL: %s\n", arg)
	}
	return nil
}

func runUpdateFeed(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	var id int64
	if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
		return fmt.Errorf("invalid feed ID: %s", args[0])
	}

	ctx := context.Background()

	before, err := store.GetFeed(ctx, id)
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("feed not found: %d", id)
	}
	subscribers, err := store.GetUsersForFeed(ctx, before.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch subscribers: %w", err)
	}

	var urlPtr, titlePtr *string
	var fullArticlePtr *bool
	var extractionStrategyPtr, extractionConfigPtr *string
	if cmd.Flags().Changed("url") {
		urlPtr = &updateURL
	}
	if cmd.Flags().Changed("title") {
		titlePtr = &updateTitle
	}
	if cmd.Flags().Changed("full-article") {
		fullArticlePtr = &updateFullArticle
	}
	if cmd.Flags().Changed("extraction-strategy") {
		extractionStrategyPtr = &updateExtractionStrategy
	}
	if cmd.Flags().Changed("extraction-config") {
		extractionConfigPtr = &updateExtractionConfig
	}

	if urlPtr == nil && titlePtr == nil && fullArticlePtr == nil && extractionStrategyPtr == nil && extractionConfigPtr == nil {
		return fmt.Errorf("at least one of --url, --title, --full-article, --extraction-strategy, or --extraction-config must be provided")
	}

	if err := store.UpdateFeed(ctx, id, urlPtr, titlePtr, fullArticlePtr, extractionStrategyPtr, extractionConfigPtr); err != nil {
		return err
	}

	after, err := store.GetFeed(ctx, id)
	if err != nil {
		return err
	}

	fmt.Println("Before:")
	printFeedInfo(before, subscribers)
	fmt.Println("\nAfter:")
	printFeedInfo(after, subscribers)
	return nil
}

func runListFeeds(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	feeds, err := store.GetFeeds(context.Background())
	if err != nil {
		return err
	}

	fmt.Printf("%-5s | %-30s | %-12s | %-12s | %s\n", "ID", "Title", "FullArticle", "Strategy", "URL")
	fmt.Println("---------------------------------------------------------------------------------------------")
	for _, f := range feeds {
		fmt.Printf("%-5d | %-30s | %-12v | %-12s | %s\n", f.ID, f.Title, f.FullArticle, f.ExtractionStrategy, f.URL)
	}
	return nil
}

func runFeedInfo(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	arg := args[0]

	var f *models.Feed
	var id int64
	if _, err := fmt.Sscanf(arg, "%d", &id); err == nil {
		f, err = store.GetFeed(ctx, id)
		if err != nil {
			return err
		}
	} else {
		f, err = store.GetFeedByURL(ctx, arg)
		if err != nil {
			return err
		}
	}

	if f == nil {
		return fmt.Errorf("feed not found: %s", arg)
	}

	subscribers, err := store.GetUsersForFeed(ctx, f.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch subscribers: %w", err)
	}

	printFeedInfo(f, subscribers)
	return nil
}

const feedInfoSep = "─────────────────────────────────────────────"

func printFeedInfo(f *models.Feed, subscribers []models.User) {
	fmt.Println(feedInfoSep)
	fmt.Printf("%-18s %d\n", "ID:", f.ID)
	fmt.Printf("%-18s %s\n", "Title:", f.Title)
	fmt.Printf("%-18s %s\n", "URL:", f.URL)
	fmt.Println(feedInfoSep)

	fmt.Printf("%-18s %v\n", "Full Article:", f.FullArticle)
	fmt.Printf("%-18s %s\n", "Strategy:", f.ExtractionStrategy)
	if f.ExtractionConfig != "" {
		fmt.Printf("%-18s %s\n", "Extraction Config:", f.ExtractionConfig)
	}
	fmt.Println(feedInfoSep)

	if f.LastPoll.IsZero() {
		fmt.Printf("%-18s never\n", "Last Poll:")
	} else {
		fmt.Printf("%-18s %s\n", "Last Poll:", f.LastPoll.Format("2006-01-02 15:04:05 MST"))
	}
	if f.ETag != "" {
		fmt.Printf("%-18s %s\n", "ETag:", f.ETag)
	}
	if f.LastModified != "" {
		fmt.Printf("%-18s %s\n", "Last-Modified:", f.LastModified)
	}
	if !f.BackoffUntil.IsZero() {
		fmt.Printf("%-18s %s\n", "Backoff Until:", f.BackoffUntil.Format("2006-01-02 15:04:05 MST"))
	}

	if f.LastErrorCode != 0 || !f.LastErrorTime.IsZero() {
		fmt.Println(feedInfoSep)
		fmt.Printf("%-18s %d\n", "Last Error Code:", f.LastErrorCode)
		fmt.Printf("%-18s %s\n", "Last Error Time:", f.LastErrorTime.Format("2006-01-02 15:04:05 MST"))
		if f.LastErrorSnippet != "" {
			fmt.Printf("%-18s %s\n", "Last Error:", f.LastErrorSnippet)
		}
	}

	fmt.Println(feedInfoSep)
	if len(subscribers) == 0 {
		fmt.Printf("%-18s none\n", "Subscribers:")
	} else {
		fmt.Printf("Subscribers (%d):\n", len(subscribers))
		for _, u := range subscribers {
			fmt.Printf("  - %s (ID: %d)\n", u.Email, u.ID)
		}
	}
	fmt.Println(feedInfoSep)
}

func runTestFeed(cmd *cobra.Command, args []string) error {
	feedURL := args[0]
	var email string
	if len(args) > 1 {
		email = args[1]
	}

	if email == "" && !testToStdout {
		return fmt.Errorf("email is required when not using --stdout")
	}

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

	// Providing --extraction-strategy or --extraction-config implies --full-article.
	doExtraction := testFullArticle ||
		cmd.Flags().Changed("extraction-strategy") ||
		cmd.Flags().Changed("extraction-config")

	var extractedContent string
	if doExtraction && item.Link != "" {
		ext, err := extractor.New(testExtractionStrategy, testExtractionConfig)
		if err != nil {
			return fmt.Errorf("invalid extraction config: %w", err)
		}

		fmt.Printf("Extracting full article from: %s (strategy: %s)\n", item.Link, testExtractionStrategy)
		cPool := crawler.NewPool(1, cfg.CrawlerTimeout, logger)

		reqCtx, cancel := context.WithTimeout(context.Background(), cfg.CrawlerTimeout)
		defer cancel()

		cPool.Submit(crawler.CrawlRequest{
			URL:  item.Link,
			Type: crawler.RequestTypeItem,
			Ctx:  reqCtx,
		})

		select {
		case resp := <-cPool.Responses():
			if resp.Error != nil {
				return fmt.Errorf("failed to fetch full article: %w", resp.Error)
			}
			extracted, err := ext.Extract(bytes.NewReader(resp.Body), item.Link, cfg.CrawlerTimeout, logger)
			if err != nil {
				return fmt.Errorf("failed to extract full article: %w", err)
			}
			extractedContent = extracted
			fmt.Println("Full article extracted successfully.")
		case <-reqCtx.Done():
			return fmt.Errorf("full article extraction timed out")
		}
		cPool.Close()
	}

	w := watcher.New(models.Feed{}, nil, nil, nil, 0, 0, cfg.MaxImageWidth, logger)
	subject, body := w.FormatItem(feed.Title, item, extractedContent)

	if testToStdout {
		fmt.Printf("\nSubject: %s\n", subject)
		fmt.Printf("Body:\n%s\n", body)
		return nil
	}

	mPool := mailer.NewPool(1, cfg, nil, logger)
	defer mPool.Close()

	fmt.Printf("Sending item: %s to %s\n", item.Title, email)

	if err := mPool.Send(mailer.MailRequest{
		To:      []string{email},
		Subject: "[TEST] " + subject,
		Body:    body,
	}); err != nil {
		return fmt.Errorf("failed to send test email: %w", err)
	}

	fmt.Println("Test email sent successfully!")
	return nil
}

func runListErrors(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
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
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	var feeds []models.Feed

	if catchupAll {
		var err error
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
		for _, item := range parsedFeed.Items {
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

		if err := store.UpdateFeedLastPoll(ctx, f.ID, f.ETag, f.LastModified); err != nil {
			fmt.Printf("  Failed to update last poll time: %v\n", err)
		}
		if err := store.ClearFeedError(ctx, f.ID); err != nil {
			fmt.Printf("  Failed to clear feed error: %v\n", err)
		}

		fmt.Printf("  Done. Marked %d new items as seen.\n", markedCount)
	}

	return nil
}
