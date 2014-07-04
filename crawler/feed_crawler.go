package crawler

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/httpclient"
)

// GetFeed gets a URL and returns a http.Response.
// Sets a reasonable timeout on the connection and read from the server.
// Users will need to Close() the resposne.Body or risk leaking connections.
func GetFeed(url string, client *http.Client) (*http.Response, error) {
	glog.Infof("Crawling %v", url)

	// Defaults to 1 second for connect and read
	connectTimeout := (5 * time.Second)
	readWriteTimeout := (15 * time.Second)

	if client == nil {
		client = httpclient.NewTimeoutClient(connectTimeout, readWriteTimeout)
	}

	r, err := client.Get(url)

	if err != nil {
		glog.Infof("Error getting %s: %s", url, err)
		return r, err
	}
	if r.StatusCode != http.StatusOK {
		err = fmt.Errorf("feed %s returned a non 200 status code: %s", url, r.Status)
		glog.Info(err)
		return r, err
	}
	return r, nil
}

// GetFeedAndMakeResponse gets a URL and returns a FeedCrawlResponse
// Sets FeedCrawlResponse.Error if there was a problem retreiving the URL.
func GetFeedAndMakeResponse(url string, client *http.Client) *feed_watcher.FeedCrawlResponse {
	resp := &feed_watcher.FeedCrawlResponse{
		URI: url,
	}
	r, err := GetFeed(url, client)
	if r != nil {
		// If there are connection issues the response will be nil
		defer r.Body.Close()
	}
	if err != nil {
		resp.Error = err
		return resp
	}

	resp.HTTPResponseStatus = r.Status
	if r.ContentLength > 0 {
		b := make([]byte, r.ContentLength)
		_, err := io.ReadFull(r.Body, b)
		if err != nil {
			resp.Error = fmt.Errorf("error reading response for %s: %s", url, err)
		}
		resp.Body = b
	} else {
		resp.Body, resp.Error = ioutil.ReadAll(r.Body)
		if err != nil {
			resp.Error = fmt.Errorf("error reading response for %s: %s", url, err)
		}
	}
	return resp
}

// FeedCrawler pulls FeedCrawlRequests from the crawl_requests channel,
// gets the given URL and returns a response
func FeedCrawler(crawlRequests chan *feed_watcher.FeedCrawlRequest) {
	for {
		glog.Info("Waiting on request")
		select {
		case req := <-crawlRequests:
			req.ResponseChan <- GetFeedAndMakeResponse(req.URI, nil)
		}
	}
}

// StartCrawlerPool creates a pool of num http crawlers listening to the crawl_channel.
func StartCrawlerPool(num int, crawlChannel chan *feed_watcher.FeedCrawlRequest) {
	for i := 0; i < num; i++ {
		glog.Infof("Starting Crawler %d", i)
		go FeedCrawler(crawlChannel)
	}
}
