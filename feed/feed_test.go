package feed

import (
	"io/ioutil"
	"strings"
	"testing"

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
	feeds, _ := db.LoadFixtures(t, d, "http://localhost")
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

func TestFeedIframeExtraction(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/milanofixed.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	_, s, _ := ParseFeed("http://localhost/feed.rss", feedResp)

	if len(s) != 1 {
		t.Fatalf("Expected 1 story from feed, got %d", len(s))
	}
	replaced, err := cleanFeedContent(s[0].Content)
	if err != nil {
		t.Fatalf("Error replacing Iframes: %s", err)
	}
	expected := `<a href="//www.youtube.com/embed/dwcwjXLSw00"`
	if !strings.Contains(replaced, expected) {
		t.Fatalf("Couldn't find %v in %v", expected, replaced)
	}
}

func TestBoingBoingFeedIframeExtraction(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/boingboing.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	_, s, _ := ParseFeed("http://localhost/feed.rss", feedResp)

	if len(s) != 30 {
		t.Fatalf("Expected 1 story from feed, got %d", len(s))
	}

	if err != nil {
		t.Fatalf("Error replacing Iframes: %s", err)
	}
	expected := `<a href="//cdn.embedly.com/widgets/media.html`
	if !strings.Contains(s[1].Content, expected) {
		t.Fatalf("Couldn't find %v in %v", expected, s[1].Content)
	}
}
