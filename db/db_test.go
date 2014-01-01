package db

import (
	"fmt"
	"runtime/debug"
	"testing"
	"time"
)

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

	if err != nil {
		t.Errorf("Error recording guid: %s\n", err.Error())
	}
}

func TestGetMostRecentGuidsForFeed(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feed_id := 1
	d.RecordGuid(feed_id, "123")
	d.RecordGuid(feed_id, "1234")
	d.RecordGuid(feed_id, "12345")

	max_guids_to_fetch := 2
	guids, err := d.GetMostRecentGuidsForFeed(feed_id, max_guids_to_fetch)

	if err != nil {
		t.Error(err.Error())
	}

	if len(guids) != 2 {
		t.Errorf("Expecting to get 2 guids.  Got %d", len(guids))
	}

	if guids[0] != "12345" {
		t.Errorf("Expecting 12345 as first guid got %s", guids[0])
	}

	if guids[1] != "1234" {
		t.Errorf("Expecting 1234 as second guid got %s", guids[1])
	}

	guids, err = d.GetMostRecentGuidsForFeed(feed_id, -1)

	if err != nil {
		t.Error(err.Error())
	}
	if len(guids) != 3 {
		t.Errorf("Expecting to get 3 guids.  Got %d", len(guids))
	}
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
		if err == nil {
			t.Errorf("AddFeed should return an error on invalid URL. Inputs: '%s','%s'", ins[0], ins[1])
		}
	}
}

func TestAddAndDeleteFeed(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feed, err := d.AddFeed("test feed", "http://test/feed.xml")

	if err != nil {
		t.Error("AddFeed shouldn't return an error")
	}

	_, err = d.AddFeed("test feed", "http://test/feed.xml")

	if err == nil {
		t.Error("AddFeed should return an error when adding a duplicate feed.")
	}

	err = d.RecordGuid(feed.Id, "abcd")
	if err != nil {
		t.Fatalf("Couldn't add item to feed: %s", err)
	}
	user1, err := d.AddUser("name", "email@example.com")
	if err != nil {
		t.Fatalf("Error creating test user: %s", err)
	}

	err = d.AddFeedsToUser(user1, []*FeedInfo{feed})
	if err != nil {
		t.Fatalf("Error adding feeds to a user: %s", err)
	}

	err = d.RemoveFeed(feed.Url, true)
	if err != nil {
		t.Errorf("RemoveFeed shouldn't return an error. Got: %s", err.Error())
	}

	_, err = d.GetFeedByUrl(feed.Url)
	if err == nil {
		t.Errorf("Feed with url %s shouldn't exist anymore.", feed.Url)
	}

	i := FeedItem{}
	err = d.Orm.Where("guid = ?", "abcd").Find(&i)
	if err == nil {
		t.Errorf("FeedItem was not deleted with feed.")
	}

	fu := UserFeed{}
	err = d.Orm.Where("feed_id = ?", feed.Id).Find(&fu)
	if err == nil {
		t.Errorf("UserFeeds were not deleted with feed.")
	}
}

func TestGetFeedItemByGuid(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)

	feed1, _ := d.AddFeed("test1", "http://foo.bar/")
	feed2, _ := d.AddFeed("test2", "http://foo.baz/")
	d.RecordGuid(feed1.Id, "foobar")
	d.RecordGuid(feed2.Id, "foobaz")
	d.RecordGuid(feed2.Id, "foobar")
	guid, err := d.GetFeedItemByGuid(feed1.Id, "foobar")
	if err != nil {
		t.Fatalf("Error getting guid: %s", err.Error())
	}
	if guid.FeedInfoId != 1 {
		t.Fatalf("Error getting guid: %s", err.Error())
	}
}

func TestGetStaleFeeds(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)

	feed1, _ := d.AddFeed("test1", "http://foo.bar/")
	feed2, _ := d.AddFeed("test2", "http://foo.baz/")
	d.RecordGuid(feed1.Id, "foobar")
	d.RecordGuid(feed2.Id, "foobaz")
	guid, err := d.GetFeedItemByGuid(feed1.Id, "foobar")
	if err != nil {
		t.Fatalf("Error getting guid: %s", err.Error())
	}
	guid.AddedOn = *new(time.Time)
	d.Orm.Save(guid)

	f, err := d.GetStaleFeeds()
	if err != nil {
		t.Errorf("Error getting stale feeds: %s", err.Error())
	}

	exp := "http://foo.bar/"
	if f[0].Url != exp {
		t.Error("Expected stale feed url of %s, instead got %s", exp, f[0].Url)
	}
}

func TestAddUserValidation(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	var inputs = [][]string{
		[]string{"test", ".bad@address"},
		[]string{"test", ""},
		[]string{"", ""},
	}

	for _, ins := range inputs {
		_, err := d.AddUser(ins[0], ins[1])
		if err == nil {
			t.Errorf("AddUser should return an error on invalid args. Inputs: '%s','%s'", ins[0], ins[1])
		}
	}
}

func TestAddRemoveUser(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feed1, err := d.AddFeed("test1", "http://foo.bar/")
	if err != nil {
		t.Fatalf("Error creating test feed: %s", err)
	}

	_, err = d.AddUser("user1", "user1@example.com")
	if err != nil {
		t.Fatalf("Error creating test user: %s", err)
	}
	_, err = d.AddUser("user1", "diff_email@example.com")
	if err == nil {
		t.Fatalf("Should have error on duplicate user name")
	}
	_, err = d.AddUser("diff_name", "user1@example.com")
	if err == nil {
		t.Fatalf("Should have error on duplicate user email")
	}

	db_user, err := d.GetUser("user1")
	if err != nil {
		t.Fatalf("Error getting user from db: %s", err)
	}

	err = d.AddFeedsToUser(db_user, []*FeedInfo{feed1})
	if err != nil {
		t.Fatalf("Error adding feeds to a user: %s", err)
	}

	err = d.RemoveUser(db_user)
	if err != nil {
		t.Fatalf("Error removing user from db: %s", err)
	}

	// Check that feed was removed b/c it has no users
	var u []UserFeed
	d.Orm.FindAll(&u)
	if len(u) != 0 {
		t.Fatalf("Expecting 0 UserFeeds remaining after deleting user, got %d",
			len(u))
	}
}

func TestRemoveFeedsFromUser(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feed1, err := d.AddFeed("test1", "http://foo.bar/")
	if err != nil {
		t.Fatalf("Error creating test feed: %s", err)
	}

	user1, err := d.AddUser("name", "email@example.com")
	if err != nil {
		t.Fatalf("Error creating test user: %s", err)
	}

	err = d.AddFeedsToUser(user1, []*FeedInfo{feed1})
	if err != nil {
		t.Fatalf("Error adding feeds to a user: %s", err)
	}

	err = d.RemoveFeedsFromUser(user1, []*FeedInfo{feed1})
	if err != nil {
		t.Fatalf("Error removing feeds from a user: %s", err)
	}
}

func TestGetFeedsWithUsers(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)

	feed1, err := d.AddFeed("test1", "http://foo.bar/")
	if err != nil {
		t.Fatalf("Error creating test feed: %s", err)
	}

	feed2, err := d.AddFeed("test2", "http://foo.baz/")
	if err != nil {
		t.Fatalf("Error creating test feed: %s", err)
	}

	user1, err := d.AddUser("name", "email@example.com")
	if err != nil {
		t.Fatalf("Error creating test user: %s", err)
	}

	user2, err := d.AddUser("name2", "email2@example.com")
	if err != nil {
		t.Fatalf("Error creating test user: %s", err)
	}

	err = d.AddFeedsToUser(user1, []*FeedInfo{feed1})
	if err != nil {
		t.Fatalf("Error adding feeds to a user: %s", err)
	}
	err = d.AddFeedsToUser(user2, []*FeedInfo{feed2})
	if err != nil {
		t.Fatalf("Error adding feeds to a user: %s", err)
	}

	user_feeds, err := d.GetUsersFeeds(user1)
	if err != nil {
		t.Fatalf("Error getting a user's feeds: %s", err)
	}

	if len(user_feeds) != 1 {
		t.Errorf("Expected 1 feed for user got %d.", len(user_feeds))
	}
	if user_feeds[0].Url != feed1.Url {
		t.Error("Expected feed to have url %s but got %s", feed1.Url,
			user_feeds[0].Url)
	}
}

func TestGetFeedUsers(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)

	feed1, err := d.AddFeed("test1", "http://foo.bar/")
	if err != nil {
		t.Fatalf("Error creating test feed: %s", err)
	}

	user1, err := d.AddUser("name", "email@example.com")
	if err != nil {
		t.Fatalf("Error creating test user: %s", err)
	}

	err = d.AddFeedsToUser(user1, []*FeedInfo{feed1})
	if err != nil {
		t.Fatalf("Error adding feeds to a user: %s", err)
	}
	feed_users, err := d.GetFeedUsers(feed1.Url)
	if err != nil {
		t.Fatalf("Error getting a feed's users: %s", err)
	}

	if len(feed_users) != 1 {
		t.Fatalf("Expected 1 user for feed got %d.", len(feed_users))
	}
	if feed_users[0].Email != user1.Email {
		t.Error("Expected user to have email %s but got %s", user1.Email,
			feed_users[0].Email)
	}
}

const USER_FIXTURES = ``

func loadFixtures(t *testing.T, d *DbDispatcher) ([]*FeedInfo, []*User) {
	users := map[string]string{
		"test1": "test1@example.com",
		"test2": "test2@example.com",
		"test3": "test3@example.com",
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
		if err != nil {
			t.Fatalf("Error adding feed to db: %s", err.Error())
		}
		db_feeds[i] = feed
		i++
	}

	db_users := make([]*User, len(users))
	i = 0
	for name, email := range users {
		u, err := d.AddUser(name, email)
		if err != nil {
			t.Fatalf("Error adding user to db: %s", err.Error())
		}
		db_users[i] = u
		i++

		err = d.AddFeedsToUser(u, db_feeds)
		if err != nil {
			t.Fatalf("Error adding feed to user: %s", err.Error())
		}
	}
	return db_feeds, db_users
}

func failOnError(t *testing.T, err error) {
	if err != nil {
		fmt.Println(string(debug.Stack()))
		t.Fatalf("Error: %s", err.Error())
	}
}

func TestUpdateUsersFeeds(t *testing.T) {
	d := NewMemoryDbDispatcher(false, true)
	feeds, users := loadFixtures(t, d)

	d.UpdateUsersFeeds(users[0], []int{})

	new_feeds, err := d.GetUsersFeeds(users[0])
	failOnError(t, err)
	if len(new_feeds) != 0 {
		t.Fatalf("Expected 0 feeds for user, got %d", len(new_feeds))
	}

	feed_ids := make([]int, len(feeds))
	for i := range feeds {
		feed_ids[i] = feeds[i].Id
	}
	d.UpdateUsersFeeds(users[0], feed_ids)

	new_feeds, err = d.GetUsersFeeds(users[0])
	failOnError(t, err)
	if len(new_feeds) != 3 {
		t.Fatalf("Expected 3 feeds for user, got %d", len(new_feeds))
	}
}
