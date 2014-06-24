package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hobeone/rss2go/db"
	"github.com/stretchr/testify/assert"
)

const getAllFeedGoldenOutput = `{
  "feeds": [
    {
      "id": 1,
      "name": "test_feed1",
      "url": "http://testfeed1/feed.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    },
    {
      "id": 2,
      "name": "test_feed2",
      "url": "http://testfeed2/feed.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    },
    {
      "id": 3,
      "name": "test_feed3",
      "url": "http://testfeed3/feed.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    }
  ]
}`

func TestGetAllFeeds(t *testing.T) {
	dbh, m := setupTest(t)
	loadFixtures(t, dbh)
	response := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/1/feeds", nil)
	if err != nil {
		t.Fatalf("Error creating request: %s", err)
	}

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}
	if response.Body.String() != getAllFeedGoldenOutput {
		t.Fatalf("Response didn't match golden response:\n%s\n---vs---\n%s", response.Body.String(), getAllFeedGoldenOutput)
	}
}

const getSomeFeedsGoldenResponse = `{
  "feeds": [
    {
      "id": 1,
      "name": "test_feed1",
      "url": "http://testfeed1/feed.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    },
    {
      "id": 2,
      "name": "test_feed2",
      "url": "http://testfeed2/feed.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    }
  ]
}`

func TestGetSomeFeeds(t *testing.T) {
	dbh, m := setupTest(t)
	loadFixtures(t, dbh)
	response := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/1/feeds?ids[]=1&ids[]=2", nil)
	assert.Nil(t, err)

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}
	if response.Body.String() != getSomeFeedsGoldenResponse {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected to find feed_list in reponse body")
	}
}

const addFeedGoldenResponse = `{
  "feed": {
    "id": 1,
    "name": "test",
    "url": "http://test/url/feed.atom",
    "lastPollTime": "0001-01-01T00:00:00Z",
    "lastPollError": ""
  }
}`

func TestAddFeed(t *testing.T) {
	_, m := setupTest(t)
	response := httptest.NewRecorder()

	f := FeedJSON{
		Feed: &db.FeedInfo{
			Url:  "http://test/url/feed.atom",
			Name: "test",
		},
	}

	req_body, err := json.Marshal(f)
	failOnError(t, err)

	req, err := http.NewRequest("POST", "/api/1/feeds",
		bytes.NewReader(req_body))
	failOnError(t, err)

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	m.ServeHTTP(response, req)

	if response.Code != 201 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 201 response code, got %d", response.Code)
	}
	if response.Body.String() != addFeedGoldenResponse {
		fmt.Println(response.Body.String())
		t.Fatalf("Response didn't match expected response.")
	}
}

type ErrorMessage []struct {
	FieldNames     []string `json:"fieldNames"`
	Classification string   `json:"classification"`
	Message        string   `json:"message"`
}

func TestAddFeedWithMalformedData(t *testing.T) {
	_, m := setupTest(t)
	response := httptest.NewRecorder()

	f := FeedJSON{}

	req_body, err := json.Marshal(f)
	failOnError(t, err)

	req, err := http.NewRequest("POST", "/api/1/feeds",
		bytes.NewReader(req_body))
	failOnError(t, err)

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	m.ServeHTTP(response, req)

	if response.Code != 422 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 422 response code, got %d", response.Code)
	}

	var a ErrorMessage
	err = json.Unmarshal(response.Body.Bytes(), &a)

	if len(a) != 1 {
		t.Fatalf("Expected only one error message got:\n%s", spew.Sdump(a))
	}
	if a[0].Classification != "RequiredError" {
		t.Fatalf("Expected to find Feed error field, found nothing")
	}
}

const getFeedGoldenOutput = `{
  "feed": {
    "id": 1,
    "name": "test_feed1",
    "url": "http://testfeed1/feed.atom",
    "lastPollTime": "0001-01-01T00:00:00Z",
    "lastPollError": ""
  }
}`

func TestGetFeed(t *testing.T) {
	dbh, m := setupTest(t)
	loadFixtures(t, dbh)
	dbfeeds, err := dbh.GetAllFeeds()
	failOnError(t, err)

	response := httptest.NewRecorder()
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("/api/1/feeds/%d", dbfeeds[0].Id),
		nil)
	failOnError(t, err)

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("GetFeedd Expected 200 response code, got %d", response.Code)
	}
	if response.Body.String() != getFeedGoldenOutput {
		fmt.Println(response.Body.String())
		t.Fatalf("Response didn't match expected response.")
	}
}

func TestDeleteFeed(t *testing.T) {
	dbh, m := setupTest(t)
	loadFixtures(t, dbh)

	feeds, err := dbh.GetAllFeeds()
	failOnError(t, err)

	req, _ := http.NewRequest("DELETE",
		fmt.Sprintf("/api/1/feeds/%d", feeds[0].Id), nil)
	response := httptest.NewRecorder()
	m.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 204 response code, got %d", response.Code)
	}
	_, err = dbh.GetFeedById(feeds[0].Id)
	if err == nil {
		t.Fatalf("Found feed when it should have been deleted")
	}
}
