package feed

import (
	"testing"
	"io/ioutil"
	"fmt"
)

func TestParseFeed(t *testing.T) {
	feed_resp, err := ioutil.ReadFile("testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	f, s := ParseFeed("http://localhost/feed.rss", feed_resp)

	fmt.Printf("%#v\n", f)
	if len(s) != 25 {
		t.Error("Expected 25 items in the feed.")
	}
}
