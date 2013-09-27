// Mostly just lifted straight from Matt Jibson's goread app
// github.com/mjibson/goread

package feed

import (
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"
	"log"
	"html"
	"bytes"
	"github.com/hobeone/rss2go/atom"
	"github.com/hobeone/rss2go/rdf"
	"github.com/hobeone/rss2go/rss"
)

var dateFormats = []string{
	"01-02-2006",
	"01/02/2006 15:04:05 MST",
	"02 Jan 2006 15:04 MST",
	"02 Jan 2006 15:04:05 -0700",
	"02 Jan 2006 15:04:05 MST",
	"02 Jan 2006 15:04:05 UT",
	"02 Jan 2006",
	"02-01-2006 15:04:05 MST",
	"02.01.2006 -0700",
	"02.01.2006 15:04:05",
	"02/01/2006 15:04:05",
	"02/01/2006",
	"06-1-2 15:04",
	"06/1/2 15:04",
	"1/2/2006 15:04:05 MST",
	"1/2/2006 3:04:05 PM",
	"15:04 02.01.2006 -0700",
	"2 Jan 2006 15:04:05 MST",
	"2 Jan 2006",
	"2 January 2006 15:04:05 -0700",
	"2 January 2006",
	"2006 January 02",
	"2006-01-02 00:00:00.0 15:04:05.0 -0700",
	"2006-01-02 15:04",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05 MST",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05Z",
	"2006-01-02",
	"2006-01-02T15:04-07:00",
	"2006-01-02T15:04:05 -0700",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05-07:00:00",
	"2006-01-02T15:04:05:-0700",
	"2006-01-02T15:04:05:00",
	"2006-01-02T15:04:05Z",
	"2006-1-02T15:04:05Z",
	"2006-1-2 15:04:05",
	"2006-1-2",
	"2006/01/02",
	"6-1-2 15:04",
	"6/1/2 15:04",
	"Jan 02 2006 03:04:05PM",
	"Jan 2, 2006 15:04:05 MST",
	"Jan 2, 2006 3:04:05 PM MST",
	"January 02, 2006 03:04 PM",
	"January 02, 2006 15:04",
	"January 02, 2006 15:04:05 MST",
	"January 02, 2006",
	"January 2, 2006 03:04 PM",
	"January 2, 2006 15:04:05 MST",
	"January 2, 2006 15:04:05",
	"January 2, 2006",
	"January 2, 2006, 3:04 p.m.",
	"Mon 02 Jan 2006 15:04:05 -0700",
	"Mon 2 Jan 2006 15:04:05 MST",
	"Mon Jan 2 15:04 2006",
	"Mon Jan 2 15:04:05 2006 MST",
	"Mon, 02 Jan 06 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04 -0700",
	"Mon, 02 Jan 2006 15:04 MST",
	"Mon, 02 Jan 2006 15:04:05 --0700",
	"Mon, 02 Jan 2006 15:04:05 -07",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 -07:00",
	"Mon, 02 Jan 2006 15:04:05 00",
	"Mon, 02 Jan 2006 15:04:05 MST -0700",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 MST-07:00",
	"Mon, 02 Jan 2006 15:04:05 UT",
	"Mon, 02 Jan 2006 15:04:05 Z",
	"Mon, 02 Jan 2006 15:04:05",
	"Mon, 02 Jan 2006 15:04:05MST",
	"Mon, 02 Jan 2006 3:04:05 PM MST",
	"Mon, 02 Jan 2006",
	"Mon, 02 January 2006",
	"Mon, 2 Jan 06 15:04:05 -0700",
	"Mon, 2 Jan 06 15:04:05 MST",
	"Mon, 2 Jan 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04",
	"Mon, 2 Jan 2006 15:04:05 -0700 MST",
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04:05 UT",
	"Mon, 2 Jan 2006 15:04:05",
	"Mon, 2 Jan 2006 15:04:05-0700",
	"Mon, 2 Jan 2006 15:04:05MST",
	"Mon, 2 Jan 2006 15:4:5 MST",
	"Mon, 2 Jan 2006",
	"Mon, 2 Jan 2006, 15:04 -0700",
	"Mon, 2 January 2006 15:04:05 -0700",
	"Mon, 2 January 2006 15:04:05 MST",
	"Mon, 2 January 2006, 15:04 -0700",
	"Mon, 2 January 2006, 15:04:05 MST",
	"Mon, 2, Jan 2006 15:4",
	"Mon, Jan 2 2006 15:04:05 -0700",
	"Mon, Jan 2 2006 15:04:05 -700",
	"Mon, January 02, 2006, 15:04:05 MST",
	"Mon, January 2 2006 15:04:05 -0700",
	"Mon,02 Jan 2006 15:04:05 -0700",
	"Mon,02 January 2006 14:04:05 MST",
	"Monday, 02 January 2006 15:04:05 -0700",
	"Monday, 02 January 2006 15:04:05 MST",
	"Monday, 02 January 2006 15:04:05",
	"Monday, 2 Jan 2006 15:04:05 -0700",
	"Monday, 2 Jan 2006 15:04:05 MST",
	"Monday, 2 January 2006 15:04:05 -0700",
	"Monday, 2 January 2006 15:04:05 MST",
	"Monday, January 02, 2006",
	"Monday, January 2, 2006 03:04 PM",
	"Monday, January 2, 2006 15:04:05 MST",
	"Monday, January 2, 2006",
	"Updated January 2, 2006",
	"mon,2 Jan 2006 15:04:05 MST",
	time.ANSIC,
	time.RFC1123,
	time.RFC1123Z,
	time.RFC3339,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RubyDate,
	time.UnixDate,
}

type Feed struct {
	Url         string
	Title       string
	Updated time.Time
	NextUpdate  time.Time
	Link string
	Checked time.Time
	Image string
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

	Content string
}

func parseDate(feed *Feed, ds ...string) (t time.Time, err error) {
	for _, d := range ds {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		for _, f := range dateFormats {
			if t, err = time.Parse(f, d); err == nil {
				return
			}
		}
	}
	err = fmt.Errorf("could not parse date: %v", strings.Join(ds, ", "))
	return
}

func ParseFeed(u string, b []byte) (*Feed, []*Story) {
	f := Feed{Url: u}
	var s []*Story

	a := atom.Feed{}
	var atomerr, rsserr, rdferr, err error
	var fb, eb *url.URL
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	if atomerr = d.Decode(&a); atomerr == nil {
		f.Title = a.Title
		if t, err := parseDate(&f, string(a.Updated)); err == nil {
			f.Updated = t
		}

		if fb, err = url.Parse(a.XMLBase); err != nil {
			fb, _ = url.Parse("")
		}
		if len(a.Link) > 0 {
			f.Link = findBestAtomLink(a.Link).Href
			if l, err := fb.Parse(f.Link); err == nil {
				f.Link = l.String()
			}
		}

		for _, i := range a.Entry {
			if eb, err = fb.Parse(i.XMLBase); err != nil {
				eb = fb
			}
			st := Story{
				Id:    i.ID,
				Title: i.Title,
			}
			if t, err := parseDate(&f, string(i.Updated)); err == nil {
				st.Updated = t
			}
			if t, err := parseDate(&f, string(i.Published)); err == nil {
				st.Published = t
			}
			if len(i.Link) > 0 {
				st.Link = findBestAtomLink(i.Link).Href
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

	r := rss.Rss{}
	d = xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	d.DefaultSpace = "DefaultSpace"
	if rsserr = d.Decode(&r); rsserr == nil {
		f.Title = r.Title
		f.Link = r.Link
		if t, err := parseDate(&f, r.LastBuildDate, r.PubDate); err == nil {
			f.Updated = t
		} else {
			log.Printf("no rss feed date: %v", f.Link)
		}

		for _, i := range r.Items {
			st := Story{
				Link:   i.Link,
				Author: i.Author,
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

	rd := rdf.RDF{}
	d = xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	if rdferr = d.Decode(&rd); rdferr == nil {
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

	log.Printf("atom parse error: %s", atomerr.Error())
	log.Printf("xml parse error: %s", rsserr.Error())
	log.Printf("rdf parse error: %s", rdferr.Error())
	return nil, nil
}

func findBestAtomLink(links []atom.Link) atom.Link {
	getScore := func(l atom.Link) int {
		switch {
		case l.Rel == "hub":
			return 0
		case l.Type == "text/html":
			return 3
		case l.Rel != "self":
			return 2
		default:
			return 1
		}
	}

	var bestlink atom.Link
	bestscore := -1
	for _, l := range links {
		score := getScore(l)
		if score > bestscore {
			bestlink = l
			bestscore = score
		}
	}

	return bestlink
}

func parseFix(f *Feed, ss []*Story) (*Feed, []*Story) {
	f.Checked = time.Now()
	//f.Image = loadImage(f)

	if u, err := url.Parse(f.Url); err == nil {
		if ul, err := u.Parse(f.Link); err == nil {
			f.Link = ul.String()
		}
	}
	base, err := url.Parse(f.Link)
	if err != nil {
		log.Printf("unable to parse link: %v", f.Link)
	}

	for _, s := range ss {
		//s.Parent = fk
		s.Created = f.Checked
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
				log.Printf("story has no id: %v", s)
				return nil, nil
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
				log.Printf("unable to resolve link: %v", s.Link)
			}
		}
		su, serr := url.Parse(s.Link)
		if serr != nil {
			su = &url.URL{}
			s.Link = ""
		}
		s.Content, s.Summary = Sanitize(s.Content, su)
	}

	return f, ss
}
