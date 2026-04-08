package watcher

import (
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

// Formatter sanitizes and formats feed items for email delivery.
// It holds only the stateless config needed for formatting — no store,
// crawler, or mailer dependencies.
type Formatter struct {
	strictPol     *bluemonday.Policy
	contentPol    *bluemonday.Policy
	maxImageWidth int
	logger        *slog.Logger
}

// NewFormatter constructs a Formatter with the given image-width cap and logger.
func NewFormatter(maxImageWidth int, logger *slog.Logger) *Formatter {
	strictPol := bluemonday.StrictPolicy()

	contentPol := bluemonday.NewPolicy()
	contentPol.AllowURLSchemes("http", "https")
	contentPol.AllowAttrs("href").OnElements("a")
	contentPol.RequireNoReferrerOnLinks(true)
	contentPol.RequireParseableURLs(true)
	contentPol.AllowElements("b", "i", "u", "strong", "em", "p", "br", "div", "span", "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "img")
	contentPol.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	contentPol.AllowStyles("max-width").Matching(regexp.MustCompile(`(?i)^100%$`)).OnElements("img")
	contentPol.AllowStyles("height").Matching(regexp.MustCompile(`(?i)^auto$`)).OnElements("img")

	return &Formatter{
		strictPol:     strictPol,
		contentPol:    contentPol,
		maxImageWidth: maxImageWidth,
		logger:        logger,
	}
}

// FormatItem sanitizes and formats a feed item into an email subject and body.
// contentOverride, when non-empty, replaces the item's own content/description
// (used when a full article was separately fetched).
func (f *Formatter) FormatItem(feedTitle string, item *gofeed.Item, contentOverride string) (subject, body string) {
	safeTitle := html.UnescapeString(strings.TrimSpace(f.strictPol.Sanitize(item.Title)))
	safeFeedTitle := html.UnescapeString(strings.TrimSpace(f.strictPol.Sanitize(feedTitle)))
	safeLink := strings.TrimSpace(f.strictPol.Sanitize(item.Link))

	content := contentOverride
	if content == "" {
		content = item.Content
		if content == "" {
			content = item.Description
		}
	}

	if len(content) > MaxItemContentSize {
		f.logger.Error("item content too large to process", "title", item.Title, "size", len(content), "limit", MaxItemContentSize)
		content = "<i>[Content omitted: item too large to process safely]</i>"
	} else {
		content = cleanFeedContent(content, f.maxImageWidth)
	}

	safeContent := strings.TrimSpace(f.contentPol.Sanitize(content))

	subject = "[" + safeFeedTitle + "] " + safeTitle
	body = safeContent + "<br><br><a href=\"" + safeLink + "\">Read more</a>"
	return
}

// cleanFeedContent pre-processes raw HTML from a feed item: replaces iframes
// with links, removes tracking pixels and feedsportal links, and applies
// responsive image sizing.
func cleanFeedContent(htmlStr string, maxWidth int) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	// Replace iframes with links.
	doc.Find("iframe").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if exists && src != "" {
			replacement := fmt.Sprintf(`<a href="%s">[Embedded Content: %s]</a>`, html.EscapeString(src), html.EscapeString(src))
			s.ReplaceWithHtml(replacement)
		} else {
			s.Remove()
		}
	})

	// Remove feedsportal tracking links.
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && strings.Contains(strings.ToLower(href), "da.feedsportal.com") {
			s.Remove()
		}
	})

	// Process images: remove tracking pixels, strip large dimensions, add responsive styling.
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		widthStr, _ := s.Attr("width")
		heightStr, _ := s.Attr("height")
		src, _ := s.Attr("src")

		// Tracking pixel — remove entirely.
		if widthStr == "1" || widthStr == "0" || heightStr == "1" || heightStr == "0" {
			s.Remove()
			return
		}

		// URL-based tracking signal — remove entirely.
		srcLower := strings.ToLower(src)
		if strings.Contains(srcLower, "tracker") || strings.Contains(srcLower, "pixel") || strings.Contains(srcLower, "analytics") {
			s.Remove()
			return
		}

		// Strip attributes that break email layout or are redundant in email clients.
		s.RemoveAttr("srcset")
		s.RemoveAttr("sizes")
		s.RemoveAttr("decoding")
		s.RemoveAttr("fetchpriority")

		if maxWidth <= 0 {
			return
		}

		w, wErr := strconv.Atoi(widthStr)
		h, hErr := strconv.Atoi(heightStr)

		// Small images (icons, buttons) keep their original dimensions.
		isSmall := wErr == nil && hErr == nil && w <= 150 && h <= 150
		if isSmall {
			return
		}

		// Strip explicit dimensions if they exceed the cap.
		if wErr == nil && w > maxWidth {
			s.RemoveAttr("width")
			s.RemoveAttr("height")
		} else if hErr == nil && h > maxWidth {
			s.RemoveAttr("width")
			s.RemoveAttr("height")
		}

		// Responsive style for all non-small images (including unknown-size ones).
		s.SetAttr("style", "max-width: 100%; height: auto;")
	})

	htmlStr, _ = doc.Find("body").Html()
	return htmlStr
}
