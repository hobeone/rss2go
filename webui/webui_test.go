package webui

import (
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTest(t *testing.T) (*db.DbDispatcher, *martini.ClassicMartini) {
	feeds := make(map[string]*feed_watcher.FeedWatcher)
	dbh := db.NewMemoryDbDispatcher(false, true)
	m := createMartini(dbh, feeds)
	return dbh, m
}

func TestUserPage(t *testing.T) {
	dbh, m := setupTest(t)
	response := httptest.NewRecorder()

	user, err := dbh.AddUser("test", "test@test.com")
	if err != nil {
		t.Fatalf("Couldn't create user: %s", err)
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("/api/1/user/%s", user.Email), nil)
	if err != nil {
		t.Fatalf("Error creating request: %s", err)
	}

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}
}

func TestFeedsPage(t *testing.T) {
	feeds := make(map[string]*feed_watcher.FeedWatcher)
	feeds["http://test/url"] = &feed_watcher.FeedWatcher{
		FeedInfo: db.FeedInfo{
			Name: "testfeed",
			Url:  "http://url/feed",
		},
	}
	dbh := db.NewMemoryDbDispatcher(false, true)
	m := createMartini(dbh, feeds)

	response := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/1/feeds", nil)
	if err != nil {
		t.Fatalf("Error creating request: %s", err)
	}

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "http://test/url") {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected to find feed_list in reponse body")
	}
}

func TestAddFeed(t *testing.T) {
	_, m := setupTest(t)
	response := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/api/1/feeds",
		strings.NewReader("name=test_feed&url=http://test_feed_url"))
	req.Header.Set("Content-Type",
		"application/x-www-form-urlencoded; param=value")

	if err != nil {
		t.Fatalf("Error creating request: %s", err)
	}

	m.ServeHTTP(response, req)

	if response.Code != 201 {
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "http://test_feed_url") {
		t.Fatalf("Expected to find feed_list in reponse body")
	}
}

func TestGetFeed(t *testing.T) {
	dbh, m := setupTest(t)
	feed, err := dbh.AddFeed("test", "http://feeds/url.atom")
	if err != nil {
		t.Fatalf("Error creating new feed: %s", err)
	}

	response := httptest.NewRecorder()
	req, err := http.NewRequest("GET", fmt.Sprintf("/api/1/feeds/%d", feed.Id), nil)
	if err != nil {
		t.Fatalf("Error creating request: %s", err)
	}

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("GetFeedd Expected 200 response code, got %d", response.Code)
	}
}

func TestSubscribeFeed(t *testing.T) {
	dbh, m := setupTest(t)
	feed, err := dbh.AddFeed("test", "http://feeds/url.atom")
	if err != nil {
		t.Fatalf("Error creating new feed: %s", err)
	}
	user, err := dbh.AddUser("testuser", "test@test.com")
	if err != nil {
		t.Fatalf("Error creating new user: %s", err)
	}

	response := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/api/1/user/subscribe",
		strings.NewReader(fmt.Sprintf("useremail=%s&url=%s",
			user.Email, feed.Url)))
	if err != nil {
		t.Fatalf("Error creating request: %s", err)
	}
	req.Header.Set("Content-Type",
		"application/x-www-form-urlencoded; param=value")

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("GetFeedd Expected 200 response code, got %d", response.Code)
	}
}
