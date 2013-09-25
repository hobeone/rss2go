package db

import (
	"testing"
	_ "fmt"
	"time"
)

func TestDbTest(t *testing.T) {
	orm := CreateAndOpenDb("test.db", false)

	_, err := GetAllFeeds(orm)

	if err != nil {
		t.Error("Error getting all feeds: ", err.Error())
	}
}

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
	orm := CreateAndOpenDb("test.db", false)

	var feed FeedInfo
	feed.Name = "Test Feed"
	feed.Url = "https://testfeed.com/test"
	feed.LastPollTime = *new(time.Time)
	err := orm.Save(&feed)
	if err != nil {
		t.Fatal("Error saving test user:", err)
	}

	err = UpdateFeedPollTime(orm, &feed)
	if err != nil {
		t.Fatal("Error updating test feed:", err.Error())
	}


	var fetched_feed FeedInfo
	orm.Where(feed.Id).Find(&fetched_feed)

	if fetched_feed.LastPollTime == feed.LastPollTime {
		t.Error("UpdateFeedPollTime didn't update the poll time.")
	}

}
