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
	"net/url"
	"strings"
	"time"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"code.google.com/p/go-html-transform/h5"
	"code.google.com/p/go-html-transform/html/transform"
	"code.google.com/p/go.net/html"

	"github.com/golang/glog"
	"github.com/hobeone/rss2go/atom"
	"github.com/hobeone/rss2go/rdf"
	"github.com/hobeone/rss2go/rss"
	"github.com/mjibson/goread/sanitizer"
)

// Feed represents the basic information for a RSS, Atom or RDF feed.
type Feed struct {
	Url        string
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
	Id           string
	Title        string
	Link         string
	Created      time.Time
	Published    time.Time
	Updated      time.Time
	Date         int64
	Author       string
	Summary      string
	MediaContent string
	Feed         *Feed

	Content string
}

func parseAtom(u string, b []byte) (*Feed, []*Story, error) {
	a := atom.Feed{}
	var fb, eb *url.URL

	xmlDecoder := xml.NewDecoder(bytes.NewReader(b))
	xmlDecoder.Strict = false
	xmlDecoder.CharsetReader = charset.NewReader
	xmlDecoder.Entity = xml.HTMLEntity
	err := xmlDecoder.Decode(&a)
	if err != nil {
		return nil, nil, err
	}

	f := Feed{
		Url: u,
	}
	s := []*Story{}

	f.Title = a.Title
	if t, err := parseDate(&f, string(a.Updated)); err == nil {
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
			Id:    i.ID,
			Title: i.Title.ToString(),
			Feed:  &f,
		}
		if t, err := parseDate(&f, string(i.Updated)); err == nil {
			st.Updated = t
		}
		if t, err := parseDate(&f, string(i.Published)); err == nil {
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

func parseRss(u string, b []byte) (*Feed, []*Story, error) {
	r := rss.Rss{}

	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = charset.NewReader
	d.DefaultSpace = "DefaultSpace"
	d.Entity = xml.HTMLEntity

	err := d.Decode(&r)
	if err != nil {
		return nil, nil, err
	}

	f := Feed{
		Url: u,
	}
	s := []*Story{}

	f.Title = r.Title
	f.Link = r.BaseLink()
	if t, err := parseDate(&f, r.LastBuildDate, r.PubDate); err == nil {
		f.Updated = t
	} else {
		glog.Infof("no rss feed date: %v", f.Link)
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
			st.Id = i.Guid.Guid
		}
		if i.Media != nil {
			st.MediaContent = i.Media.URL
		}
		if t, err := parseDate(&f, i.PubDate, i.Date, i.Published); err == nil {
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
	d.CharsetReader = charset.NewReader
	d.Strict = false
	d.Entity = xml.HTMLEntity
	err := d.Decode(&rd)
	if err != nil {
		return nil, nil, err
	}

	f := Feed{
		Url: u,
	}
	s := []*Story{}

	if rd.Channel != nil {
		f.Title = rd.Channel.Title
		f.Link = rd.Channel.Link
		if t, err := parseDate(&f, rd.Channel.Date); err == nil {
			f.Updated = t
		}
	}

	for _, i := range rd.Item {
		st := Story{
			Id:     i.About,
			Title:  i.Title,
			Link:   i.Link,
			Author: i.Creator,
			Feed:   &f,
		}
		st.Content = html.UnescapeString(i.Description)
		if t, err := parseDate(&f, i.Date); err == nil {
			st.Published = t
			st.Updated = t
		}
		s = append(s, &st)
	}

	return parseFix(&f, s)
}

// ParseFeed will try to find an Atom, RSS or RDF feed in the given byte array (in that order).
func ParseFeed(url string, b []byte) (*Feed, []*Story, error) {
	tr, err := charset.TranslatorTo("utf-8")
	if err != nil {
		return nil, nil, err
	}
	_, b, err = tr.Translate(b, true)
	if err != nil {
		return nil, nil, err
	}

	feed, stories, atomerr := parseAtom(url, b)
	if atomerr == nil {
		glog.Infof("Parsed %s as Atom", url)
		return feed, stories, nil
	}

	feed, stories, rsserr := parseRss(url, b)
	if rsserr == nil {
		glog.Infof("Parsed %s as RSS", url)
		return feed, stories, nil
	}

	feed, stories, rdferr := parseRdf(url, b)
	if rdferr == nil {
		glog.Infof("Parsed %s as RDF", url)
		return feed, stories, nil
	}

	err = fmt.Errorf("couldn't find ATOM, RSS or RDF feed for %s. ATOM Error: %s, RSS Error: %s, RDF Error: %s\n", url, atomerr, rsserr, rdferr)

	glog.Info(err.Error())
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

	if u, err := url.Parse(f.Url); err == nil {
		if ul, err := u.Parse(f.Link); err == nil {
			f.Link = ul.String()
		}
	}
	base, err := url.Parse(f.Link)
	if err != nil {
		glog.Infof("unable to parse link: %v", f.Link)
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
		if s.Id == "" {
			if s.Link != "" {
				s.Id = s.Link
			} else if s.Title != "" {
				s.Id = s.Title
			} else {
				glog.Infof("story has no id: %v", s)
				return nil, nil, fmt.Errorf("story has no id: %v", s)
			}
		}
		s.Title = fullyUnescape(s.Title)
		// if a story doesn't have a link, see if its id is a URL
		if s.Link == "" {
			if u, err := url.Parse(s.Id); err == nil {
				s.Link = u.String()
			}
		}
		if base != nil && s.Link != "" {
			link, err := base.Parse(s.Link)
			if err == nil {
				s.Link = link.String()
			} else {
				glog.Infof("unable to resolve link: %s: %v", err, s.Link)
			}
		}
		su, serr := url.Parse(s.Link)
		if serr != nil {
			su = &url.URL{}
			s.Link = ""
		}
		const snipLen = 100

		// Most mail readers disallow IFRAMES in mail content.  This breaks
		// embedding of things like youtube videos.  By changing them to anchor
		// tags things like Gmail will do their own embedding when reading the
		// mail.
		s.Content, err = replaceIframes(s.Content)
		if err != nil {
			glog.Errorf("Error replacing IFRAMES with Anchor tags: %s", err)
		}
		s.Content, s.Summary = sanitizer.Sanitize(s.Content, su)
		s.Summary = sanitizer.SnipText(s.Summary, snipLen)
		s.Content = fullyUnescape(s.Content)
		s.Summary = fullyUnescape(s.Summary)
	}

	return f, ss, nil
}

// Try to (up to 10 times) to unescape a string.
// Some feeds are double escaped with things like: &amp;amp;
func fullyUnescape(orig string) string {
	mod := orig
	for i := 0; i < 10; i++ {
		mod = html.UnescapeString(orig)
		if orig == mod {
			fmt.Printf("Had to unesca[e %d times\n", i)
			return mod
		}
		orig = mod
	}
	fmt.Printf("Had to unesca[e %d times\n", 10)
	return mod
}

func convertIframeToAnchor(n *html.Node) {
	p := n.Parent
	if n.Parent == nil {
		// Won't mess with the root node
		return
	}
	linkSrc := ""
	for i, attr := range n.Attr {
		if attr.Key == "src" {
			linkSrc = n.Attr[i].Val
		}
	}
	if linkSrc != "" {
		p.InsertBefore(h5.Anchor(linkSrc, linkSrc), n)
		p.RemoveChild(n)
	}
}

func replaceIframes(htmlFrag string) (string, error) {
	doc, err := transform.NewFromReader(strings.NewReader(htmlFrag))
	if err != nil {
		return htmlFrag, err
	}
	doc.ApplyAll(
		transform.MustTrans(convertIframeToAnchor, "iframe"),
	)
	return doc.String(), nil
}
