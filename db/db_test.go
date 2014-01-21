package db

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func loadFixtures(t *testing.T, d *DbDispatcher) ([]*FeedInfo, []*User) {
	users := map[string][]string{
		"test1": []string{"test1@example.com", "pass"},
		"test2": []string{"test2@example.com", "pass"},
		"test3": []string{"test3@example.com", "pass"},
	}
	feeds := map[string]string{
		"test_feed1": "http://testfeed1/feed.atom",
		"test_feed2": "http://testfeed2/feed.atom",
		"test_feed3": "http://testfeed3/feed.atom",
	}
	db_feeds := make([]*FeedInfo, len(feeds))
	i := 0
	for name, url := range feeds {
		feed, err := d.AddFeed(name, url)
		assert.Nil(t, err, "Error adding feed to db")
		db_feeds[i] = feed
		i++
	}

	db_users := make([]*User, len(users))
	i = 0
	for name, user_data := range users {
		u, err := d.AddUser(name, user_data[0], user_data[1])
		assert.Nil(t, err, "Error adding user to db")
		db_users[i] = u
		i++

		err = d.AddFeedsToUser(u, db_feeds)
		assert.Nil(t, err, "Error adding feed to user")
	}
	return db_feeds, db_users
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
	d := NewMemoryDbDispatcher(false, true)
	err := d.RecordGuid(1, "123")
	assert.Nil(t, err)
}

func TestGetMostRecentGuidsForFeed(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feeds, _ := loadFixtures(t, d)

	d.RecordGuid(feeds[0].Id, "123")
	d.RecordGuid(feeds[0].Id, "1234")
	d.RecordGuid(feeds[0].Id, "12345")

	max_guids_to_fetch := 2
	guids, err := d.GetMostRecentGuidsForFeed(feeds[0].Id, max_guids_to_fetch)
	assert.Nil(t, err)
	assert.Equal(t, len(guids), max_guids_to_fetch)

	assert.Equal(t, guids[0], "12345")
	assert.Equal(t, guids[1], "1234")

	guids, err = d.GetMostRecentGuidsForFeed(feeds[0].Id, -1)
	assert.Nil(t, err)
	assert.Equal(t, len(guids), 3)
}

func TestAddFeedValidation(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)

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
	d := NewMemoryDbDispatcher(false, true)
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
	err = d.RemoveFeed(feed.Url, true)
	assert.Nil(t, err)

	_, err = d.GetFeedByUrl(feed.Url)
	assert.NotNil(t, err, "Feed with url %s shouldn't exist anymore.", feed.Url)

	i := FeedItem{}
	err = d.Orm.Where("guid = ?", "abcd").Find(&i)
	assert.NotNil(t, err, "FeedItem was not deleted with feed.")

	fu := UserFeed{}
	err = d.Orm.Where("feed_id = ?", feed.Id).Find(&fu)
	assert.NotNil(t, err, "UserFeeds were not deleted with feed.")
}

func TestGetFeedItemByGuid(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feeds, _ := loadFixtures(t, d)

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
	d := NewMemoryDbDispatcher(false, true)
	feeds, _ := loadFixtures(t, d)
	feed1 := feeds[0]

	d.RecordGuid(feed1.Id, "foobar")
	d.RecordGuid(feeds[1].Id, "foobaz")
	d.RecordGuid(feeds[2].Id, "foobaz")
	guid, err := d.GetFeedItemByGuid(feed1.Id, "foobar")
	assert.Nil(t, err, "Error getting guid")
	guid.AddedOn = *new(time.Time)
	err = d.Orm.Save(guid)
	assert.Nil(t, err)

	f, err := d.GetStaleFeeds()
	assert.Nil(t, err, "Error getting stale feeds")

	assert.Equal(t, f[0].Id, feed1.Id)
}

func TestAddUserValidation(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
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
	d := NewMemoryDbDispatcher(false, true)
	feeds, users := loadFixtures(t, d)

	_, err := d.AddUser(users[0].Name, "diff_email@example.com", "")
	assert.NotNil(t, err, "Should have error on duplicate user name")

	_, err = d.AddUser("diff_name", users[0].Email, "")
	assert.NotNil(t, err, "Should have error on duplicate user email")

	db_user, err := d.GetUser(users[0].Name)
	assert.Nil(t, err)

	err = d.AddFeedsToUser(db_user, []*FeedInfo{feeds[0]})
	assert.Nil(t, err)

	err = d.RemoveUser(db_user)
	assert.Nil(t, err)

	// Check that feed was removed b/c it has no users
	var u []UserFeed
	d.Orm.FindAll(&u)
	assert.NotEqual(t, len(u), 0, "Expecting 0 UserFeeds remaining after deleting user, got %d", len(u))
}

func TestRemoveFeedsFromUser(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feeds, users := loadFixtures(t, d)

	err := d.AddFeedsToUser(users[0], []*FeedInfo{feeds[0]})
	assert.Nil(t, err, "Error adding feeds to a user")

	err = d.RemoveFeedsFromUser(users[0], []*FeedInfo{feeds[0]})
	assert.Nil(t, err, "Error removing feeds to a user")
}

func TestGetFeedsWithUsers(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feeds, users := loadFixtures(t, d)

	user_feeds, err := d.GetUsersFeeds(users[0])
	assert.Nil(t, err, "Error getting a user's feeds")

	assert.Equal(t, len(user_feeds), 3,
		"Expected 1 feed for user got %d.", len(user_feeds))

	assert.Equal(t, user_feeds[0].Url, feeds[0].Url)
}

func TestGetFeedUsers(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feeds, users := loadFixtures(t, d)

	feed_users, err := d.GetFeedUsers(feeds[0].Url)
	assert.Nil(t, err)

	assert.Equal(t, len(feed_users), 3)
	assert.Equal(t, feed_users[0].Email, users[0].Email)
}

func TestUpdateUsersFeeds(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feeds, users := loadFixtures(t, d)

	d.UpdateUsersFeeds(users[0], []int{})

	new_feeds, err := d.GetUsersFeeds(users[0])
	assert.Nil(t, err)
	assert.Equal(t, len(new_feeds), 0)

	feed_ids := make([]int, len(feeds))
	for i := range feeds {
		feed_ids[i] = feeds[i].Id
	}
	d.UpdateUsersFeeds(users[0], feed_ids)

	new_feeds, err = d.GetUsersFeeds(users[0])
	assert.Nil(t, err)
	assert.Equal(t, len(new_feeds), 3)
}
