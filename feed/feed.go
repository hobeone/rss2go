// Mostly just lifted straight from Matt Jibson's goread app
// github.com/mjibson/goread
//
// Copyright (c) 2013 Matt Jibson <matt.jibson@gmail.com>
//

package feed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/Sirupsen/logrus"
	"github.com/microcosm-cc/bluemonday"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"

	"github.com/hobeone/rss2go/atom"
	"github.com/hobeone/rss2go/rdf"
	"github.com/hobeone/rss2go/rss"
)

// Feed represents the basic information for a RSS, Atom or RDF feed.
type Feed struct {
	URL        string
	Title      string
	Updated    time.Time
	NextUpdate time.Time
	Link       string
	Checked    time.Time
	Image      string
	Hub        string
}

// Story represents in individual item from a feed.
type Story struct {
	ID           string
	Title        string
	Link         string
	Created      time.Time
	Published    time.Time
	Updated      time.Time
	Date         int64
	Author       string
	MediaContent string
	Feed         *Feed

	Content string
}

func parseAtom(u string, b []byte) (*Feed, []*Story, error) {
	a := atom.Feed{}
	var fb, eb *url.URL

	xmlDecoder := xml.NewDecoder(bytes.NewReader(b))
	xmlDecoder.Strict = false
	xmlDecoder.CharsetReader = charset.NewReaderLabel
	xmlDecoder.Entity = xml.HTMLEntity
	err := xmlDecoder.Decode(&a)
	if err != nil {
		return nil, nil, err
	}

	f := Feed{
		URL: u,
	}
	s := []*Story{}

	f.Title = a.Title
	if t, err := parseDate(string(a.Updated)); err == nil {
		f.Updated = t
	}

	if fb, err = url.Parse(a.XMLBase); err != nil {
		fb, _ = url.Parse("")
	}
	if len(a.Link) > 0 {
		f.Link = findBestAtomLink(a.Link)
		if l, err := fb.Parse(f.Link); err == nil {
			f.Link = l.String()
		}
		for _, l := range a.Link {
			if l.Rel == "hub" {
				f.Hub = l.Href
				break
			}
		}
	}

	for _, i := range a.Entry {
		if eb, err = fb.Parse(i.XMLBase); err != nil {
			eb = fb
		}
		st := Story{
			ID:    i.ID,
			Title: i.Title.ToString(),
			Feed:  &f,
		}
		if t, err := parseDate(string(i.Updated)); err == nil {
			st.Updated = t
		}
		if t, err := parseDate(string(i.Published)); err == nil {
			st.Published = t
		}
		if len(i.Link) > 0 {
			st.Link = findBestAtomLink(i.Link)
			if l, err := eb.Parse(st.Link); err == nil {
				st.Link = l.String()
			}
		}
		if i.Author != nil {
			st.Author = i.Author.Name
		}
		if i.Content != nil {
			if len(strings.TrimSpace(i.Content.Body)) != 0 {
				st.Content = i.Content.Body
			} else if len(i.Content.InnerXML) != 0 {
				st.Content = i.Content.InnerXML
			}
		} else if i.Summary != nil {
			st.Content = i.Summary.Body
		}
		s = append(s, &st)
	}

	return parseFix(&f, s)
}

func defaultCharsetReader(cs string, input io.Reader) (io.Reader, error) {
	e, _ := charset.Lookup(cs)
	if e == nil {
		return nil, fmt.Errorf("cannot decode charset %v", cs)
	}
	return transform.NewReader(input, e.NewDecoder()), nil
}
func nilCharsetReader(cs string, input io.Reader) (io.Reader, error) {
	return input, nil
}

func parseRss(u string, b []byte) (*Feed, []*Story, error) {
	r := rss.Rss{}

	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = charset.NewReaderLabel
	d.DefaultSpace = "DefaultSpace"
	d.Entity = xml.HTMLEntity

	err := d.Decode(&r)
	if err != nil {
		return nil, nil, err
	}

	f := Feed{
		URL: u,
	}
	s := []*Story{}

	f.Title = r.Title
	f.Link = r.BaseLink()
	if t, err := parseDate(r.LastBuildDate, r.PubDate); err == nil {
		f.Updated = t
	} else {
		logrus.Infof("feed: no rss feed date: %v", f.Link)
	}

	for _, i := range r.Items {
		st := Story{
			Link:   i.Link,
			Author: i.Author,
			Feed:   &f,
		}
		if i.Title != "" {
			st.Title = i.Title
		} else if i.Description != "" {
			i.Title = i.Description
		}
		if i.Content != "" {
			st.Content = i.Content
		} else if i.Title != "" && i.Description != "" {
			st.Content = i.Description
		}
		if i.Guid != nil {
			st.ID = i.Guid.Guid
		}
		if i.Media != nil {
			st.MediaContent = i.Media.URL
		}
		if t, err := parseDate(i.PubDate, i.Date, i.Published); err == nil {
			st.Published = t
			st.Updated = t
		}

		s = append(s, &st)
	}

	return parseFix(&f, s)
}

func parseRdf(u string, b []byte) (*Feed, []*Story, error) {
	rd := rdf.RDF{}

	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReaderLabel
	d.Strict = false
	d.Entity = xml.HTMLEntity
	err := d.Decode(&rd)
	if err != nil {
		return nil, nil, err
	}

	f := Feed{
		URL: u,
	}
	s := []*Story{}

	if rd.Channel != nil {
		f.Title = rd.Channel.Title
		f.Link = rd.Channel.Link
		if t, err := parseDate(rd.Channel.Date); err == nil {
			f.Updated = t
		}
	}

	for _, i := range rd.Item {
		st := Story{
			ID:     i.About,
			Title:  i.Title,
			Link:   i.Link,
			Author: i.Creator,
			Feed:   &f,
		}
		st.Content = html.UnescapeString(i.Description)
		if t, err := parseDate(i.Date); err == nil {
			st.Published = t
			st.Updated = t
		}
		s = append(s, &st)
	}

	return parseFix(&f, s)
}

// Copied from xml decoder
func isInCharacterRange(r rune) (inrange bool) {
	return r == 0x09 ||
		r == 0x0A ||
		r == 0x0D ||
		r >= 0x20 && r <= 0xDF77 ||
		r >= 0xE000 && r <= 0xFFFD ||
		r >= 0x10000 && r <= 0x10FFFF
}

func removeInvalidCharacters(s string) string {
	v := make([]rune, 0, len(s))
	for i, r := range s {
		if r == utf8.RuneError {
			_, size := utf8.DecodeRuneInString(s[i:])
			if size == 1 {
				continue
			}
		}
		if !isInCharacterRange(r) {
			r = ' '
		}
		v = append(v, r)
	}
	return string(v)
}

// ParseFeed will try to find an Atom, RSS or RDF feed in the given byte array (in that order).
func ParseFeed(url string, b []byte) (*Feed, []*Story, error) {
	// super lame
	b = []byte(removeInvalidCharacters(string(b)))

	feed, stories, rsserr := parseRss(url, b)
	if rsserr == nil {
		logrus.Infof("feed: parsed %s as RSS", url)
		return feed, stories, nil
	}

	feed, stories, atomerr := parseAtom(url, b)
	if atomerr == nil {
		logrus.Infof("feed: parsed %s as Atom", url)
		return feed, stories, nil
	}

	feed, stories, rdferr := parseRdf(url, b)
	if rdferr == nil {
		logrus.Infof("feed: parsed %s as RDF", url)
		return feed, stories, nil
	}

	err := fmt.Errorf("feed: couldn't find ATOM, RSS or RDF feed for %s. ATOM Error: %s, RSS Error: %s, RDF Error: %s\n", url, atomerr, rsserr, rdferr)

	logrus.Errorf("feed: %v", err.Error())
	return nil, nil, err
}

func findBestAtomLink(links []atom.Link) string {
	getScore := func(l atom.Link) int {
		switch {
		case l.Rel == "hub":
			return 0
		case l.Rel == "alternate" && l.Type == "text/html":
			return 5
		case l.Type == "text/html":
			return 4
		case l.Rel == "self":
			return 2
		case l.Rel == "":
			return 3
		default:
			return 1
		}
	}

	var bestlink string
	bestscore := -1
	for _, l := range links {
		score := getScore(l)
		if score > bestscore {
			bestlink = l.Href
			bestscore = score
		}
	}

	return bestlink
}

func parseFix(f *Feed, ss []*Story) (*Feed, []*Story, error) {
	f.Checked = time.Now()

	f.Link = strings.TrimSpace(f.Link)
	f.Title = html.UnescapeString(strings.TrimSpace(f.Title))

	if u, err := url.Parse(f.URL); err == nil {
		if ul, err := u.Parse(f.Link); err == nil {
			f.Link = ul.String()
		}
	}
	base, err := url.Parse(f.Link)
	if err != nil {
		logrus.Infof("unable to parse link: %v", f.Link)
	}

	for _, s := range ss {
		s.Created = f.Checked
		s.Link = strings.TrimSpace(s.Link)
		if !s.Updated.IsZero() && s.Published.IsZero() {
			s.Published = s.Updated
		}
		if s.Published.IsZero() || f.Checked.Before(s.Published) {
			s.Published = f.Checked
		}
		if !s.Updated.IsZero() {
			s.Date = s.Updated.Unix()
		} else {
			s.Date = s.Published.Unix()
		}
		if s.ID == "" {
			if s.Link != "" {
				s.ID = s.Link
			} else if s.Title != "" {
				s.ID = s.Title
			} else {
				logrus.Infof("feed: story has no id: %v", s)
				return nil, nil, fmt.Errorf("story has no id: %v", s)
			}
		}
		s.Title = fullyHTMLUnescape(s.Title)
		// if a story doesn't have a link, see if its id is a URL
		if s.Link == "" {
			if u, err := url.Parse(s.ID); err == nil {
				s.Link = u.String()
			}
		}
		if base != nil && s.Link != "" {
			link, err := base.Parse(s.Link)
			if err == nil {
				s.Link = link.String()
			} else {
				logrus.Infof("feed: unable to resolve link: %s: %v", err, s.Link)
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
			logrus.Errorf("feed: error cleaning up content: %s", err)
		}

		p := bluemonday.UGCPolicy()
		s.Content = fullyHTMLUnescape(p.Sanitize(s.Content))

		s.Content, err = rewriteFeedContent(s.Content)
		if err != nil {
			logrus.Errorf("feed: error cleaning up content: %s", err)
		}

	}

	return f, ss, nil
}

// Try to (up to 10 times) to unescape a string.
// Some feeds are double escaped with things like: &amp;amp;
func fullyHTMLUnescape(orig string) string {
	mod := orig
	for i := 0; i < 10; i++ {
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
				logrus.Info("feed: error unescaping iframe URL. Error: %s, URL: %#v", err, val)
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
