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
	catchupAll        bool
	updateURL         string
	updateTitle       string
	feedFullArticle   bool
	updateFullArticle bool
	testFullArticle   bool
	testToStdout      bool
)

func init() {
	feedAddCmd.Flags().BoolVar(&feedFullArticle, "full-article", false, "Extract full article content for this feed")
	feedCmd.AddCommand(feedAddCmd)

	feedCmd.AddCommand(feedDelCmd)

	feedUpdateCmd.Flags().StringVar(&updateURL, "url", "", "New URL for the feed")
	feedUpdateCmd.Flags().StringVar(&updateTitle, "title", "", "New title for the feed")
	feedUpdateCmd.Flags().BoolVar(&updateFullArticle, "full-article", false, "Extract full article content for this feed")
	feedCmd.AddCommand(feedUpdateCmd)

	feedCmd.AddCommand(feedListCmd)

	feedTestCmd.Flags().BoolVar(&testFullArticle, "full-article", false, "Test full article extraction")
	feedTestCmd.Flags().BoolVar(&testToStdout, "stdout", false, "Output to stdout instead of mailing")
	feedCmd.AddCommand(feedTestCmd)

	feedCmd.AddCommand(feedErrorsCmd)

	feedCatchupCmd.Flags().BoolVar(&catchupAll, "all", false, "catchup all feeds")
	feedCmd.AddCommand(feedCatchupCmd)

	rootCmd.AddCommand(feedCmd)
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

	id, err := store.AddFeed(context.Background(), args[0], args[1], feedFullArticle)
	if err != nil {
		return err
	}
	fmt.Printf("Added feed: %s (ID: %d, full_article: %v)\n", args[1], id, feedFullArticle)
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
	var fullArticlePtr *bool
	if cmd.Flags().Changed("url") {
		urlPtr = &updateURL
	}
	if cmd.Flags().Changed("title") {
		titlePtr = &updateTitle
	}
	if cmd.Flags().Changed("full-article") {
		fullArticlePtr = &updateFullArticle
	}

	if urlPtr == nil && titlePtr == nil && fullArticlePtr == nil {
		return fmt.Errorf("at least one of --url, --title, or --full-article must be provided")
	}

	if err := store.UpdateFeed(context.Background(), id, urlPtr, titlePtr, fullArticlePtr); err != nil {
		return err
	}
	fmt.Printf("Updated feed ID: %d\n", id)
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

	fmt.Printf("%-5s | %-30s | %-12s | %s\n", "ID", "Title", "FullArticle", "URL")
	fmt.Println("-----------------------------------------------------------------------------")
	for _, f := range feeds {
		fmt.Printf("%-5d | %-30s | %-12v | %s\n", f.ID, f.Title, f.FullArticle, f.URL)
	}
	return nil
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

	var extractedContent string
	if testFullArticle && item.Link != "" {
		fmt.Printf("Extracting full article from: %s\n", item.Link)
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
			extracted, err := extractor.Extract(bytes.NewReader(resp.Body), item.Link, cfg.CrawlerTimeout)
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

	mPool := mailer.NewPool(1, cfg, logger)
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
		if err := store.UpdateFeedError(ctx, f.ID, 0, ""); err != nil {
			fmt.Printf("  Failed to clear feed error: %v\n", err)
		}

		fmt.Printf("  Done. Marked %d new items as seen.\n", markedCount)
	}

	return nil
}
