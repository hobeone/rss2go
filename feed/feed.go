// Mostly just lifted straight from Matt Jibson's goread app
// github.com/mjibson/goread

package feed

import (
	"bytes"
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"encoding/xml"
	"fmt"
	"github.com/hobeone/rss2go/atom"
	"github.com/hobeone/rss2go/rdf"
	"github.com/hobeone/rss2go/rss"
	"github.com/moovweb/gokogiri"
	"html"
	"log"
	"net/url"
	"strings"
	"time"
)

type Feed struct {
	Url        string
	Title      string
	Updated    time.Time
	NextUpdate time.Time
	Link       string
	Checked    time.Time
	Image      string
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
	ParentFeed   *Feed

	Content string
}

func ParseFeed(u string, b []byte) (*Feed, []*Story, error) {
	f := Feed{Url: u}
	s := []*Story{}


	tr, err := charset.TranslatorTo("utf-8")
	if err != nil {
		return nil, nil, err
	}
	_, b, err = tr.Translate(b, true)
	if err != nil {
		return nil, nil, err
	}

	doc, err := gokogiri.ParseXml(b)
	if err != nil {
		return nil, nil, err
	}

	b = []byte(doc.String())

	a := atom.Feed{}
	var atomerr, rsserr, rdferr error
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
				Id:         i.ID,
				Title:      i.Title,
				ParentFeed: &f,
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
				Link:       i.Link,
				Author:     i.Author,
				ParentFeed: &f,
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
				Id:         i.About,
				Title:      i.Title,
				Link:       i.Link,
				Author:     i.Creator,
				ParentFeed: &f,
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
	log.Printf("rss parse error: %s", rsserr.Error())
	log.Printf("rdf parse error: %s", rdferr.Error())
	return nil, nil, atomerr
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

func parseFix(f *Feed, ss []*Story) (*Feed, []*Story, error) {
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

	return f, ss, nil
}
