package crawler

import (
	"fmt"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/httpclient"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

func GetFeed(url string) (*http.Response, error) {
	log.Printf("Crawling %v", url)

	// Defaults to 1 second for connect and read
	connectTimeout := (5 * time.Second)
	readWriteTimeout := (15 * time.Second)

	httpClient := httpclient.NewTimeoutClient(connectTimeout, readWriteTimeout)
	r, err := httpClient.Get(url)

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

func GetFeedAndMakeResponse(url string) *feed_watcher.FeedCrawlResponse {
	resp := &feed_watcher.FeedCrawlResponse{
		URI: url,
	}
	r, err := GetFeed(url)

	if err != nil {
		resp.Error = err
		return resp
	}
	resp.HttpResponseStatus = r.Status
	defer r.Body.Close()
	if r.ContentLength > 0 {
		b := make([]byte, r.ContentLength)
		_, err := io.ReadFull(r.Body, b)
		if err != nil {
			resp.Error = fmt.Errorf("Error reading response for %s: %s", url, err)
		}
		resp.Body = b
	} else {
		resp.Body, resp.Error = ioutil.ReadAll(r.Body)
		if err != nil {
			resp.Error = fmt.Errorf("Error reading response for %s: %s", url, err)
		}
	}
	return resp
}

func FeedCrawler(crawl_requests chan *feed_watcher.FeedCrawlRequest) {
	for {
		select {
		case req := <-crawl_requests:
			req.ResponseChan <- GetFeedAndMakeResponse(req.URI)
		}
	}
}

func StartCrawlerPool(num int, crawl_channel chan *feed_watcher.FeedCrawlRequest) {
	for i := 0; i < num; i++ {
		go FeedCrawler(crawl_channel)
	}
}
