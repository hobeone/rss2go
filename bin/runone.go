package main

import (
	"flag"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/flagutil"
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/server"
	"net/http"
	"time"
)

func make_cmd_runone() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       runOne,
		UsageLine: "runone",
		Short:     "Crawl a feed and mail new items.",
		Long: `
		Crawls a known feed from the database, finds new items, mails them and then
		exits.

		Example:
		runone --db_updates=false http://test/feed.rss
		`,
		Flag: *flag.NewFlagSet("runone", flag.ExitOnError),
	}
	cmd.Flag.Bool("send_mail", true, "Actually send mail or not.")
	cmd.Flag.Bool("db_updates", true, "Don't actually update feed info in the db.")
	cmd.Flag.String("config_file", "", "Config file to use.")
	cmd.Flag.Int("loops", 1, "Number of times to pool this feed. -1 == forever.")

	return cmd
}

func runOne(cmd *flagutil.Command, args []string) {
	if len(args) < 1 {
		printErrorAndExit("No url given to crawl")
	}
	feed_url := args[0]

	send_mail := cmd.Flag.Lookup("send_mail").Value.(flag.Getter).Get().(bool)
	update_db := cmd.Flag.Lookup("db_updates").Value.(flag.Getter).Get().(bool)
	loops := cmd.Flag.Lookup("loops").Value.(flag.Getter).Get().(int)

	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	// Override config settings from flags:
	cfg.Mail.SendMail = send_mail
	cfg.Db.UpdateDb = update_db

	mailer := mail.CreateAndStartMailer(cfg)

	db := db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

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
	feeds := make(map[string]*feed_watcher.FeedWatcher)
	feeds[fw.FeedInfo.Url] = fw
	go server.StartHttpServer(cfg, feeds)
	if loops == -1 {
		for {
			http.DefaultTransport.(*http.Transport).CloseIdleConnections()
			fw.UpdateFeed()
			time.Sleep(time.Second * time.Duration(cfg.Crawl.MinInterval))
		}
	} else {
		for i := 0; i < loops; i++ {
			http.DefaultTransport.(*http.Transport).CloseIdleConnections()
			fw.UpdateFeed()
			time.Sleep(time.Second * time.Duration(cfg.Crawl.MinInterval))
		}
	}
}
