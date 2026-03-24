package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gorilla/feeds"
	dateparser "github.com/markusmobius/go-dateparser"
)

func parseItem(s *goquery.Selection) *feeds.Item {
	name := strings.TrimSpace(s.Find("h3").Text())
	name = strings.Join(strings.Fields(name), " ")

	link := s.Find("a").AttrOr(`href`, ``)
	img := s.Find("img").AttrOr(`data-src`, ``)
	published := s.Find("span[data-post-time]").Text()

	item := feeds.Item{}
	item.Title = name
	item.Link = &feeds.Link{Href: link}
	item.Content = img
	item.Id = link

	dt, err := dateparser.Parse(nil, published)
	if err != nil {
		item.Created = time.Now()
	} else {
		item.Created = dt.Time
	}

	item.Content = fmt.Sprintf(`<a href="%s">%s</a><br /><img src="%s">`, item.Link.Href, item.Title, item.Content)

	return &item
}

// escapecollective handles the /escapecollective path
func escapecollective(w http.ResponseWriter, r *http.Request) {
	url := "https://escapecollective.com/stories/"
	log.Println("Handling /escapecollective")
	
	res, err := http.Get(url)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to fetch document: %s\n", err)
		return
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != 200 {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "HTTP Error %d: %s", res.StatusCode, res.Status)
		return
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to parse document: %s", err)
		return
	}

	f := &feeds.Feed{
		Title: "EscapeCollective",
		Link:  &feeds.Link{Href: "https://escapecollective.com/stories/"},
	}

	doc.Find("div.post").Each(func(i int, s *goquery.Selection) {
		item := parseItem(s)
		f.Items = append(f.Items, item)
	})

	rss, err := f.ToRss()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error converting to RSS feed: %s", err)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml")
	fmt.Fprintln(w, rss)
}

func main() {
	http.HandleFunc("/escapecollective", escapecollective)

	log.Println("Scraper listening on :8282...")
	server := &http.Server{
		Addr:         ":8282",
		Handler:      nil,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
