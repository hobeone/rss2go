package main

import (
	"fmt"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/feed_watcher"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"log"
)

func MakeDbFixtures(d DbDispatcher, local_url string) {

	d.OrmHandle.Exec("delete from feed_info;")

	all_feeds := []FeedInfo{
		{
			Name: "Testing1",
			Url:  local_url + "/test.rss",
		},
	}

	for _, f := range all_feeds {
		d.OrmHandle.Save(&f)
	}
}

var fake_server_handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var content []byte
	switch {
	case strings.HasSuffix(r.URL.Path, "test.rss"):

		feed_resp, err := ioutil.ReadFile("testdata/ars.rss")
		if err != nil {
			log.Fatalf("Error reading test feed: %s", err.Error())
		}
		fmt.Println("Handling test.rss")
		content = feed_resp
	case true:
		content = []byte("456")
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
})

func main() {
	ts := httptest.NewServer(fake_server_handler)
	defer ts.Close()

  crawler.Sleep = func(d time.Duration) {
		fmt.Println("mock sleep")
		time.Sleep(time.Second * time.Duration(10))
		return
	}

	db := NewDbDispatcher("integration.db")
	MakeDbFixtures(db, ts.URL)
	all_feeds, err := db.GetAllFeeds()
	fmt.Printf("Got %d feeds to watch.\n", len(all_feeds))

	if err != nil {
		log.Fatalf("Error reading feeds: %s", err.Error())
	}

	http_crawl_channel := make(chan *feed_watcher.FeedCrawlRequest)
	response_channel := make(chan *feed_watcher.FeedCrawlResponse)
	// poll now chanel and exit now chanel are per feed.

	// start crawler pool
	crawler.StartCrawlerPool(1, http_crawl_channel)
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
	fmt.Println("got here")
	for resp := range response_channel {
		if resp.Error != nil {
			fmt.Printf("Error getting feed: %s", resp.Error.Error())
			continue
		}
		db.UpdateFeedTimesByUrl(resp.URI, resp.CrawlTime, resp.LastItemTime)
		fmt.Printf("Got response from %v", resp.URI)
	}
}
