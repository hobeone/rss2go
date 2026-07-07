package sanitizer

import (
	"strings"
	"testing"
)

func TestSanitizeXSSPrevention(t *testing.T) {
	s := NewSanitizer(800)
	html := `<div><script>alert("xss")</script><p onload="hack()">Hello</p></div>`

	res, err := s.Sanitize(html, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(res, "<script>") || strings.Contains(res, "onload") {
		t.Errorf("XSS vulnerabilities not stripped: %s", res)
	}
	if !strings.Contains(res, "Hello") {
		t.Errorf("main body content stripped incorrectly: %s", res)
	}
}

func TestSanitizeRelativeURLResolution(t *testing.T) {
	s := NewSanitizer(800)
	html := `
	<div>
		<a href="/relative/page.html">Link</a>
		<img src="relative/image.png" />
		<a href="https://other.com/absolute">Absolute Link</a>
	</div>`

	res, err := s.Sanitize(html, "https://example.com/subpath/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(res, `href="https://example.com/relative/page.html"`) {
		t.Errorf("relative root URL not resolved: %s", res)
	}
	if !strings.Contains(res, `src="https://example.com/subpath/relative/image.png"`) {
		t.Errorf("relative path URL not resolved: %s", res)
	}
	if !strings.Contains(res, `href="https://other.com/absolute"`) {
		t.Errorf("absolute URL mutated incorrectly: %s", res)
	}
	if !strings.Contains(res, `target="_blank"`) {
		t.Errorf("expected target=\"_blank\" on links: %s", res)
	}
	if !strings.Contains(res, `rel="noopener noreferrer nofollow"`) {
		t.Errorf("expected rel=\"noopener noreferrer nofollow\" on links: %s", res)
	}
}

func TestSanitizeImageScaling(t *testing.T) {
	s := NewSanitizer(800)
	html := `
	<div>
		<!-- Large Image -->
		<img class="large" src="large.jpg" width="1200" height="800" srcset="abc" sizes="100vw" decoding="async" />
		<!-- Small Icon -->
		<img class="icon" src="icon.png" width="32" height="32" />
		<!-- Exact Max Width Image -->
		<img class="exact" src="exact.jpg" width="800" height="400" />
		<!-- No Dimensions -->
		<img class="raw" src="raw.jpg" />
	</div>`

	res, err := s.Sanitize(html, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Large image should be capped at 800 width, responsive height, and layout attrs stripped
	if !strings.Contains(res, `width="800"`) {
		t.Error("large image width not updated to 800")
	}
	if !strings.Contains(res, `style="max-width: 100%; height: auto; width: 800px;"`) {
		t.Errorf("large image style not set correctly, got: %s", res)
	}
	if strings.Contains(res, "srcset") || strings.Contains(res, "sizes") || strings.Contains(res, "decoding") {
		t.Error("large image layout attributes were not stripped")
	}
	if strings.Contains(res, `height="800"`) {
		t.Error("large image height attribute was not removed")
	}

	// 2. Small image should preserve its original width & height attributes
	if !strings.Contains(res, `width="32"`) || !strings.Contains(res, `height="32"`) {
		t.Error("small image attributes were modified")
	}
	if !strings.Contains(res, `style="max-width: 100%; height: auto; width: 32px;"`) {
		t.Errorf("small image style not set correctly, got: %s", res)
	}

	// 3. Exact max-width image should preserve original dimensions (including height)
	if !strings.Contains(res, `width="800"`) || !strings.Contains(res, `height="400"`) {
		t.Error("exact max-width image attributes were modified")
	}
	if !strings.Contains(res, `style="max-width: 100%; height: auto; width: 800px;"`) {
		t.Errorf("exact image style not set correctly, got: %s", res)
	}

	// 4. Raw image (no width attribute) should fallback to max-width styling
	if !strings.Contains(res, `style="max-width: 100%; height: auto; max-width: 800px;"`) {
		t.Errorf("raw image style not set correctly, got: %s", res)
	}
}

func TestSanitizeTrackerBlocking(t *testing.T) {
	s := NewSanitizer(800)
	html := `
	<div>
		<p>Start</p>
		<img src="tracking.jpg" width="1" height="1" />
		<img src="https://google-analytics.com/collect" />
		<img src="https://other.com/pixel.gif" />
		<img src="legit.png" width="200" height="200" />
	</div>`

	res, err := s.Sanitize(html, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(res, "Start") {
		t.Fatal("content stripped incorrectly")
	}
	if strings.Contains(res, "tracking.jpg") {
		t.Error("1x1 tracking pixel not stripped")
	}
	if strings.Contains(res, "google-analytics.com") {
		t.Error("Google Analytics pixel not stripped")
	}
	if strings.Contains(res, "pixel.gif") {
		t.Error("tracking keyword pixel not stripped")
	}
	if !strings.Contains(res, "legit.png") {
		t.Error("legitimate image stripped incorrectly")
	}
}

func TestSanitizeEmbedFallbacks(t *testing.T) {
	s := NewSanitizer(800)
	html := `
	<div>
		<iframe src="/video/embed/123" width="560" height="315"></iframe>
		<object data="http://legacy.swf"></object>
	</div>`

	res, err := s.Sanitize(html, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(res, "iframe") || strings.Contains(res, "object") {
		t.Error("embed tags (iframe, object) were not stripped")
	}
	if !strings.Contains(res, `href="https://example.com/video/embed/123"`) {
		t.Errorf("iframe absolute fallback link not generated: %s", res)
	}
	if !strings.Contains(res, `href="http://legacy.swf"`) {
		t.Errorf("object absolute fallback link not generated: %s", res)
	}
}

func TestSanitizeEmptyBody(t *testing.T) {
	s := NewSanitizer(800)
	res, err := s.Sanitize("   ", "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "" {
		t.Errorf("expected empty string for blank input, got %q", res)
	}
}

func TestSanitizerDefaultWidthFallback(t *testing.T) {
	// Zero width
	sZero := NewSanitizer(0)
	if sZero.maxImageWidth != 1200 {
		t.Errorf("expected default fallback 800, got %d", sZero.maxImageWidth)
	}

	// Negative width
	sNeg := NewSanitizer(-50)
	if sNeg.maxImageWidth != 1200 {
		t.Errorf("expected default fallback 800, got %d", sNeg.maxImageWidth)
	}
}
