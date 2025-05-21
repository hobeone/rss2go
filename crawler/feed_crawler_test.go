package crawler

import (
	"fmt"
	"io/ioutil" // Added for ioutil.ReadAll
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	feedwatcher "github.com/hobeone/rss2go/feed_watcher"
	"github.com/sirupsen/logrus"
)

func TestFeedCrawler(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	ch := make(chan *feedwatcher.FeedCrawlRequest)
	rchan := make(chan *feedwatcher.FeedCrawlResponse)
	go FeedCrawler(ch, NewHTTPClient(defaultTimeout))

	req := &feedwatcher.FeedCrawlRequest{
		URI:          fmt.Sprintf("%s/%s", ts.URL, "ars.rss"),
		ResponseChan: rchan,
	}

	ch <- req
	resp := <-rchan
	if resp.URI != req.URI {
		t.Fatalf("Response URI differs from request.\n")
	}

	if resp.Error != nil {
		t.Fatalf("Response had an error when it shouldn't have: %s",
			resp.Error.Error())
	}
}

func TestGetFeed(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	resp, err := GetFeed(fmt.Sprintf("%s/%s", ts.URL, "ars.rss"), nil)
	if err != nil {
		t.Fatalf("Error getting feed: %s\n", err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatal("GetFeed should return an error when status != 200\n.")
	}

	resp, err = GetFeed(fmt.Sprintf("%s/%s", ts.URL, "error.rss"), nil)

	if err == nil {
		t.Fatalf("Should have gotten error for feed: %s\n", "error.rss")
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatal("GetFeed should return an error when status != 200\n.")
	}

	dialErrorClient := &HTTPClient{
		http.Client{
			Transport: &http.Transport{
				Dial: func(netw, addr string) (net.Conn, error) {
					return nil, fmt.Errorf("error connecting to host")
				},
			},
		},
	}

	_, err = GetFeed(fmt.Sprintf("%s/%s", ts.URL, "timeout"), dialErrorClient)
	if err == nil {
		t.Fatalf("Should have gotten timeout")
	}
}

func TestGetFeedAndMakeResponse(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	resp := GetFeedAndMakeResponse(fmt.Sprintf("%s/%s", ts.URL, "ars.rss"), nil)
	if resp.Error != nil {
		t.Fatalf("Error getting feed: %s\n", resp.Error.Error())
	}
	if resp.HTTPResponseStatusCode != http.StatusOK {
		t.Fatal("GetFeed should return an error when status != 200\n.")
	}

	resp = GetFeedAndMakeResponse(fmt.Sprintf("%s/%s", ts.URL, "error.rss"), nil)

	if resp.Error == nil {
		t.Fatalf("Should have gotten error for feed: %s\n", "error.rss")
	}
	if resp.HTTPResponseStatusCode != http.StatusInternalServerError {
		t.Fatalf("GetFeed should return an error when status != 200\n %v.", resp.HTTPResponseStatusCode)
	}

	dialErrorClient := &HTTPClient{
		http.Client{
			Transport: &http.Transport{
				Dial: func(netw, addr string) (net.Conn, error) {
					return nil, fmt.Errorf("error connecting to host")
				},
			},
		},
	}

	resp = GetFeedAndMakeResponse(fmt.Sprintf("%s/%s", ts.URL, "timeout"), dialErrorClient)
	if resp.Error == nil {
		t.Fatalf("Should have gotten timeout")
	}

	resp = GetFeedAndMakeResponse("http://testfeed", dialErrorClient)

	if resp.Error == nil {
		t.Fatalf("Should have returned an error on connect timeout")
	}

	resp = GetFeedAndMakeResponse(fmt.Sprintf("%s/%s", ts.URL, "ars.rss"), nil)
	if resp.Error != nil {
		t.Fatalf("Error getting feed: %s\n", resp.Error.Error())
	}
	bodyWithoutContentLength := string(resp.Body)

	resp = GetFeedAndMakeResponse(fmt.Sprintf("%s/%s", ts.URL, "ars_with_content_length.rss"), nil)
	if resp.Error != nil {
		t.Fatalf("Error getting feed: %s\n", resp.Error)
	}
	bodyWithContentLength := string(resp.Body)
	if bodyWithContentLength != bodyWithoutContentLength {
		t.Fatalf("Responses with and without Content-Length should get the same result")
	}

}

var fakeServerHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var content []byte
	// Default content type, can be overridden by cases
	w.Header().Set("Content-Type", "application/rss+xml")

	switch {
	case strings.HasSuffix(r.URL.Path, "ars.rss") || strings.HasSuffix(r.URL.Path, "/redirect_target"):
		feedResp, err := os.ReadFile("../testdata/ars.rss")
		if err != nil {
			logrus.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feedResp
	case strings.HasSuffix(r.URL.Path, "ars_with_content_length.rss"):
		feedResp, err := os.ReadFile("../testdata/ars.rss")
		if err != nil {
			logrus.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feedResp
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	case strings.HasSuffix(r.URL.Path, "error.rss"):
		http.Error(w, "Error request", http.StatusInternalServerError)
		return // http.Error already writes to w
	case strings.HasSuffix(r.URL.Path, "timeout"):
		time.Sleep(10 * time.Second)
		return // Let the client time out
	case r.URL.Path == "/redirect_301":
		http.Redirect(w, r, "/redirect_target", http.StatusMovedPermanently)
		return
	case r.URL.Path == "/redirect_302":
		http.Redirect(w, r, "/redirect_target", http.StatusFound)
		return
	case r.URL.Path == "/redirect_307":
		http.Redirect(w, r, "/redirect_target", http.StatusTemporaryRedirect)
		return
	case r.URL.Path == "/client_error_401":
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	case r.URL.Path == "/client_error_403":
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	case r.URL.Path == "/client_error_404":
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	case r.URL.Path == "/malformed_xml":
		content = []byte("<rss><channel><item>this is not closed properly")
		// Keep default Content-Type: application/rss+xml
	case r.URL.Path == "/wrong_content_type":
		feedResp, err := os.ReadFile("../testdata/ars.rss")
		if err != nil {
			logrus.Fatalf("Error reading test feed: %s", err.Error())
		}
		content = feedResp
		w.Header().Set("Content-Type", "text/plain")
	default:
		content = []byte("Default fake server response")
	}
	_, _ = w.Write([]byte(content))
})

func TestGetFeedExtendedScenarios(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	// Helper to read body for assertions
	// This helper seems unused or problematic, especially os.ReadFile(resp.Request.URL.Path)
	// The direct use of ioutil.ReadAll(resp.Body) in the test cases is preferred.
	// _ = readBody // Keep the linter happy for now, will use it or fix it.

	// Expected content for successful redirect
	expectedRedirectContent, err := ioutil.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatalf("Failed to read expected content file: %v", err)
	}

	tests := []struct {
		name                 string
		path                 string
		expectError          bool
		expectedStatusCode   int
		expectBodyContains   string // For malformed XML or specific error messages if any
		checkRedirectContent bool
	}{
		{
			name:                 "301 Redirect",
			path:                 "/redirect_301",
			expectError:          false,
			expectedStatusCode:   http.StatusOK, // Client should follow to a 200
			checkRedirectContent: true,
		},
		{
			name:                 "302 Redirect",
			path:                 "/redirect_302",
			expectError:          false,
			expectedStatusCode:   http.StatusOK, // Client should follow to a 200
			checkRedirectContent: true,
		},
		{
			name:               "401 Unauthorized",
			path:               "/client_error_401",
			expectError:        true,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "403 Forbidden",
			path:               "/client_error_403",
			expectError:        true,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "404 Not Found",
			path:               "/client_error_404",
			expectError:        true,
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:                 "Malformed XML",
			path:                 "/malformed_xml",
			expectError:          false, // GetFeed itself doesn't parse XML
			expectedStatusCode:   http.StatusOK,
			expectBodyContains:   "<rss><channel><item>this is not closed properly",
			checkRedirectContent: false,
		},
		{
			name:                 "Wrong Content Type",
			path:                 "/wrong_content_type",
			expectError:          false, // GetFeed itself doesn't validate Content-Type against body
			expectedStatusCode:   http.StatusOK,
			checkRedirectContent: true, // Body is valid XML from ars.rss
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := GetFeed(ts.URL+tt.path, nil)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}

			if resp == nil {
				t.Fatalf("Response is nil")
			}

			if resp.StatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, but got %d", tt.expectedStatusCode, resp.StatusCode)
			}

			if tt.checkRedirectContent || tt.expectBodyContains != "" {
				bodyBytes, readErr := ioutil.ReadAll(resp.Body) // Using ioutil.ReadAll
				if readErr != nil {
					t.Fatalf("Failed to read response body: %v", readErr)
				}
				bodyStr := string(bodyBytes)

				if tt.checkRedirectContent {
					if bodyStr != string(expectedRedirectContent) {
						t.Errorf("Expected body to match target content, but it didn't")
					}
				}
				if tt.expectBodyContains != "" {
					if !strings.Contains(bodyStr, tt.expectBodyContains) {
						t.Errorf("Expected body to contain '%s', but got '%s'", tt.expectBodyContains, bodyStr)
					}
				}
			}
		})
	}
}

func TestNewHTTPClientConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		inputTimeout    time.Duration
		expectedTimeout time.Duration
	}{
		{
			name:            "Zero Timeout",
			inputTimeout:    0,
			expectedTimeout: defaultTimeout,
		},
		{
			name:            "Negative Timeout",
			inputTimeout:    -1 * time.Second,
			expectedTimeout: defaultTimeout,
		},
		{
			name:            "Custom Timeout",
			inputTimeout:    15 * time.Second,
			expectedTimeout: 15 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewHTTPClient(tt.inputTimeout)

			if client.Timeout != tt.expectedTimeout {
				t.Errorf("Expected client.Timeout to be %v, but got %v", tt.expectedTimeout, client.Timeout)
			}

			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("client.Transport is not an *http.Transport")
			}

			if transport.DialContext == nil {
				t.Error("Expected transport.DialContext to be non-nil")
			}
			if transport.TLSHandshakeTimeout != defaultTLSHandshakeTimeout {
				t.Errorf("Expected transport.TLSHandshakeTimeout to be %v, but got %v", defaultTLSHandshakeTimeout, transport.TLSHandshakeTimeout)
			}
			if transport.MaxIdleConns <= 0 {
				t.Errorf("Expected transport.MaxIdleConns to be > 0, but got %d", transport.MaxIdleConns)
			}
			if transport.IdleConnTimeout <= 0 {
				t.Errorf("Expected transport.IdleConnTimeout to be > 0, but got %v", transport.IdleConnTimeout)
			}
			if transport.ExpectContinueTimeout <= 0 {
				t.Errorf("Expected transport.ExpectContinueTimeout to be > 0, but got %v", transport.ExpectContinueTimeout)
			}
		})
	}
}

func TestGetFeedAndMakeResponseExtendedScenarios(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(fakeServerHandler)
	defer ts.Close()

	expectedRedirectContent, err := os.ReadFile("../testdata/ars.rss")
	if err != nil {
		t.Fatalf("Failed to read expected content file: %v", err)
	}

	tests := []struct {
		name                 string
		path                 string
		expectError          bool
		expectedStatusCode   int
		expectBodyContains   string
		checkRedirectContent bool
	}{
		{
			name:                 "301 Redirect",
			path:                 "/redirect_301",
			expectError:          false,
			expectedStatusCode:   http.StatusOK,
			checkRedirectContent: true,
		},
		{
			name:                 "302 Redirect",
			path:                 "/redirect_302",
			expectError:          false,
			expectedStatusCode:   http.StatusOK,
			checkRedirectContent: true,
		},
		{
			name:               "401 Unauthorized",
			path:               "/client_error_401",
			expectError:        true,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "403 Forbidden",
			path:               "/client_error_403",
			expectError:        true,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "404 Not Found",
			path:               "/client_error_404",
			expectError:        true,
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:                 "Malformed XML",
			path:                 "/malformed_xml",
			expectError:          false,
			expectedStatusCode:   http.StatusOK,
			expectBodyContains:   "<rss><channel><item>this is not closed properly",
			checkRedirectContent: false,
		},
		{
			name:                 "Wrong Content Type",
			path:                 "/wrong_content_type",
			expectError:          false,
			expectedStatusCode:   http.StatusOK,
			checkRedirectContent: true, // Body is valid XML from ars.rss
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := GetFeedAndMakeResponse(ts.URL+tt.path, nil)

			if tt.expectError {
				if resp.Error == nil {
					t.Errorf("Expected an error, but got nil")
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected no error, but got: %v", resp.Error)
				}
			}

			if resp.HTTPResponseStatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, but got %d", tt.expectedStatusCode, resp.HTTPResponseStatusCode)
			}

			if tt.checkRedirectContent {
				if string(resp.Body) != string(expectedRedirectContent) {
					t.Errorf("Expected body to match target content, but it didn't")
				}
			}
			if tt.expectBodyContains != "" {
				if !strings.Contains(string(resp.Body), tt.expectBodyContains) {
					t.Errorf("Expected body to contain '%s', but got '%s'", tt.expectBodyContains, string(resp.Body))
				}
			}
		})
	}
}
