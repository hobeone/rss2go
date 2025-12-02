package feed

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"

	"golang.org/x/net/html"
)

// ParseFeed will try to find an Atom, RSS or RDF feed in the given byte array (in that order).
func ParseFeed(url string, b []byte) (*gofeed.Feed, error) {
	feedString := strings.ToValidUTF8(string(b), "")
	fp := gofeed.NewParser()
	feed, err := fp.ParseString(feedString)

	if err != nil {
		return nil, err
	}

	feed, err = parseFix(feed)
	if err != nil {
		return nil, err
	}
	return feed, nil
}

func parseFix(f *gofeed.Feed) (*gofeed.Feed, error) {
	f.Link = strings.TrimSpace(f.Link)
	f.Title = html.UnescapeString(strings.TrimSpace(f.Title))

	if ul, err := url.Parse(f.Link); err == nil {
		f.Link = ul.String()
	}
	base, err := url.Parse(f.Link)
	if err != nil {
		slog.Info("unable to parse link", "link", f.Link)
	}

	for _, s := range f.Items {
		if len(strings.TrimSpace(s.Content)) == 0 {
			if len(s.Description) != 0 {
				s.Content = s.Description
			}
		}

		s.Link = strings.TrimSpace(s.Link)
		if s.UpdatedParsed != nil && s.PublishedParsed != nil {
			if !s.UpdatedParsed.IsZero() && s.PublishedParsed.IsZero() {
				s.Published = s.Updated
			}
		}
		/*
			if !s.UpdatedParsed.IsZero() {
				s.Date = s.UpdatedParsed.Unix()
			} else {
				s.Date = s.PublishedParsed.Unix()
			}
		*/
		if s.GUID == "" {
			if s.Link != "" {
				s.GUID = s.Link
			} else if s.Title != "" {
				s.GUID = s.Title
			} else {
				slog.Info("feed: story has no id", "story", s)
				return nil, fmt.Errorf("story has no id: %v", s)
			}
		}
		s.Title = fullyHTMLUnescape(s.Title)
		// if a story doesn't have a link, see if its id is a URL
		if s.Link == "" {
			if u, err := url.Parse(s.GUID); err == nil {
				s.Link = u.String()
			}
		}
		if base != nil && s.Link != "" {
			link, err := base.Parse(s.Link)
			if err == nil {
				s.Link = link.String()
			} else {
				slog.Info("feed: unable to resolve link", "error", err, "link", s.Link)
			}
		}
		_, serr := url.Parse(s.Link)
		if serr != nil {
			s.Link = ""
		}

		// Most mail readers disallow IFRAMES in mail content.  This breaks
		// embedding of things like youtube videos.  By changing them to anchor
		// tags things like Gmail will do their own embedding when reading the
		// mail.
		//
		// The following ends up parsing each of the feed items at least 3 times
		// which seems excessive - but meh.
		s.Content, err = cleanFeedContent(s.Content)
		if err != nil {
			slog.Error("feed: error cleaning up content", "error", err)
		}

		p := bluemonday.UGCPolicy()
		s.Content = fullyHTMLUnescape(p.Sanitize(s.Content))

		s.Content, err = rewriteFeedContent(s.Content)
		if err != nil {
			slog.Error("feed: error cleaning up content", "error", err)
		}

	}

	return f, nil
}

// Try to (up to 10 times) to unescape a string.
// Some feeds are double escaped with things like: &amp;amp;
func fullyHTMLUnescape(orig string) string {
	mod := orig
	for range 10 {
		mod = html.UnescapeString(orig)
		if orig == mod {
			return mod
		}
		orig = mod
	}
	return mod
}

func cleanFeedContent(htmlFrag string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlFrag))
	if err != nil {
		return htmlFrag, err
	}
	doc.Find("iframe").Each(func(i int, s *goquery.Selection) {
		val, exists := s.Attr("src")
		if exists && val != "" {
			escLinkSrc, err := url.QueryUnescape(val)
			if err != nil {
				slog.Info("feed: error unescaping iframe URL", "error", err, "url", val)
				return
			}
			s.ReplaceWithHtml(fmt.Sprintf(`<a href="%s">%s</a>`, escLinkSrc, escLinkSrc))
		}
	})
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		val, exists := s.Attr("href")
		if exists && strings.Contains(val, "da.feedsportal.com") {
			s.Remove()
		}
	})
	doc.Find(".feedflare").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})
	r, err := doc.Html()
	if err != nil {
		return htmlFrag, err
	}

	return r, nil

}

// Run post bluemonday sanitization.  These rewrites will introduce
// modifications that bluemonday would strip but since we control them they
// should be safe.
func rewriteFeedContent(htmlFrag string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlFrag))
	if err != nil {
		return htmlFrag, err
	}
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		s.RemoveAttr("width")
		s.RemoveAttr("style")
		s.RemoveAttr("height")
		s.SetAttr("style", `padding: 0; display: inline;	margin: 0 auto; max-height: 100%; max-width: 100%;`)
	})
	r, err := doc.Html()
	if err != nil {
		return htmlFrag, err
	}

	return r, nil
}
