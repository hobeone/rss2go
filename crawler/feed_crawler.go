package crawler

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	feedwatcher "github.com/hobeone/rss2go/feed_watcher"
	"github.com/sirupsen/logrus"
)

const (
	//Timeouts for client connections
	defaultTimeout             = 10 * time.Second
	defaultConnectTimeout      = 5 * time.Second // Timeout for establishing the connection
	defaultTLSHandshakeTimeout = 5 * time.Second // Timeout for the TLS handshake
	defaultKeepAlive           = 15 * time.Second

	UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/73.0.3683.86 Safari/537.36"
	Accept    = "application/rss+xml, application/rdf+xml;q=0.8, application/atom+xml;q=0.6, application/xml;q=0.4, text/xml;q=0.4"
)

// HTTPClient is a wrapper around net/http.Client with improved defaults and error handling.
type HTTPClient struct {
	http.Client
}

// NewHTTPClient creates a new HTTPClient with sensible timeouts and keep-alive settings.
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   defaultConnectTimeout,
			KeepAlive: defaultKeepAlive,
		}).DialContext,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
		MaxIdleConns:          5, // Adjust as needed for connection pooling
		IdleConnTimeout:       20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &HTTPClient{
		http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// GetFeed gets a URL and returns a http.Response.
// Sets a reasonable timeout on the connection and read from the server.
// Users will need to Close() the resposne.Body or risk leaking connections.
func GetFeed(url string, client *HTTPClient) (*http.Response, error) {
	logrus.Infof("Crawling %v", url)

	if client == nil {
		client = NewHTTPClient(defaultTimeout)
	}

	ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)
	//defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)

	if err != nil {
		logrus.Errorf("Error creating request: %v", err)
		return nil, err
	}
	req.Header.Set("Accept", Accept)
	req.Header.Set("User-Agent", UserAgent)

	requestDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		logrus.Errorf("Couldn't dump request: %s", err)
	} else {
		logrus.Debugln(string(requestDump))
	}

	r, err := client.Do(req)

	if err != nil {
		logrus.Infof("Error getting %s: %s", url, err)
		return r, err
	}
	if r.StatusCode != http.StatusOK {
		err = fmt.Errorf("feed %s returned a non 200 status code: %s", url, r.Status)
		logrus.Info(err)
		return r, err
	}
	return r, nil
}

// GetFeedAndMakeResponse gets a URL and returns a FeedCrawlResponse
// Sets FeedCrawlResponse.Error if there was a problem retreiving the URL.
func GetFeedAndMakeResponse(url string, client *HTTPClient) *feedwatcher.FeedCrawlResponse {
	resp := &feedwatcher.FeedCrawlResponse{
		URI: url,
	}
	logrus.Infof("Crawling %v", url)

	if client == nil {
		client = NewHTTPClient(defaultTimeout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)

	if err != nil {
		logrus.Errorf("Error creating request: %v", err)
		resp.Error = err
		return resp
	}
	req.Header.Set("Accept", Accept)
	req.Header.Set("User-Agent", UserAgent)

	requestDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		logrus.Errorf("Couldn't dump request: %s", err)
	} else {
		logrus.Debugln(string(requestDump))
	}

	r, err := client.Do(req)

	if err != nil {
		logrus.Infof("Error getting %s: %s", url, err)
		resp.Error = err
		return resp
	}
	resp.HTTPResponseStatus = r.Status
	resp.HTTPResponseStatusCode = r.StatusCode

	if r.StatusCode != http.StatusOK {
		err = fmt.Errorf("feed %s returned a non 200 status code: %s", url, r.Status)
		logrus.Info(err)
		resp.Error = err
		return resp
	}

	if r != nil {
		// If there are connection issues the response will be nil
		defer r.Body.Close()
	}

	if err != nil {
		resp.Error = err
		return resp
	}

	resp.HTTPResponseStatus = r.Status
	resp.HTTPResponseStatusCode = r.StatusCode
	if r.ContentLength > 0 {
		b := make([]byte, r.ContentLength)
		_, err := io.ReadFull(r.Body, b)
		if err != nil {
			resp.Error = fmt.Errorf("error reading response for %s: %s", url, err)
		}
		resp.Body = b
	} else {
		resp.Body, err = io.ReadAll(r.Body)
		if err != nil {
			resp.Error = fmt.Errorf("error reading response for %s: %s", url, err)
		}
	}

	return resp
}

// FeedCrawler pulls FeedCrawlRequests from the crawl_requests channel,
// gets the given URL and returns a response
func FeedCrawler(crawlRequests chan *feedwatcher.FeedCrawlRequest, client *HTTPClient) {
	for {
		logrus.Info("Waiting on request")
		req := <-crawlRequests
		req.ResponseChan <- GetFeedAndMakeResponse(req.URI, client)
	}
}

// StartCrawlerPool creates a pool of num http crawlers listening to the crawl_channel.
func StartCrawlerPool(num int, crawlChannel chan *feedwatcher.FeedCrawlRequest) {
	httpClient := NewHTTPClient(defaultTimeout)
	for i := 0; i < num; i++ {
		logrus.Infof("Starting Crawler %d", i)
		go FeedCrawler(crawlChannel, httpClient)
	}
}
