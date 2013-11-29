package main

import (
	"github.com/golang/glog"
	"github.com/hobeone/rss2go/commands"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func MakeDbFixtures(d db.DbDispatcher, local_url string) {
	all_feeds := []db.FeedInfo{
		{
			Name: "Testing1",
			Url:  local_url + "/test.rss",
		},
	}

	for _, f := range all_feeds {
		d.Orm.Save(&f)
	}
}

var fake_server_handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var content []byte
	switch {
	case strings.HasSuffix(r.URL.Path, "test.rss"):
		feed_resp, err := ioutil.ReadFile("testdata/ars.rss")
		if err != nil {
			glog.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feed_resp
	case true:
		content = []byte("456")
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
})

func TestEndToEndIntegration(t *testing.T) {
	ts := httptest.NewServer(fake_server_handler)
	defer ts.Close()

	// Override the sleep function
	feed_watcher.Sleep = func(d time.Duration) {
		glog.Infof("Call to mock sleep, sleeping for just 1 second.")
		time.Sleep(time.Second * time.Duration(1))
		return
	}

	config := config.NewConfig()

	// Set first argument to true to debug sql
	db := db.NewMemoryDbDispatcher(false, true)
	MakeDbFixtures(*db, ts.URL)
	all_feeds, err := db.GetAllFeeds()

	if err != nil {
		glog.Fatalf("Error reading feeds: %s", err.Error())
	}

	mailer := mail.CreateAndStartStubMailer()

	_, _, response_channel := commands.CreateAndStartFeedWatchers(
		all_feeds, config, mailer, db)

	resp := <-response_channel
	if len(resp.Items) != 25 {
		t.Errorf("Expected 25 items from the feed. Got %d", len(resp.Items))
	}

	resp = <-response_channel
	if len(resp.Items) != 0 {
		t.Errorf("Expected 0 items from the feed. Got %d", len(resp.Items))
	}
	os.Remove("rss2go_test.db")
}
