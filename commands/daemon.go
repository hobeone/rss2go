package commands

import (
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/flagutil"
	"flag"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/server"
	"github.com/golang/glog"
	"net/http"
)

func MakeCmdDaemon() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       daemon,
		UsageLine: "daemon",
		Short:     "Start a daemon to collect feeds and mail items.",
		Long: `
		Starts up as a daemon and will watch feeds and send new items to the configured
		mail address.
		`,
		Flag: *flag.NewFlagSet("daemon", flag.ExitOnError),
	}
	cmd.Flag.Bool("send_mail", true, "Actually send mail or not.")
	cmd.Flag.Bool("db_updates", true, "Don't actually update feed info in the db.")

	cmd.Flag.String("config_file", default_config, "Config file to use.")
	return cmd
}

func daemon(cmd *flagutil.Command, args []string) {
	send_mail := cmd.Flag.Lookup("send_mail").Value.(flag.Getter).Get().(bool)
	update_db := cmd.Flag.Lookup("db_updates").Value.(flag.Getter).Get().(bool)

	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	// Override config settings from flags:
	cfg.Mail.SendMail = send_mail
	cfg.Db.UpdateDb = update_db

	mailer := mail.CreateAndStartMailer(cfg)

	db := db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	all_feeds, err := db.GetAllFeeds()

	if err != nil {
		glog.Fatal(err.Error())
	}
	feeds, response_channel := CreateAndStartFeedWatchers(
		all_feeds, cfg, mailer, db)

	glog.Infof("Got %d feeds to watch.\n", len(all_feeds))

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
			glog.Infof("Found duplicate feed: %s", f.Url)
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
