package feed

import (
	"fmt"
	"io/ioutil"
	"testing"
	"unicode/utf8"

	"github.com/hobeone/rss2go/db"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
)

func TestParseFeed(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	_, s, _ := ParseFeed("http://localhost/feed.rss", feedResp)

	if len(s) != 25 {
		t.Fatal("Expected 25 items in the feed.")
	}
}

func TestParseFeedInvalidUTF(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/rapha_all.xml")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}

	tr, err := charset.TranslatorTo("utf-8")
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := tr.Translate(feedResp, true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(utf8.Valid(b))
	_, s, err := ParseFeed("http://localhost/feed.rss", b)

	if err != nil {
		t.Fatal(err)
	}

	if len(s) != 2017 {
		t.Fatalf("Expected 2017 items in the feed, got %d", len(s))
	}
}

func TestFeedWithBadEntity(t *testing.T) {
	d := db.NewMemoryDBHandle(false, true)
	feeds, _ := db.LoadFixtures(t, d)
	u := *feeds[0]

	feedResp, err := ioutil.ReadFile("../testdata/bad_entity.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	_, _, err = ParseFeed(u.Url, feedResp)

	if err != nil {
		t.Fatal("Feed should be able to parse feeds with unescaped entities")
	}
}
