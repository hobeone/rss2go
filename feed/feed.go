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
	"html"
	"net/url"
	"strings"
	"time"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"github.com/golang/glog"
	"github.com/hobeone/rss2go/atom"
	"github.com/hobeone/rss2go/rdf"
	"github.com/hobeone/rss2go/rss"
	"github.com/mjibson/goread/sanitizer"
)

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

// parent: Feed, key: story ID
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

	xml_decoder := xml.NewDecoder(bytes.NewReader(b))
	xml_decoder.Strict = false
	xml_decoder.CharsetReader = charset.NewReader
	xml_decoder.Entity = xml.HTMLEntity
	err := xml_decoder.Decode(&a)
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
			Title: i.Title,
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
	f.Link = r.Link
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

func ParseFeed(u string, b []byte) (*Feed, []*Story, error) {
	tr, err := charset.TranslatorTo("utf-8")
	if err != nil {
		return nil, nil, err
	}
	_, b, err = tr.Translate(b, true)
	if err != nil {
		return nil, nil, err
	}

	feed, stories, atomerr := parseAtom(u, b)
	if atomerr == nil {
		glog.Infof("Parsed %s as Atom", u)
		return feed, stories, nil
	}

	feed, stories, rsserr := parseRss(u, b)
	if rsserr == nil {
		glog.Infof("Parsed %s as RSS", u)
		return feed, stories, nil
	}

	feed, stories, rdferr := parseRdf(u, b)
	if rdferr == nil {
		glog.Infof("Parsed %s as RDF", u)
		return feed, stories, nil
	}

	err = fmt.Errorf("Error parsing feed. Couldn't find ATOM, RSS or RDF feed. ATOM Error: %s, RSS Error: %s, RDF Error: %s\n", atomerr, rsserr, rdferr)

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
				glog.Infof("unable to resolve link: %v", s.Link)
			}
		}
		su, serr := url.Parse(s.Link)
		if serr != nil {
			su = &url.URL{}
			s.Link = ""
		}
		const snipLen = 100
		s.Content, s.Summary = sanitizer.Sanitize(s.Content, su)
		s.Summary = sanitizer.SnipText(s.Summary, snipLen)
	}

	return f, ss, nil
}
