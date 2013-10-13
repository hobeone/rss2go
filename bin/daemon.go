package main

import (
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/server"
	"log"
	"net/http"
)

func make_cmd_daemon() *commander.Command {
	cmdDaemon := &commander.Command{
		Run:       daemon,
		UsageLine: "daemon",
		Short:     "Start a daemon to collect feeds and mail items.",
		Long: `
		Starts up as a daemon and will watch feeds and send new items to the configured
		mail address.
		`,
		Flag: *flag.NewFlagSet("daemon", flag.ExitOnError),
	}
	cmdDaemon.Flag.Bool("send_mail", true, "Actually send mail or not.")
	cmdDaemon.Flag.Bool("db_updates", true, "Don't actually update feed info in the db.")

	return cmdDaemon
}

func daemon(cmd *commander.Command, args []string) {
	send_mail := cmd.Flag.Lookup("send_mail").Value.Get().(bool)
	update_db := cmd.Flag.Lookup("db_updates").Value.Get().(bool)

	cfg := loadConfig(g_cmd.Flag.Lookup("config_file").Value.Get().(string))

	// Override config settings from flags:
	cfg.Mail.SendMail = send_mail
	cfg.Db.UpdateDb = update_db

	mailer := mail.CreateAndStartMailer(cfg)

	db := db.NewDbDispatcher(cfg.Db.Path, true, update_db)

	all_feeds, err := db.GetAllFeeds()
	if err != nil {
		log.Fatal(err.Error())
	}
	feeds, response_channel := CreateAndStartFeedWatchers(
		all_feeds, cfg, mailer, db)

	log.Printf("Got %d feeds to watch.\n", len(all_feeds))

	go server.StartHttpServer(cfg, feeds)
	for {
		http.DefaultTransport.(*http.Transport).CloseIdleConnections()
		_ = <-response_channel
	}
}

func startPollers(
	all_feeds []db.FeedInfo,
	http_crawl_channel chan *feed_watcher.FeedCrawlRequest,
	response_channel chan *feed_watcher.FeedCrawlResponse,
	mail_chan chan *mail.MailRequest,
	db db.FeedDbDispatcher,
	cfg *config.Config,
) map[string]*feed_watcher.FeedWatcher {
	// make feeds unique
	feeds := make(map[string]*feed_watcher.FeedWatcher)
	for _, f := range all_feeds {
		if _, ok := feeds[f.Url]; ok {
			log.Printf("Found duplicate feed: %s", f.Url)
			continue
		}

		feeds[f.Url] = feed_watcher.NewFeedWatcher(
			f,
			http_crawl_channel,
			response_channel,
			mail_chan,
			db,
			[]string{},
			cfg.Crawl.MinInterval,
			cfg.Crawl.MaxInterval,
		)
		go feeds[f.Url].PollFeed()
	}
	return feeds
}

func CreateAndStartFeedWatchers(
	feeds []db.FeedInfo,
	cfg *config.Config,
	mailer *mail.MailDispatcher,
	db *db.DbDispatcher,
) (map[string]*feed_watcher.FeedWatcher,
	chan *feed_watcher.FeedCrawlResponse) {
	// Start Crawlers
	// Http pool channel
	http_crawl_channel := make(chan *feed_watcher.FeedCrawlRequest)
	response_channel := make(chan *feed_watcher.FeedCrawlResponse)

	// start crawler pool
	crawler.StartCrawlerPool(
		cfg.Crawl.MaxCrawlers,
		http_crawl_channel)

	// Start Polling
	return startPollers(
		feeds,
		http_crawl_channel,
		response_channel,
		mailer.OutgoingMail,
		db,
		cfg,
	), response_channel
}
