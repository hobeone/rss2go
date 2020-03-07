package feed

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/sirupsen/logrus"
)

func NullLogger() logrus.FieldLogger {
	l := logrus.New()
	l.Out = ioutil.Discard
	return l
}

func TestParseFeed(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	f, _ := ParseFeed("http://localhost/feed.rss", feedResp)

	if len(f.Items) != 25 {
		t.Fatal("Expected 25 items in the feed.")
	}
}

func TestParseFeedInvalidUTF(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/rapha_all.xml")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	f, err := ParseFeed("http://localhost/feed.rss", feedResp)
	if err != nil {
		t.Fatal(err)
	}

	if len(f.Items) != 2017 {
		t.Fatalf("Expected 2017 items in the feed, got %d", len(f.Items))
	}
}

func TestFeedWithBadEntity(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/bad_entity.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	gf := gofeed.NewParser()
	_, err = gf.ParseString(string(feedResp))
	if err != nil {
		t.Fatalf("Feed should be able to parse feeds with unescaped entities: %s", err)
	}
}

func TestFeedIframeExtraction(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/milanofixed.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	f, _ := ParseFeed("http://localhost/feed.rss", feedResp)

	if len(f.Items) != 1 {
		t.Fatalf("Expected 1 story from feed, got %d", len(f.Items))
	}
	expected := `<a href="//www.youtube.com/embed/dwcwjXLSw00"`
	if !strings.Contains(f.Items[0].Content, expected) {
		t.Fatalf("Couldn't find %v in %v", expected, f.Items[0].Content)
	}
}

func TestBoingBoingFeedIframeExtraction(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/boingboing.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	f, _ := ParseFeed("http://localhost/feed.rss", feedResp)

	if len(f.Items) != 30 {
		t.Fatalf("Expected 1 story from feed, got %d", len(f.Items))
	}
	if err != nil {
		t.Fatalf("Error replacing Iframes: %s", err)
	}
	expected := `<a href="//cdn.embedly.com/widgets/media.html`
	if !strings.Contains(f.Items[1].Content, expected) {
		t.Fatalf("Couldn't find %v in %v", expected, f.Items[1].Content)
	}
}

func TestRadavistImageSizer(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/radavist.rss")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	f, _ := ParseFeed("http://localhost/feed.rss", feedResp)

	if len(f.Items) != 1 {
		t.Fatalf("Expected 1 story from feed, got %d", len(f.Items))
	}
	expected := `<img src="http://theradavist.com/wp-content/uploads/2016/06/TooShort.png" alt="TooShort" style="padding: 0; display: inline;	margin: 0 auto; max-height: 100%; max-width: 100%;"/>`
	if !strings.Contains(f.Items[0].Content, expected) {
		t.Fatalf("Couldn't find %v in %v", expected, f.Items[0].Content)
	}
}
func TestFeedBurnerSocialFlareRemover(t *testing.T) {
	feedResp, err := ioutil.ReadFile("../testdata/seriouseatsfeaturesvideos.atom")
	if err != nil {
		t.Fatal("Error reading test feed.")
	}
	f, _ := ParseFeed("http://localhost/feed.rss", feedResp)

	if len(f.Items) != 1 {
		t.Fatalf("Expected 1 story from feed, got %d", len(f.Items))
	}
	notexpected := `div class="feedflare"`
	if strings.Contains(f.Items[0].Content, notexpected) {
		t.Fatalf("Found %v in %v", notexpected, f.Items[0].Content)
	}
}
