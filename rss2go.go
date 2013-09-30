package rss2go

import (
	"flag"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/server"
	"log"
)


const DEFAULT_CONFIG = "~/.config/rssgomail/config.toml"

func startPollers(
	all_feeds []db.FeedInfo,
	http_crawl_channel chan *feed_watcher.FeedCrawlRequest,
	response_channel chan *feed_watcher.FeedCrawlResponse,
	mail_chan chan *mail.MailRequest,
	db db.FeedDbDispatcher,
	config *config.Config,
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
			config.Crawl.MinInterval,
			config.Crawl.MaxInterval,
		)
		go feeds[f.Url].PollFeed()
	}
	return feeds
}

func CreateAndStartFeedWatchers(
	feeds []db.FeedInfo,
	config *config.Config,
	mailer *mail.MailDispatcher,
	db *db.DbDispatcher,
) (map[string]*feed_watcher.FeedWatcher,
	chan *feed_watcher.FeedCrawlResponse){
	// Start Crawlers
	// Http pool channel
	http_crawl_channel := make(chan *feed_watcher.FeedCrawlRequest)
	response_channel := make(chan *feed_watcher.FeedCrawlResponse)

	// start crawler pool
	crawler.StartCrawlerPool(
		config.Crawl.MaxCrawlers,
		http_crawl_channel)

	// Start Polling
	return startPollers(
		feeds,
		http_crawl_channel,
		response_channel,
		mailer.OutgoingMail,
		db,
		config,
	), response_channel
}

func RunCollector() {
	var config_file = flag.String("config_file", "", "Config file to use.")
	var send_mail = flag.Bool("send_mail", true, "Actually send mail or not.")
	var update_db = flag.Bool("db_updates", true, "Don't actually update feed info in the db.")

	log.Println("Parsing flags...")
	flag.Parse()

	if len(*config_file) == 0 {
		log.Printf("No --config_file given.  Using default: %s\n", DEFAULT_CONFIG)
		*config_file = DEFAULT_CONFIG
	}

	log.Printf("Got config file: %s\n", *config_file)
	config := config.NewConfig()
	err := config.ReadConfig(*config_file)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Config contents: %#v\n", &config)

	// Override config settings from flags:
	config.Mail.SendMail = *send_mail
	config.Db.UpdateDb = *update_db

	mailer := mail.CreateAndStartMailer(config)

	db := db.NewDbDispatcher(config.Db.Path, true, *update_db)

	all_feeds, err := db.GetAllFeeds()
	if err != nil {
		log.Fatal(err.Error())
	}
	feeds, response_channel := CreateAndStartFeedWatchers(
		all_feeds, config, mailer, db)

	log.Printf("Got %d feeds to watch.\n", len(all_feeds))

	go server.StartHttpServer(config, feeds)

	//TODO: figure out if we still need a response_channel
	for {
		_ = <-response_channel
	}
}
