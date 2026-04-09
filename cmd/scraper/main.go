package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gorilla/feeds"
	"github.com/hobeone/rss2go/internal/version"
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
	item.Id = link

	dt, err := dateparser.Parse(nil, published)
	if err != nil {
		item.Created = time.Now()
	} else {
		item.Created = dt.Time
	}

	item.Content = fmt.Sprintf(`<a href="%s">%s</a><br /><img src="%s">`, item.Link.Href, item.Title, img)

	return &item
}

// escapecollective handles the /escapecollective path.
func escapecollective(targetURL, selector string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Info("handling request", "path", r.URL.Path)

		// #nosec G107 - URL is operator-supplied via flag, not user input
		res, err := http.Get(targetURL)
		if err != nil {
			logger.Error("failed to fetch source page", "url", targetURL, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to fetch document: %s\n", err)
			return
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != 200 {
			logger.Error("unexpected status from source page", "url", targetURL, "status", res.StatusCode)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "HTTP Error %d: %s", res.StatusCode, res.Status)
			return
		}

		doc, err := goquery.NewDocumentFromReader(res.Body)
		if err != nil {
			logger.Error("failed to parse source page", "url", targetURL, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to parse document: %s", err)
			return
		}

		f := &feeds.Feed{
			Title: "EscapeCollective",
			Link:  &feeds.Link{Href: targetURL},
		}

		doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
			item := parseItem(s)
			f.Items = append(f.Items, item)
		})

		rss, err := f.ToRss()
		if err != nil {
			logger.Error("failed to generate RSS", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error converting to RSS feed: %s", err)
			return
		}

		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprintln(w, rss)
	}
}

func run() error {
	addr := flag.String("addr", ":8282", "address to listen on")
	targetURL := flag.String("url", "https://escapecollective.com/stories/", "URL of the page to scrape")
	selector := flag.String("selector", "div.post", "CSS selector for feed item elements")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	logger.Info("starting scraper", "version", version.Info(), "addr", *addr, "url", *targetURL, "selector", *selector)

	mux := http.NewServeMux()
	mux.HandleFunc("/escapecollective", escapecollective(*targetURL, *selector, logger))

	server := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logger.Info("scraper listening", "addr", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
