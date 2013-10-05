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
	"time"
	"net/http"
)

func make_cmd_runone() *commander.Command {
	cmdDaemon := &commander.Command{
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
	cmdDaemon.Flag.Bool("send_mail", true, "Actually send mail or not.")
	cmdDaemon.Flag.Bool("db_updates", true, "Don't actually update feed info in the db.")
	cmdDaemon.Flag.String("config_file", "", "Config file to use.")
	cmdDaemon.Flag.Int("loops", 1, "Number of times to pool this feed. -1 == forever.")

	return cmdDaemon
}

func runOne(cmd *commander.Command, args []string) {
	if len(args) < 1 {
		printErrorAndExit("No url given to crawl")
	}
	feed_url := args[0]

	send_mail := cmd.Flag.Lookup("send_mail").Value.Get().(bool)
	update_db := cmd.Flag.Lookup("db_updates").Value.Get().(bool)
	ConfigFile := cmd.Flag.Lookup("config_file").Value.Get().(string)
	loops := cmd.Flag.Lookup("loops").Value.Get().(int)

	if len(ConfigFile) == 0 {
		log.Printf("No --config_file given.  Using default: %s\n", DEFAULT_CONFIG)
		ConfigFile = DEFAULT_CONFIG
	}

	log.Printf("Got config file: %s\n", ConfigFile)
	config := config.NewConfig()
	err := config.ReadConfig(ConfigFile)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Config contents: %#v\n", &config)

	// Override config settings from flags:
	config.Mail.SendMail = send_mail
	config.Db.UpdateDb = update_db

	mailer := mail.CreateAndStartMailer(config)

	db := db.NewDbDispatcher(config.Db.Path, true, update_db)

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
	go server.StartHttpServer(config, feeds)
	if loops == -1 {
		for {
			http.DefaultTransport.(*http.Transport).CloseIdleConnections()
			fw.UpdateFeed()
			time.Sleep(time.Second * time.Duration(config.Crawl.MinInterval))
		}
	} else {
		for i := 0; i < loops; i++ {
			http.DefaultTransport.(*http.Transport).CloseIdleConnections()
			fw.UpdateFeed()
			time.Sleep(time.Second * time.Duration(config.Crawl.MinInterval))
		}
	}
}
