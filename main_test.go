package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/hobeone/rss2go/commands"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
)

func MakeDbFixtures(d *db.DBHandle, localURL string) {
	allFeeds := []db.FeedInfo{
		{
			Name: "Testing1",
			Url:  localURL + "/test.rss",
		},
	}

	for _, f := range allFeeds {
		err := d.SaveFeed(&f)
		if err != nil {
			panic(err)
		}
	}
}

var fakeServerHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var content []byte
	switch {
	case strings.HasSuffix(r.URL.Path, "test.rss"):
		feedResp, err := ioutil.ReadFile("testdata/ars.rss")
		if err != nil {
			glog.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feedResp
	case true:
		content = []byte("456")
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
})

func TestEndToEndIntegration(t *testing.T) {
	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	// Override the sleep function
	feedwatcher.After = func(d time.Duration) <-chan time.Time {
		glog.Infof("Call to mock After, waiting for just 1 second.")
		return time.After(time.Second * time.Duration(1))
	}

	cfg := config.NewConfig()
	cfg.Mail.SendMail = false
	cfg.Db.Verbose = false

	d := commands.NewDaemon(cfg)
	d.Dbh = db.NewMemoryDBHandle(false, true)
	MakeDbFixtures(d.Dbh, ts.URL)
	allFeeds, err := d.Dbh.GetAllFeeds()

	if err != nil {
		glog.Fatalf("Error reading feeds: %s", err.Error())
	}

	d.CreateAndStartFeedWatchers(allFeeds)

	resp := <-d.RespChan
	if len(resp.Items) != 25 {
		t.Errorf("Expected 25 items from the feed. Got %d", len(resp.Items))
	}

	resp = <-d.RespChan
	if len(resp.Items) != 0 {
		t.Errorf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
}
