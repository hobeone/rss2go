package main

import (
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/server"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type NullWriter int

func (NullWriter) Write([]byte) (int, error) { return 0, nil }

func DisableLogging() {
	log.SetOutput(new(NullWriter))
}

func init() {
	DisableLogging()
}

func MakeDbFixtures(d db.DbDispatcher, local_url string) {

	d.OrmHandle.Exec("delete from feed_info;")

	all_feeds := []db.FeedInfo{
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
		feed_resp, err := ioutil.ReadFile("../testdata/ars.rss")
		if err != nil {
			log.Fatalf("Error reading test feed: %s", err.Error())
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
		log.Printf("Call to mock sleep, sleeping for just 1 second.")
		time.Sleep(time.Second * time.Duration(1))
		return
	}

	config := config.NewConfig()

	os.Remove("rss2go_test.db")

	// Set second argument to true to debug sql
	db := db.NewDbDispatcher("rss2go_test.db", false, true)
	MakeDbFixtures(*db, ts.URL)
	all_feeds, err := db.GetAllFeeds()
	fmt.Printf("Got %d feeds to watch.\n", len(all_feeds))

	if err != nil {
		log.Fatalf("Error reading feeds: %s", err.Error())
	}

	mailer := mail.CreateAndStartStubMailer()

	feeds, response_channel := CreateAndStartFeedWatchers(
		all_feeds, config, mailer, db)

	go server.StartHttpServer(config, feeds)

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
