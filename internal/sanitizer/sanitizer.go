package sanitizer

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
)

// Sanitizer processes feed item HTML body content to ensure it is secure, responsive,
// and stripped of tracking/analytics widgets.
type Sanitizer struct {
	maxImageWidth int
	policy        *bluemonday.Policy
}

// NewSanitizer creates a new Sanitizer instance with custom max image width limit.
func NewSanitizer(maxImageWidth int) *Sanitizer {
	if maxImageWidth <= 0 {
		maxImageWidth = 1200
	}

	// Build custom UGC policy that permits inline image styles, fallback classes, target, and rel attributes on links
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("style").OnElements("img")
	p.AllowAttrs("class", "target", "rel").OnElements("a")
	p.AllowAttrs("class").OnElements("p")
	p.AllowAttrs("width", "height").OnElements("img")

	return &Sanitizer{
		maxImageWidth: maxImageWidth,
		policy:        p,
	}
}

// Sanitize cleanses HTML, resolves relative URLs, block-formats images, removes trackers,
// and replaces unsupported frames/embeds with text fallback links.
func (s *Sanitizer) Sanitize(htmlBody string, siteURL string) (string, error) {
	if strings.TrimSpace(htmlBody) == "" {
		return "", nil
	}

	// 1. Parse base URL for reference resolution
	base, err := url.Parse(siteURL)
	if err != nil {
		return "", fmt.Errorf("sanitizer: parse base URL: %w", err)
	}

	// 2. Parse HTML using goquery to run DOM transformations
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return "", fmt.Errorf("sanitizer: parse HTML: %w", err)
	}

	// 3. Resolve Relative to Absolute URLs (links & images) and enforce secure link target/rel attributes
	doc.Find("a").Each(func(i int, sel *goquery.Selection) {
		if href, exists := sel.Attr("href"); exists {
			if absHref := resolveURL(base, href); absHref != "" {
				sel.SetAttr("href", absHref)
			}
		}
		sel.SetAttr("target", "_blank")
		sel.SetAttr("rel", "noopener noreferrer nofollow")
	})

	// 4. Process Images (Tracker blocking, URL resolution, responsive inline styling)
	doc.Find("img").Each(func(i int, sel *goquery.Selection) {
		src, exists := sel.Attr("src")
		if !exists {
			sel.Remove()
			return
		}

		// Resolve relative image URLs
		if absSrc := resolveURL(base, src); absSrc != "" {
			sel.SetAttr("src", absSrc)
			src = absSrc
		}

		// Block tracking pixels & analytics images
		widthStr := sel.AttrOr("width", "")
		heightStr := sel.AttrOr("height", "")
		if isTracker(src, widthStr, heightStr) {
			sel.Remove()
			return
		}

		// Protect layouts: strip layout-breaking attributes
		sel.RemoveAttr("srcset")
		sel.RemoveAttr("sizes")
		sel.RemoveAttr("decoding")

		// Apply inline styles to cap large images and preserve icons
		w := -1
		if widthStr != "" {
			if parsedW, err := strconv.Atoi(widthStr); err == nil {
				w = parsedW
			}
		}

		if w > 0 {
			if w > s.maxImageWidth {
				// Large image: scale down to cap limit
				sel.SetAttr("style", fmt.Sprintf("max-width: 100%%; height: auto; width: %dpx;", s.maxImageWidth))
				sel.SetAttr("width", strconv.Itoa(s.maxImageWidth))
				sel.RemoveAttr("height")
			} else {
				// Small image (e.g. icon): preserve original dimensions
				sel.SetAttr("style", fmt.Sprintf("max-width: 100%%; height: auto; width: %dpx;", w))
			}
		} else {
			// Unknown/percentage width: enforce responsive scaling
			sel.SetAttr("style", fmt.Sprintf("max-width: 100%%; height: auto; max-width: %dpx;", s.maxImageWidth))
		}
	})

	// 5. Replace Unsupported embeds (iframe, embed, object) with text links
	doc.Find("iframe, embed, object").Each(func(i int, sel *goquery.Selection) {
		src := sel.AttrOr("src", sel.AttrOr("data", ""))
		if src == "" {
			sel.Remove()
			return
		}

		absSrc := resolveURL(base, src)
		if absSrc == "" {
			absSrc = src
		}

		// Replace tag with fallback paragraph containing absolute text link
		fallbackText := fmt.Sprintf(`<p class="rss2go-embed-fallback"><a href="%s" target="_blank">View Embedded Content</a></p>`, absSrc)
		sel.ReplaceWithHtml(fallbackText)
	})

	// 6. Convert modified DOM back to HTML string
	processedHTML, err := doc.Find("body").Html()
	if err != nil {
		return "", fmt.Errorf("sanitizer: render body: %w", err)
	}

	// 7. Apply XSS Sanitization (bluemonday) on processed body HTML
	sanitizedHTML := s.policy.Sanitize(processedHTML)

	return sanitizedHTML, nil
}

// resolveURL converts relative paths to absolute URLs using the base context.
func resolveURL(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}

// isTracker checks if an image is likely a tracking pixel based on size or domain.
func isTracker(src, width, height string) bool {
	// Match tiny dimensions
	if width == "0" || height == "0" || width == "1" || height == "1" {
		return true
	}

	srcLower := strings.ToLower(src)

	// Known analytics/tracking domain keywords
	trackers := []string{
		"doubleclick",
		"google-analytics",
		"statcounter",
		"piwik",
		"openstat",
		"/open/",
		"/track/",
		"/pixel",
		"tracking-pixel",
		"feedsportal.com",
		"pheedo.com",
	}

	for _, keyword := range trackers {
		if strings.Contains(srcLower, keyword) {
			return true
		}
	}

	return false
}
