package sidecar

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Server wraps the scraper sidecar HTTP routing.
type Server struct {
	client     *http.Client
	addr       string
	httpServer *http.Server
	log        *slog.Logger
}

// NewServer creates a new sidecar scraper HTTP server.
// If client is nil, http.DefaultClient is used.
func NewServer(addr string, client *http.Client, log *slog.Logger) *Server {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	if log == nil {
		log = slog.Default().With("component", "sidecar")
	}
	return &Server{
		client: client,
		addr:   addr,
		log:    log,
	}
}

// Start launches the sidecar server and blocks until context is cancelled or Stop is called.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /scrape", s.handleScrape)

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	s.log.Info("Starting Scraper Sidecar Server", "addr", s.addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("sidecar: listen and serve failed: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the sidecar server.
func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
}

func (s *Server) handleScrape(w http.ResponseWriter, r *http.Request) {
	targetURLStr := r.URL.Query().Get("url")
	itemSelector := r.URL.Query().Get("item")
	titleSelector := r.URL.Query().Get("title")
	linkSelector := r.URL.Query().Get("link")
	descSelector := r.URL.Query().Get("description")

	if targetURLStr == "" || itemSelector == "" || titleSelector == "" || linkSelector == "" {
		http.Error(w, "Missing required query parameters: url, item, title, link", http.StatusBadRequest)
		return
	}

	targetURL, err := url.Parse(targetURLStr)
	if err != nil || targetURL.Scheme == "" || targetURL.Host == "" {
		http.Error(w, "Invalid target url parameter", http.StatusBadRequest)
		return
	}

	// Fetch target HTML page
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL.String(), nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to build request: %v", err), http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "rss2go-scraper-sidecar/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch target page: %v", err), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Target server returned status %d %s", resp.StatusCode, resp.Status), http.StatusBadGateway)
		return
	}

	// Parse HTML Document
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse target HTML: %v", err), http.StatusInternalServerError)
		return
	}

	// Scrape repeating elements
	var feedItems []rssItem
	doc.Find(itemSelector).Each(func(i int, sel *goquery.Selection) {
		title := strings.TrimSpace(sel.Find(titleSelector).First().Text())
		if title == "" {
			return
		}

		linkSel := sel.Find(linkSelector).First()
		link, exists := linkSel.Attr("href")
		if !exists || strings.TrimSpace(link) == "" {
			return
		}
		link = strings.TrimSpace(link)

		// Resolve relative URL
		parsedLink, err := url.Parse(link)
		if err == nil {
			link = targetURL.ResolveReference(parsedLink).String()
		}

		description := ""
		if descSelector != "" {
			description = strings.TrimSpace(sel.Find(descSelector).First().Text())
		}

		feedItems = append(feedItems, rssItem{
			Title:       title,
			Link:        link,
			Description: description,
			GUID:        link,
			PubDate:     time.Now().Format(time.RFC1123Z),
		})
	})

	// Render output RSS XML
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = io.WriteString(w, xmlHeader)
	_, _ = fmt.Fprintf(w, "<rss version=\"2.0\">\n  <channel>\n")
	_, _ = fmt.Fprintf(w, "    <title>%s</title>\n", html.EscapeString(targetURL.Host+" Scraped Feed"))
	_, _ = fmt.Fprintf(w, "    <link>%s</link>\n", html.EscapeString(targetURL.String()))
	_, _ = fmt.Fprintf(w, "    <description>Dynamically generated scraped RSS feed for %s</description>\n", html.EscapeString(targetURL.String()))

	for _, item := range feedItems {
		_, _ = io.WriteString(w, "    <item>\n")
		_, _ = fmt.Fprintf(w, "      <title>%s</title>\n", html.EscapeString(item.Title))
		_, _ = fmt.Fprintf(w, "      <link>%s</link>\n", html.EscapeString(item.Link))
		_, _ = fmt.Fprintf(w, "      <guid>%s</guid>\n", html.EscapeString(item.GUID))
		if item.Description != "" {
			_, _ = fmt.Fprintf(w, "      <description>%s</description>\n", html.EscapeString(item.Description))
		}
		_, _ = fmt.Fprintf(w, "      <pubDate>%s</pubDate>\n", item.PubDate)
		_, _ = io.WriteString(w, "    </item>\n")
	}

	_, _ = io.WriteString(w, "  </channel>\n</rss>\n")
}

type rssItem struct {
	Title       string
	Link        string
	Description string
	GUID        string
	PubDate     string
}

const xmlHeader = `<?xml version="1.0" encoding="utf-8"?>
`
