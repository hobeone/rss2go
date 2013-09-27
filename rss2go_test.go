package rss2go

import (
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"

	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
	log.Printf("Got request for %s", r.URL.Path)
	switch {
	case strings.HasSuffix(r.URL.Path, "test.rss"):
		feed_resp, err := ioutil.ReadFile("testdata/ars.rss")
		if err != nil {
			log.Fatalf("Error reading test feed: %s", err.Error())
		}
		fmt.Println("Handling test.rss")
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

	db := db.NewDbDispatcher("rss2go_test.db", true)
	MakeDbFixtures(*db, ts.URL)
	all_feeds, err := db.GetAllFeeds()
	fmt.Printf("Got %d feeds to watch.\n", len(all_feeds))

	if err != nil {
		log.Fatalf("Error reading feeds: %s", err.Error())
	}

	mailer := mail.CreateAndStartStubMailer()

	_, response_channel := CreateAndStartFeedWatchers(
		all_feeds, config, mailer, db)

	for i := 0; i < 3; i++ {
		_ = <-response_channel
	}
}
