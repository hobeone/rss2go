package crawler

import (
	"fmt"
	"github.com/hobeone/rss2go/feed_watcher"
	"io/ioutil"
	"log"
	"net/http"
)

func FeedCrawler(crawl_requests chan *feed_watcher.FeedCrawlRequest) {
	for {
		select {
		case req := <-crawl_requests:
			resp := &feed_watcher.FeedCrawlResponse{}
			resp.URI = req.URI
			log.Printf("Crawling %v", req.URI)
			r, err := http.Get(req.URI)
			if err != nil {
				resp.Body = make([]byte, 0)
				resp.Error = err
				log.Printf("Error getting %v: %v", req.URI, err)
				req.ResponseChan <- resp
				continue
			}
			defer r.Body.Close()
			if r.StatusCode != http.StatusOK {
				resp.Body = make([]byte, 0)
				resp.Error = fmt.Errorf("Feed %v returned a non 200 status code: %v",
					req.URI, r.Status)
				log.Printf("Error getting %v: %v", req.URI, resp.Error)
				req.ResponseChan <- resp
				continue
			}
			resp.Body, resp.Error = ioutil.ReadAll(r.Body)
			resp.HttpResponseStatus = r.Status
			log.Printf("Crawled %v: %v", req.URI, r.Status)
			req.ResponseChan <- resp
		}
	}
}

func StartCrawlerPool(num int, crawl_channel chan *feed_watcher.FeedCrawlRequest) {
	for i := 0; i < num; i++ {
		go FeedCrawler(crawl_channel)
	}
}
