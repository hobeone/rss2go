package db

import (
	"testing"
	"time"
	"log"
)

type NullWriter int
func (NullWriter) Write([]byte) (int, error) { return 0, nil }
func DisableLogging() {
	log.SetOutput(new(NullWriter))
}

func init() {
	DisableLogging()
}

func TestFeedCreation(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)

	var feed FeedInfo
	feed.Name = "Test Feed"
	feed.Url = "https://testfeed.com/test"
	feed.LastPollTime = time.Now()
	err := d.Orm.Save(&feed)
	if err != nil {
		t.Fatal("Error saving test feed.")
	}

	var fetched_feed FeedInfo

	d.Orm.Where(feed.Id).Find(&fetched_feed)
}

func TestCheckRecordGuid(t *testing.T) {
	d := NewMemoryDbDispatcher(true, true)
	err := d.RecordGuid(1, "123")

	if err != nil {
		t.Errorf("Error recording guid: %s\n", err.Error())
	}
}

func TestGetGuidsForFeed(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)

	guids := []string{"1","2","3"}

	var feed FeedItem
	feed.FeedInfoId = 1
	feed.Guid = "1"
	err := d.Orm.Save(&feed)
	if err != nil {
		t.Fatalf("Error saving test item: %s", err)
	}

	known_guids, err := d.GetGuidsForFeed(1, &guids)
	if err != nil {
		t.Fatalf("Error running SQL: %s", err.Error())
	}
	if len(*known_guids) != 1 {
		t.Fatalf("Error getting guids from db.  Expected 1, got: %#v", known_guids)
	}
}

func TestAddAndDeleteFeed(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feed, err := d.AddFeed("test feed", "http://test/feed.xml")

	if err != nil {
		t.Error("AddFeed shouldn't return an error")
	}

	feed, err = d.AddFeed("test feed", "http://test/feed.xml")

	if err == nil {
		t.Error("AddFeed should return an error")
	}

	err = d.RemoveFeed(feed.Url, true)
	if err != nil {
		t.Errorf("RemoveFeed shouldn't return an error. Got: %s", err.Error())
	}

	_, err = d.GetFeedByUrl(feed.Url)
	if err == nil {
		t.Errorf("Feed with url %s shouldn't exist anymore.", feed.Url)
	}
}
