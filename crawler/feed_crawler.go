package crawler

import (
	"fmt"
	"github.com/hobeone/rss2go/feed_watcher"
	"io/ioutil"
	"log"
	"net/http"
)

func GetFeed(url string) (*http.Response, error) {
	log.Printf("Crawling %v", url)
	r, err := http.Get(url)
	if err != nil {
		log.Printf("Error getting %s: %s", url, err)
		return r, err
	}
	if r.StatusCode != http.StatusOK {
		err = fmt.Errorf("Feed %s returned a non 200 status code: %s", url, r.Status)
		log.Print(err)
		return r, err
	}
	return r, nil
}

func FeedCrawler(crawl_requests chan *feed_watcher.FeedCrawlRequest) {
	for {
		select {
		case req := <-crawl_requests:
			resp := &feed_watcher.FeedCrawlResponse{}
			resp.URI = req.URI
			r, err := GetFeed(req.URI)
			if err != nil {
				resp.Body = make([]byte, 0)
				resp.Error = err
				req.ResponseChan <- resp
				continue
			}
			resp.Body, resp.Error = ioutil.ReadAll(r.Body)
			r.Body.Close()
			resp.HttpResponseStatus = r.Status
			req.ResponseChan <- resp
		}
	}
}

func StartCrawlerPool(num int, crawl_channel chan *feed_watcher.FeedCrawlRequest) {
	for i := 0; i < num; i++ {
		go FeedCrawler(crawl_channel)
	}
}
