package crawler

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/hobeone/rss2go/feed_watcher"
)

func TestFeedCrawler(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	ch := make(chan *feedwatcher.FeedCrawlRequest)
	rchan := make(chan *feedwatcher.FeedCrawlResponse)
	go FeedCrawler(ch)

	req := &feedwatcher.FeedCrawlRequest{
		URI:          fmt.Sprintf("%s/%s", ts.URL, "ars.rss"),
		ResponseChan: rchan,
	}

	ch <- req
	resp := <-rchan
	if resp.URI != req.URI {
		t.Fatalf("Response URI differs from request.\n")
	}

	if resp.Error != nil {
		t.Fatalf("Response had an error when it shouldn't have: %s",
			resp.Error.Error())
	}
}

func TestGetFeed(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	resp, err := GetFeed(fmt.Sprintf("%s/%s", ts.URL, "ars.rss"), nil)
	if err != nil {
		t.Fatalf("Error getting feed: %s\n", err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatal("GetFeed should return an error when status != 200\n.")
	}

	resp, err = GetFeed(fmt.Sprintf("%s/%s", ts.URL, "error.rss"), nil)

	if err == nil {
		t.Fatalf("Should have gotten error for feed: %s\n", "error.rss")
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatal("GetFeed should return an error when status != 200\n.")
	}

	dialErrorClient := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				return nil, fmt.Errorf("error connecting to host")
			},
		},
	}

	resp, err = GetFeed(fmt.Sprintf("%s/%s", ts.URL, "timeout"), dialErrorClient)
	if err == nil {
		t.Fatalf("Should have gotten timeout")
	}
}

func TestGetFeedAndMakeResponse(t *testing.T) {
	t.Parallel()
	dialErrorClient := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				return nil, fmt.Errorf("error connecting to host")
			},
		},
	}

	resp := GetFeedAndMakeResponse("http://testfeed", dialErrorClient)

	if resp.Error == nil {
		t.Fatalf("Should have returned an error on connect timeout")
	}

	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	resp = GetFeedAndMakeResponse(fmt.Sprintf("%s/%s", ts.URL, "ars.rss"), nil)
	if resp.Error != nil {
		t.Fatalf("Error getting feed: %s\n", resp.Error.Error())
	}
	bodyWithoutContentLength := string(resp.Body)

	resp = GetFeedAndMakeResponse(fmt.Sprintf("%s/%s", ts.URL, "ars_with_content_length.rss"), nil)
	if resp.Error != nil {
		t.Fatalf("Error getting feed: %s\n", resp.Error)
	}
	bodyWithContentLength := string(resp.Body)
	if bodyWithContentLength != bodyWithoutContentLength {
		t.Fatalf("Responses with and without Content-Length should get the same result")
	}

}

var fakeServerHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var content []byte
	w.Header().Set("Content-Type", "text/html")
	switch {
	case strings.HasSuffix(r.URL.Path, "ars.rss"):
		feedResp, err := ioutil.ReadFile("../testdata/ars.rss")
		if err != nil {
			logrus.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feedResp
	case strings.HasSuffix(r.URL.Path, "ars_with_content_length.rss"):
		feedResp, err := ioutil.ReadFile("../testdata/ars.rss")
		if err != nil {
			logrus.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feedResp
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	case strings.HasSuffix(r.URL.Path, "error.rss"):
		http.Error(w, "Error request", http.StatusInternalServerError)

	case strings.HasSuffix(r.URL.Path, "timeout"):
		time.Sleep(10 * time.Second)

	case true:
		content = []byte("456")
	}
	w.Write([]byte(content))
})
