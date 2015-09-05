package commands

import (
	"time"

	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"github.com/spf13/cobra"
)

// Controlls how many times to poll a feed.
var LoopsToRun int

func MakeCmdRunOne() *cobra.Command {
	cmd := &cobra.Command{
		Run:   runOne,
		Use:   "runone",
		Short: "Crawl a feed and mail new items.",
		Long: `
		Crawls a known feed from the database, finds new items, mails them and then
		exits.

		Example:
		runone --db_updates=false http://test/feed.rss
		`,
	}
	cmd.Flags().IntVarP(&LoopsToRun, "loops", "l", 1, "Number of times to pool this feed. -1 == forever.")

	return cmd
}

func runOne(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		PrintErrorAndExit("No url given to crawl")
	}
	feedURL := args[0]

	cfg := loadConfig(ConfigFile)

	// Override config settings from flags:
	cfg.Mail.SendMail = SendMail
	cfg.Db.UpdateDb = DBUpdates
	dbh := db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	mailer := mail.CreateAndStartMailer(cfg)

	feed, err := dbh.GetFeedByURL(feedURL)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}

	httpCrawlChannel := make(chan *feedwatcher.FeedCrawlRequest, 1)
	responseChannel := make(chan *feedwatcher.FeedCrawlResponse)

	// start crawler pool
	crawler.StartCrawlerPool(1, httpCrawlChannel)

	fw := feedwatcher.NewFeedWatcher(
		*feed,
		httpCrawlChannel,
		responseChannel,
		mailer.OutgoingMail,
		dbh,
		[]string{},
		10,
		100,
	)
	feeds := make(map[string]*feedwatcher.FeedWatcher)
	feeds[fw.FeedInfo.URL] = fw
	if LoopsToRun == -1 {
		for {
			resp := fw.CrawlFeed()
			fw.UpdateFeed(resp)
			time.Sleep(time.Second * time.Duration(cfg.Crawl.MinInterval))
		}
	} else if LoopsToRun == 1 {
		resp := fw.CrawlFeed()
		fw.UpdateFeed(resp)
	} else {
		for i := 0; i < LoopsToRun; i++ {
			resp := fw.CrawlFeed()
			fw.UpdateFeed(resp)
			time.Sleep(time.Second * time.Duration(cfg.Crawl.MinInterval))
		}
	}
}
