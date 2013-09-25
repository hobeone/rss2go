package rss2go

import (
	"flag"
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/crawler"
	"log"
	"net/http"
	"time"
)

// get flags
// get config
// - smtp or sendmail
// - max and min scan interval
// setup http interface
// setup scanners
//
func startHttpServer(addr string) {
	log.Printf("Starting http server on %v", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

const DEFAULT_CONFIG = "~/.config/rssgomail/config.toml"

func startPollers(all_feeds []db.FeedInfo,
	http_crawl_channel chan *feed_watcher.FeedCrawlRequest,
	response_channel chan *feed_watcher.FeedCrawlResponse) map[string]*feed_watcher.FeedWatcher {
	// make feeds unique
	feeds := make(map[string]*feed_watcher.FeedWatcher)
	for _, f := range all_feeds {
		if _, ok := feeds[f.Url]; ok {
			fmt.Printf("Found duplicate feed: %s", f.Url)
			continue
		}

		feeds[f.Url] = feed_watcher.NewFeedWatcher(
			f.Url, http_crawl_channel, response_channel, *new(time.Time))
		go feeds[f.Url].PollFeed()
	}
	return feeds
}

func RunCollector() {
	var config_file = flag.String("config_file", "", "Config file to use")

	fmt.Println("Parsing flags...")
	flag.Parse()

	if len(*config_file) == 0 {
		fmt.Printf("No config file given.  Using default: %s\n", DEFAULT_CONFIG)
		*config_file = DEFAULT_CONFIG
	}

	fmt.Printf("Got config file: %s\n", *config_file)
	config := config.NewConfig()
	err := config.ReadConfig(*config_file)
	if err != nil {
		log.Fatal(err)
	}

	db := db.NewDbDispatcher(config.Db.Path)

	all_feeds, err := db.GetAllFeeds()

	fmt.Printf("Got %d feeds to watch.\n", len(all_feeds))

	// Start Crawlers
	//
	// channels needed for comms:
	//
	// Http pool channel
	http_crawl_channel := make(chan *feed_watcher.FeedCrawlRequest)
	response_channel := make(chan *feed_watcher.FeedCrawlResponse)
	// poll now chanel and exit now chanel are per feed.

	// start crawler pool
	crawler.StartCrawlerPool(config.Crawl.MaxCrawlers, http_crawl_channel)

	// Start Polling
	feeds := startPollers(all_feeds, http_crawl_channel, response_channel)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello: I have %v feeds.\n", len(feeds))
		for uri, f := range feeds {
			fmt.Fprintf(w, "Feed %v polling? %v. crawling? %v\n", uri, f.Polling(),
				f.Crawling())
		}
	})

	go startHttpServer("localhost:7000")

	for resp := range response_channel {
		log.Printf("Got response from %v", resp.URI)
	}
}
