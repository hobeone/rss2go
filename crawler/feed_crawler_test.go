package crawler

import (
	"fmt"
	"github.com/hobeone/rss2go/feed_watcher"
	"io/ioutil"
	"github.com/golang/glog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFeedCrawler(t *testing.T) {
	ts := httptest.NewServer(fake_server_handler)
	defer ts.Close()

	ch := make(chan *feed_watcher.FeedCrawlRequest)
	rchan := make(chan *feed_watcher.FeedCrawlResponse)
	go FeedCrawler(ch)

	req := &feed_watcher.FeedCrawlRequest{
		URI:          fmt.Sprintf("%s/%s", ts.URL, "ars.rss"),
		ResponseChan: rchan,
	}

	ch <- req
	resp := <-rchan
	if resp.URI != req.URI {
		t.Errorf("Response URI differs from request.\n")
	}

	if resp.Error != nil {
		t.Errorf("Response had an error when it shouldn't have: %s",
		resp.Error.Error())
	}
}

func TestGetFeed(t *testing.T) {
	ts := httptest.NewServer(fake_server_handler)
	defer ts.Close()

	resp, err := GetFeed(fmt.Sprintf("%s/%s", ts.URL, "ars.rss"))

	if err != nil {
		t.Fatalf("Error getting feed: %s\n", err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		t.Error("GetFeed should return an error when status != 200\n.")
	}

	resp, err = GetFeed(fmt.Sprintf("%s/%s", ts.URL, "error.rss"))

	if err == nil {
		t.Fatalf("Should have gotten error for feed: %s\n", err.Error())
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Error("GetFeed should return an error when status != 200\n.")
	}
}

var fake_server_handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var content []byte
	w.Header().Set("Content-Type", "text/html")
	switch {
	case strings.HasSuffix(r.URL.Path, "ars.rss"):
		feed_resp, err := ioutil.ReadFile("../testdata/ars.rss")
		if err != nil {
			glog.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feed_resp
	case strings.HasSuffix(r.URL.Path, "error.rss"):
		http.Error(w, "Error request", http.StatusInternalServerError)
	case true:
		content = []byte("456")
	}
	w.Write([]byte(content))
})
