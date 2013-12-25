package feed

import (
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"fmt"
	"io/ioutil"
	"testing"
	"unicode/utf8"
)

func TestParseFeed(t *testing.T) {
	feed_resp, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	_, s, _ := ParseFeed("http://localhost/feed.rss", feed_resp)

	if len(s) != 25 {
		t.Error("Expected 25 items in the feed.")
	}
}

func TestParseFeedInvalidUTF(t *testing.T) {
	feed_resp, err := ioutil.ReadFile("../testdata/rapha_all.xml")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	tr, err := charset.TranslatorTo("utf-8")
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := tr.Translate(feed_resp, true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(utf8.Valid(b))
	_, s, err := ParseFeed("http://localhost/feed.rss", b)

	if err != nil {
		t.Error(err)
	}

	if len(s) != 2017 {
		t.Errorf("Expected 2017 items in the feed, got %d", len(s))
	}
}
