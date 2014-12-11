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
	. "github.com/onsi/gomega"
)

const getAllFeedGoldenOutput = `{
  "feeds": [
    {
      "id": 1,
      "name": "testfeed1",
      "url": "http://localhost/feed1.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    },
    {
      "id": 2,
      "name": "testfeed2",
      "url": "http://localhost/feed2.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    },
    {
      "id": 3,
      "name": "testfeed3",
      "url": "http://localhost/feed3.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    }
  ]
}`

func TestGetAllFeeds(t *testing.T) {
	dbh, m := setupTest(t)
	db.LoadFixtures(t, dbh, "http://localhost")
	RegisterTestingT(t)
	response := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/1/feeds", nil)
	Expect(err).ToNot(HaveOccurred(), "Error creating request: %s", err)

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}
	Expect(response.Body.String()).Should(MatchJSON(getAllFeedGoldenOutput))
}

const getSomeFeedsGoldenResponse = `{
  "feeds": [
    {
      "id": 1,
      "name": "testfeed1",
      "url": "http://localhost/feed1.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    },
    {
      "id": 2,
      "name": "testfeed2",
      "url": "http://localhost/feed2.atom",
      "lastPollTime": "0001-01-01T00:00:00Z",
      "lastPollError": ""
    }
  ]
}`

func TestGetSomeFeeds(t *testing.T) {
	dbh, m := setupTest(t)
	db.LoadFixtures(t, dbh, "http://localhost")
	RegisterTestingT(t)
	response := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/1/feeds?ids[]=1&ids[]=2", nil)
	if err != nil {
		t.Fatalf("Error getting response: %s", err)
	}

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}
	Expect(response.Body.String()).Should(MatchJSON(getSomeFeedsGoldenResponse))
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
	RegisterTestingT(t)
	f := FeedJSON{
		Feed: &db.FeedInfo{
			Url:  "http://test/url/feed.atom",
			Name: "test",
		},
	}

	reqBody, err := json.Marshal(f)
	failOnError(t, err)

	req, err := http.NewRequest("POST", "/api/1/feeds",
		bytes.NewReader(reqBody))
	failOnError(t, err)

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	m.ServeHTTP(response, req)

	if response.Code != 201 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 201 response code, got %d", response.Code)
	}
	Expect(response.Body.String()).Should(MatchJSON(addFeedGoldenResponse))
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

	reqBody, err := json.Marshal(f)
	failOnError(t, err)

	req, err := http.NewRequest("POST", "/api/1/feeds",
		bytes.NewReader(reqBody))
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
    "name": "testfeed1",
    "url": "http://localhost/feed1.atom",
    "lastPollTime": "0001-01-01T00:00:00Z",
    "lastPollError": ""
  }
}`

func TestGetFeed(t *testing.T) {
	dbh, m := setupTest(t)
	db.LoadFixtures(t, dbh, "http://localhost")
	RegisterTestingT(t)

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
	Expect(response.Body.String()).Should(MatchJSON(getFeedGoldenOutput))
}

func TestDeleteFeed(t *testing.T) {
	dbh, m := setupTest(t)
	db.LoadFixtures(t, dbh, "http://localhost")

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
