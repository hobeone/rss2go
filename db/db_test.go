package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFeedCreation(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	var feed FeedInfo
	feed.Name = "Test Feed"
	feed.Url = "https://testfeed.com/test"
	feed.LastPollTime = time.Now()
	err := d.DB.Save(&feed).Error
	if err != nil {
		t.Fatal("Error saving test feed.")
	}

	var fetchedFeed FeedInfo

	err = d.DB.Find(&fetchedFeed, feed.Id).Error
	if err != nil {
		t.Fatalf("Expected to find just created Feed, got error: %s", err)
	}
}

func TestCheckRecordGuid(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	err := d.RecordGuid(1, "123")
	assert.Nil(t, err)
}

func TestGetMostRecentGuidsForFeed(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d)

	d.RecordGuid(feeds[0].Id, "123")
	d.RecordGuid(feeds[0].Id, "1234")
	d.RecordGuid(feeds[0].Id, "12345")

	maxGuidsToFetch := 2
	guids, err := d.GetMostRecentGuidsForFeed(feeds[0].Id, maxGuidsToFetch)
	assert.Nil(t, err)
	assert.Equal(t, len(guids), maxGuidsToFetch)

	assert.Equal(t, guids[0], "12345")
	assert.Equal(t, guids[1], "1234")

	guids, err = d.GetMostRecentGuidsForFeed(feeds[0].Id, -1)
	assert.Nil(t, err)
	assert.Equal(t, len(guids), 3)
}

func TestAddFeedValidation(t *testing.T) {
	d := NewMemoryDBHandle(false, true)

	var inputs = [][]string{
		[]string{"test", "bad url"},
		[]string{"test", "http://"},
		[]string{"", ""},
	}

	for _, ins := range inputs {
		_, err := d.AddFeed(ins[0], ins[1])
		assert.NotNil(t, err, "AddFeed should return an error on invalid URL. Inputs: '%s','%s'", ins[0], ins[1])
	}
}

func TestAddAndDeleteFeed(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feed, err := d.AddFeed("test feed", "http://test/feed.xml")

	assert.Nil(t, err, "AddFeed shouldn't return an error")

	_, err = d.AddFeed("test feed", "http://test/feed.xml")

	assert.NotNil(t, err, "AddFeed should return an error when adding a duplicate feed.")

	err = d.RecordGuid(feed.Id, "abcd")
	assert.Nil(t, err)
	user1, err := d.AddUser("name", "email@example.com", "pass")
	assert.Nil(t, err)

	err = d.AddFeedsToUser(user1, []*FeedInfo{feed})
	assert.Nil(t, err)
	err = d.RemoveFeed(feed.Url)
	assert.Nil(t, err)

	_, err = d.GetFeedByUrl(feed.Url)
	assert.NotNil(t, err, "Feed with url %s shouldn't exist anymore.", feed.Url)

	i := FeedItem{}
	err = d.DB.Where("guid = ?", "abcd").Find(&i).Error
	assert.NotNil(t, err, "FeedItem was not deleted with feed.")

	fu := UserFeed{}
	err = d.DB.Where("feed_id = ?", feed.Id).Find(&fu).Error
	assert.NotNil(t, err, "UserFeeds were not deleted with feed.")
}

func TestGetFeedItemByGuid(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d)

	feed1 := feeds[0]
	feed2 := feeds[1]
	d.RecordGuid(feed1.Id, "foobar")
	d.RecordGuid(feed2.Id, "foobaz")
	d.RecordGuid(feed2.Id, "foobar")
	guid, err := d.GetFeedItemByGuid(feed1.Id, "foobar")
	assert.Nil(t, err, "Error getting guid")

	assert.Equal(t, guid.FeedInfoId, 1)
}

func TestGetStaleFeeds(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d)
	feed1 := feeds[0]

	d.RecordGuid(feed1.Id, "foobar")
	d.RecordGuid(feeds[1].Id, "foobaz")
	d.RecordGuid(feeds[2].Id, "foobaz")
	guid, err := d.GetFeedItemByGuid(feed1.Id, "foobar")
	assert.Nil(t, err, "Error getting guid")
	guid.AddedOn = *new(time.Time)
	err = d.DB.Save(guid).Error
	assert.Nil(t, err)

	f, err := d.GetStaleFeeds()
	if err != nil {
		t.Fatalf("Error getting stale feeds: %v", err)
	}

	assert.Equal(t, f[0].Id, feed1.Id)
}

func TestAddUserValidation(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	var inputs = [][]string{
		[]string{"test", ".bad@address"},
		[]string{"test", ""},
		[]string{"", ""},
	}

	for _, ins := range inputs {
		_, err := d.AddUser(ins[0], ins[1], "pass")
		assert.NotNil(t, err, "AddUser should return an error on invalid args. Inputs: '%s','%s'", ins[0], ins[1])
	}
}

func TestAddRemoveUser(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d)

	_, err := d.AddUser(users[0].Name, "diff_email@example.com", "")
	assert.NotNil(t, err, "Should have error on duplicate user name")

	_, err = d.AddUser("diff_name", users[0].Email, "")
	assert.NotNil(t, err, "Should have error on duplicate user email")

	dbUser, err := d.GetUser(users[0].Name)
	assert.Nil(t, err)

	err = d.AddFeedsToUser(dbUser, []*FeedInfo{feeds[0]})
	assert.Nil(t, err)

	err = d.RemoveUser(dbUser)
	assert.Nil(t, err)

	// Check that feed was removed b/c it has no users
	var u []UserFeed
	d.DB.Find(&u)
	assert.NotEqual(t, len(u), 0, "Expecting 0 UserFeeds remaining after deleting user, got %d", len(u))
}

func TestRemoveFeedsFromUser(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d)

	err := d.AddFeedsToUser(users[0], []*FeedInfo{feeds[0]})
	assert.Nil(t, err, "Error adding feeds to a user")

	err = d.RemoveFeedsFromUser(users[0], []*FeedInfo{feeds[0]})
	assert.Nil(t, err, "Error removing feeds to a user")
}

func TestGetFeedsWithUsers(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d)

	userFeeds, err := d.GetUsersFeeds(users[0])
	assert.Nil(t, err, "Error getting a user's feeds")

	assert.Equal(t, len(userFeeds), 3,
		"Expected 3 feed for user got %d.", len(userFeeds))

	assert.Equal(t, userFeeds[0].Name, feeds[0].Name)
	assert.Equal(t, userFeeds[0].Url, feeds[0].Url)
}

func TestGetFeedUsers(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d)

	feedUsers, err := d.GetFeedUsers(feeds[0].Url)
	assert.Nil(t, err)

	assert.Equal(t, len(feedUsers), 3)
	assert.Equal(t, feedUsers[0].Email, users[0].Email)
}

func TestUpdateUsersFeeds(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d)

	err := d.UpdateUsersFeeds(users[0], []int64{})
	if err != nil {
		t.Fatalf("Error updating user feeds: %s", err)
	}

	newFeeds, err := d.GetUsersFeeds(users[0])
	assert.Nil(t, err)
	assert.Equal(t, len(newFeeds), 0)

	feedIDs := make([]int64, len(feeds))
	for i := range feeds {
		feedIDs[i] = feeds[i].Id
	}
	d.UpdateUsersFeeds(users[0], feedIDs)

	newFeeds, err = d.GetUsersFeeds(users[0])
	assert.Nil(t, err)
	assert.Equal(t, len(newFeeds), 3)
}
