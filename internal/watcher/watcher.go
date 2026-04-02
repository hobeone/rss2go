package watcher

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hobeone/rss2go/internal/crawler"
	"github.com/hobeone/rss2go/internal/db"
	"github.com/hobeone/rss2go/internal/extractor"
	"github.com/hobeone/rss2go/internal/mailer"
	"github.com/hobeone/rss2go/internal/metrics"
	"github.com/hobeone/rss2go/internal/models"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

// CrawlerPool defines the interface for submitting crawl requests.
type CrawlerPool interface {
	Submit(crawler.CrawlRequest)
}

// MailerPool defines the interface for submitting mail requests.
type MailerPool interface {
	Submit(ctx context.Context, req mailer.MailRequest)
}

// Watcher handles crawl responses and email formatting for a single feed.
// Scheduling and goroutine lifecycle are managed by the Scheduler; Watcher
// itself is goroutine-free.
type Watcher struct {
	feed            models.Feed
	store           db.Store
	crawler         CrawlerPool
	mailer          MailerPool
	logger          *slog.Logger
	interval        time.Duration
	jitter          time.Duration
	strictPol       *bluemonday.Policy
	contentPol      *bluemonday.Policy
	maxImageWidth   int
	currentInterval time.Duration // backoff state; only touched by handleFeedResponse
	parser          *gofeed.Parser
	pendingItems    map[string]*gofeed.Item
	pendingItemsMu  sync.RWMutex
}

const (
	maxBackoff         = 24 * time.Hour
	MaxItemContentSize = 5 * 1024 * 1024
)

// New creates a new feed watcher.
func New(feed models.Feed, store db.Store, c CrawlerPool, m MailerPool, interval, jitter time.Duration, maxImageWidth int, logger *slog.Logger) *Watcher {

	// Strict policy for titles and subjects (no HTML)
	strictPol := bluemonday.StrictPolicy()

	// Let's build a strict safe HTML policy from scratch or modify UGC.
	// A simpler safe policy:
	contentPol := bluemonday.NewPolicy()
	contentPol.AllowURLSchemes("http", "https")
	contentPol.AllowAttrs("href").OnElements("a")
	contentPol.RequireNoReferrerOnLinks(true)
	contentPol.RequireParseableURLs(true)
	contentPol.AllowElements("b", "i", "u", "strong", "em", "p", "br", "div", "span", "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "img")
	contentPol.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	contentPol.AllowStyles("max-width").Matching(regexp.MustCompile(`(?i)^100%$`)).OnElements("img")
	contentPol.AllowStyles("height").Matching(regexp.MustCompile(`(?i)^auto$`)).OnElements("img")

	return &Watcher{
		feed:            feed,
		store:           store,
		crawler:         c,
		mailer:          m,
		logger:          logger.With("feed_id", feed.ID, "url", feed.URL),
		interval:        interval,
		jitter:          jitter,
		strictPol:       strictPol,
		contentPol:      contentPol,
		maxImageWidth:   maxImageWidth,
		currentInterval: interval,
		parser:          gofeed.NewParser(),
		pendingItems:    make(map[string]*gofeed.Item),
	}
}

// Feed returns the feed model associated with the watcher.
func (w *Watcher) Feed() models.Feed {
	return w.feed
}

// getUserEmails fetches the subscriber list for this feed and returns their
// email addresses. Pre-allocates the slice to avoid repeated appends.
func (w *Watcher) getUserEmails(ctx context.Context) ([]string, error) {
	users, err := w.store.GetUsersForFeed(ctx, w.feed.ID)
	if err != nil {
		return nil, err
	}
	emails := make([]string, len(users))
	for i, u := range users {
		emails[i] = u.Email
	}
	return emails, nil
}

func (w *Watcher) crawl(ctx context.Context) {
	w.logger.Debug("triggering crawl")
	w.crawler.Submit(crawler.CrawlRequest{
		FeedID:       w.feed.ID,
		URL:          w.feed.URL,
		Type:         crawler.RequestTypeFeed,
		Ctx:          ctx,
		ETag:         w.feed.ETag,
		LastModified: w.feed.LastModified,
	})
}

// HandleResponse processes a crawl result. For feed responses it returns the
// next poll interval and true; for item responses it returns (0, false).
// The Scheduler uses the returned interval to reschedule the feed.
func (w *Watcher) HandleResponse(ctx context.Context, resp crawler.CrawlResponse) (time.Duration, bool) {
	if resp.Type == crawler.RequestTypeFeed {
		return w.handleFeedResponse(ctx, resp), true
	}
	w.handleItemResponse(ctx, resp)
	return 0, false
}

// handleFeedResponse processes a feed crawl result and returns the next poll
// interval (normal interval on success, exponentially backed-off on error).
func (w *Watcher) handleFeedResponse(ctx context.Context, resp crawler.CrawlResponse) time.Duration {
	atomic.AddUint64(&metrics.FeedsCrawledTotal, 1)
	if resp.Error != nil {
		atomic.AddUint64(&metrics.FeedsCrawledErrors, 1)
		w.logger.Error("crawl failed", "error", resp.Error)

		snippet := ""
		if len(resp.Body) > 0 {
			maxLen := min(len(resp.Body), 500)
			snippet = string(resp.Body[:maxLen])
		} else {
			snippet = resp.Error.Error()
		}

		if err := w.store.SetFeedError(ctx, w.feed.ID, resp.StatusCode, snippet); err != nil {
			w.logger.Error("failed to update feed error in DB", "error", err)
		}

		if resp.RetryAfter > 0 {
			// Server told us exactly how long to wait; honour it.
			w.currentInterval = resp.RetryAfter
			w.logger.Warn("rate limited, honouring Retry-After", "retry_after", resp.RetryAfter)
		} else {
			w.currentInterval *= 2
			if w.currentInterval > maxBackoff {
				w.currentInterval = maxBackoff
			}
			w.logger.Warn("backing off due to error", "new_interval", w.currentInterval)
		}
		if err := w.store.UpdateFeedBackoff(ctx, w.feed.ID, time.Now().Add(w.currentInterval)); err != nil {
			w.logger.Error("failed to persist backoff", "error", err)
		}
		return w.currentInterval
	}

	w.currentInterval = w.interval
	if err := w.store.UpdateFeedBackoff(ctx, w.feed.ID, time.Time{}); err != nil {
		w.logger.Error("failed to clear backoff state", "error", err)
	}

	// Clear error in DB on success
	if err := w.store.ClearFeedError(ctx, w.feed.ID); err != nil {
		w.logger.Error("failed to clear feed error in DB", "error", err)
	}

	if resp.StatusCode == http.StatusNotModified {
		w.logger.Debug("feed not modified since last poll")
		if err := w.store.UpdateFeedLastPoll(ctx, w.feed.ID, w.feed.ETag, w.feed.LastModified); err != nil {
			w.logger.Error("failed to update last poll time", "error", err)
		}
		return w.currentInterval
	}

	w.feed.ETag = resp.ETag
	w.feed.LastModified = resp.LastModified

	feed, err := w.parser.Parse(bytes.NewReader(resp.Body))
	if err != nil {
		w.logger.Error("failed to parse feed", "error", err)
		return w.currentInterval
	}

	userEmails, err := w.getUserEmails(ctx)
	if err != nil {
		w.logger.Error("failed to get users for feed", "error", err)
		return w.currentInterval
	}

	if len(userEmails) == 0 {
		w.logger.Debug("no users subscribed to feed")
		return w.currentInterval
	}

	newItemsCount := 0
	for item := range feed.Items {
		itm := feed.Items[item]
		guid := itm.GUID
		if guid == "" {
			guid = itm.Link
		}

		seen, err := w.store.IsSeen(ctx, w.feed.ID, guid)
		if err != nil {
			w.logger.Error("failed to check if item is seen", "guid", guid, "error", err)
			continue
		}

		if seen {
			continue
		}

		if w.feed.FullArticle && itm.Link != "" {
			w.logger.Debug("scheduling full article extraction", "guid", guid, "url", itm.Link)
			w.pendingItemsMu.Lock()
			w.pendingItems[guid] = itm
			w.pendingItemsMu.Unlock()

			w.crawler.Submit(crawler.CrawlRequest{
				FeedID:   w.feed.ID,
				URL:      itm.Link,
				Type:     crawler.RequestTypeItem,
				ItemGUID: guid,
				Ctx:      ctx,
			})
		} else {
			subject, body := w.FormatItem(w.feed.Title, itm, "")

			w.logger.Info("new item found", "title", itm.Title, "guid", guid)
			w.mailer.Submit(ctx, mailer.MailRequest{
				To:      userEmails,
				Subject: subject,
				Body:    body,
			})

			if err := w.store.MarkSeen(ctx, w.feed.ID, guid); err != nil {
				w.logger.Error("failed to mark item as seen", "guid", guid, "error", err)
			}
		}
		newItemsCount++
	}

	if err := w.store.UpdateFeedLastPoll(ctx, w.feed.ID, w.feed.ETag, w.feed.LastModified); err != nil {
		w.logger.Error("failed to update last poll time", "error", err)
	}

	w.logger.Debug("crawl complete", "new_items", newItemsCount)
	return w.currentInterval
}

func (w *Watcher) handleItemResponse(ctx context.Context, resp crawler.CrawlResponse) {
	w.pendingItemsMu.Lock()
	itm, ok := w.pendingItems[resp.ItemGUID]
	delete(w.pendingItems, resp.ItemGUID)
	w.pendingItemsMu.Unlock()

	if !ok {
		w.logger.Warn("received item response for non-pending item", "guid", resp.ItemGUID)
		return
	}

	userEmails, err := w.getUserEmails(ctx)
	if err != nil {
		w.logger.Error("failed to get users for feed", "error", err)
		return
	}

	var extractedContent string
	if resp.Error == nil {
		ext, extErr := extractor.New(w.feed.ExtractionStrategy, w.feed.ExtractionConfig)
		if extErr != nil {
			w.logger.Warn("invalid extraction config, falling back to readability", "error", extErr)
			ext, _ = extractor.New(extractor.StrategyReadability, "")
		}
		extracted, err := ext.Extract(bytes.NewReader(resp.Body), itm.Link, 30*time.Second, w.logger)
		if err != nil {
			w.logger.Warn("failed to extract content", "url", itm.Link, "error", err)
		} else {
			extractedContent = extracted
		}
	} else {
		w.logger.Warn("failed to fetch item for extraction", "url", itm.Link, "error", resp.Error)
	}

	subject, body := w.FormatItem(w.feed.Title, itm, extractedContent)

	w.logger.Info("new item found (with full article)", "title", itm.Title, "guid", resp.ItemGUID)
	w.mailer.Submit(ctx, mailer.MailRequest{
		To:      userEmails,
		Subject: subject,
		Body:    body,
	})

	if err := w.store.MarkSeen(ctx, w.feed.ID, resp.ItemGUID); err != nil {
		w.logger.Error("failed to mark item as seen", "guid", resp.ItemGUID, "error", err)
	}
}

// FormatItem sanitizes and formats a feed item for an email.
func (w *Watcher) FormatItem(feedTitle string, item *gofeed.Item, contentOverride string) (subject, body string) {
	safeTitle := html.UnescapeString(strings.TrimSpace(w.strictPol.Sanitize(item.Title)))
	safeFeedTitle := html.UnescapeString(strings.TrimSpace(w.strictPol.Sanitize(feedTitle)))
	safeLink := strings.TrimSpace(w.strictPol.Sanitize(item.Link))

	// Use Content if available, otherwise Description.
	// Many feeds use Description for summary and Content for full text.
	content := contentOverride
	if content == "" {
		content = item.Content
		if content == "" {
			content = item.Description
		}
	}

	if len(content) > MaxItemContentSize {
		w.logger.Error("item content too large to process", "title", item.Title, "size", len(content), "limit", MaxItemContentSize)
		content = "<i>[Content omitted: item too large to process safely]</i>"
	} else {
		// Pre-process to clean content, remove trackers, and handle embedded elements
		content = cleanFeedContent(content, w.maxImageWidth)
	}

	safeContent := strings.TrimSpace(w.contentPol.Sanitize(content))

	subject = "[" + safeFeedTitle + "] " + safeTitle
	body = safeContent + "<br><br><a href=\"" + safeLink + "\">Read more</a>"
	return
}

func cleanFeedContent(htmlStr string, maxWidth int) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr // Fallback to returning original string if parsing fails
	}

	// Replace iframes with links
	doc.Find("iframe").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if exists && src != "" {
			replacement := fmt.Sprintf(`<a href="%s">[Embedded Content: %s]</a>`, html.EscapeString(src), html.EscapeString(src))
			s.ReplaceWithHtml(replacement)
		} else {
			s.Remove()
		}
	})

	// Remove feedsportal tracking links
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && strings.Contains(strings.ToLower(href), "da.feedsportal.com") {
			s.Remove()
		}
	})

	// Process images: remove tracking pixels, strip large dimensions, and add responsive styling
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		widthStr, _ := s.Attr("width")
		heightStr, _ := s.Attr("height")
		src, _ := s.Attr("src")

		// Remove if width or height is 1 or 0 (likely tracking pixel)
		if widthStr == "1" || widthStr == "0" || heightStr == "1" || heightStr == "0" {
			s.Remove()
			return
		}

		// Remove if URL contains common tracking keywords
		srcLower := strings.ToLower(src)
		if strings.Contains(srcLower, "tracker") || strings.Contains(srcLower, "pixel") || strings.Contains(srcLower, "analytics") {
			s.Remove()
			return
		}

		// Strip problematic attributes that can break email layout or are redundant
		s.RemoveAttr("srcset")
		s.RemoveAttr("sizes")
		s.RemoveAttr("decoding")
		s.RemoveAttr("fetchpriority")

		// Responsive sizing logic
		if maxWidth > 0 {
			w, wErr := strconv.Atoi(widthStr)
			h, hErr := strconv.Atoi(heightStr)

			// Define what we consider a "small image" (e.g., icons, buttons)
			// Small images should not have max-width: 100% forced on them
			isSmall := false
			if wErr == nil && hErr == nil && w <= 150 && h <= 150 {
				isSmall = true
			}

			if !isSmall {
				// Strip width/height if they exceed maxWidth
				if wErr == nil && w > maxWidth {
					s.RemoveAttr("width")
					s.RemoveAttr("height")
				} else if hErr == nil && h > maxWidth {
					s.RemoveAttr("width")
					s.RemoveAttr("height")
				}

				// Add responsive styling to everything except small images.
				// This includes images with no dimensions (which might be large)
				// and medium images (to ensure they fit on mobile).
				s.SetAttr("style", "max-width: 100%; height: auto;")
			}
		}
	})

	htmlStr, _ = doc.Find("body").Html()
	return htmlStr
}

func (w *Watcher) getJitter() time.Duration {
	if w.jitter == 0 {
		return 0
	}
	// #nosec G404 - cryptographic security not required for polling jitter
	return time.Duration(rand.Int64N(int64(w.jitter)))
}
