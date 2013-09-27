package db

import (
	"testing"
	_ "fmt"
	"time"
)

func TestFeedCreation(t *testing.T) {
	orm := CreateAndOpenDb("test.db", false)

	var feed FeedInfo
	feed.Name = "Test Feed"
	feed.Url = "https://testfeed.com/test"
	feed.LastPollTime = time.Now()
	err := orm.Save(&feed)
	if err != nil {
		t.Fatal("Error saving test user.")
	}

	var fetched_feed FeedInfo

	orm.Where(feed.Id).Find(&fetched_feed)

	if !fetched_feed.LastItemTime.IsZero() {
		t.Error("LastItemTime should be zero when not set.")
	}
}

func TestUpdateFeedPollTime(t *testing.T) {
	d := NewDbDispatcher("test.db", false)

	var feed FeedInfo
	feed.Name = "Test Feed"
	feed.Url = "https://testfeed.com/test"
	feed.LastPollTime = *new(time.Time)
	err := d.OrmHandle.Save(&feed)
	if err != nil {
		t.Fatal("Error saving test user:", err)
	}

	err = d.UpdateFeedLastItemTimeByUrl(feed.Url, feed.LastPollTime)
	if err != nil {
		t.Fatal("Error updating test feed:", err.Error())
	}

	var fetched_feed FeedInfo
	d.OrmHandle.Where(feed.Id).Find(&fetched_feed)

	if fetched_feed.LastPollTime == feed.LastPollTime {
		t.Error("UpdateFeedPollTime didn't update the poll time.")
	}
}
