package main

import (
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/flagutil"
	"flag"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"fmt"
)

func make_cmd_addfeed() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       addFeed,
		UsageLine: "addfeed FeedName FeedUrl",
		Short:     "Add a feed to the database.",
		Long: `
		Add a feed URL to the database and optionally poll it and add existing
		items to the list of known items.

		Example:
		addfeed --poll_feed TestFeed http://test/feed.rss
		`,
		Flag: *flag.NewFlagSet("addfeed", flag.ExitOnError),
	}
	cmd.Flag.Bool("poll_feed", false, "Get the current feed contents and add them to the database.")
	cmd.Flag.String("config_file", default_config, "Config file to use.")

	return cmd
}

func addFeed(cmd *flagutil.Command, args []string) {
	if len(args) < 2 {
		printErrorAndExit("Must supply feed name and url")
	}
	feed_name := args[0]
	feed_url := args[1]

	poll_feed := cmd.Flag.Lookup("poll_feed").Value.(flag.Getter).Get().(bool)

	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	// Override config settings
	cfg.Mail.SendMail = false
	cfg.Db.UpdateDb = true

	mailer := mail.CreateAndStartMailer(cfg)

	db := db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	_, err := db.AddFeed(feed_name, feed_url)
	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Printf("Added feed %s at url %s\n", feed_name, feed_url)

	if poll_feed {
		feed, err := db.GetFeedByUrl(feed_url)
		if err != nil {
			printErrorAndExit(err.Error())
		}

		http_crawl_channel := make(chan *feed_watcher.FeedCrawlRequest)
		response_channel := make(chan *feed_watcher.FeedCrawlResponse)

		// start crawler pool
		crawler.StartCrawlerPool(1, http_crawl_channel)

		fw := feed_watcher.NewFeedWatcher(
			*feed,
			http_crawl_channel,
			response_channel,
			mailer.OutgoingMail,
			db,
			[]string{},
			10,
			100,
		)

		fw.UpdateFeed()
	}
}
