package db

import (
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	testdb "github.com/erikstmartin/go-testdb"
	"github.com/sirupsen/logrus"
)

func NullLogger() logrus.FieldLogger {
	l := logrus.New()
	l.Level = logrus.DebugLevel
	l.Out = io.Discard
	return l
}

func TestConnectionError(t *testing.T) {
	t.Parallel()
	defer testdb.Reset()

	testdb.SetOpenFunc(func(dsn string) (driver.Conn, error) {
		return testdb.Conn(), errors.New("failed to connect")
	})
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Didn't get expected panic")
		}
	}()

	openDB("testdb", "", NullLogger())
}

func TestGettingFeedWithTestDB(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)

	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting all Feeds: %v", err)
	}
	if len(feeds) != 3 {
		t.Fatalf("Expected to get 3 feeds got %d", len(feeds))
	}
}

func TestGetFeedByID(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	_, err := d.GetFeedByID(-1)
	if err == nil {
		t.Fatalf("Expected error on negative id, got nil")
	}

	_, err = d.GetFeedByID(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllFeedsWithUsers(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)

	feeds, err := d.GetAllFeedsWithUsers()
	if err != nil {
		t.Fatalf("unexpected error on query: %v", err)
	}
	if len(feeds) != 3 {
		t.Fatalf("Expected 3 feeds, got %d", len(feeds))
	}
}

func TestGettingFeedsWithError(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)

	allFeeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting all Feeds: %v", err)
	}

	feeds, err := d.GetFeedsWithErrors()
	if err != nil {
		t.Fatalf("Error getting all Feeds with errors: %v", err)
	}
	if len(feeds) != 0 {
		t.Fatalf("Expected to get 0 feeds got %d", len(feeds))
	}

	allFeeds[0].LastPollError = "Error"
	err = d.SaveFeed(allFeeds[0])
	if err != nil {
		t.Fatalf("Error saving feed: %s", err)
	}

	feeds, err = d.GetFeedsWithErrors()
	if err != nil {
		t.Fatalf("Error getting all Feeds with errors: %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("Expected to get 1 feeds got %d", len(feeds))
	}

}

func TestGettingUsers(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)

	dbusers, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting all users: %v", err)
	}
	if len(dbusers) != 3 {
		t.Fatalf("Expected to get 3 users got %d", len(dbusers))
	}

	u, err := d.GetUserByID(1)
	if err != nil {
		t.Fatalf("Error gettings user by id: %v", err)
	}
	if u.ID != 1 {
		t.Fatalf("Expectd user to have ID 1, got %d", u.ID)
	}

	addr := "test1@example.com"
	u, err = d.GetUserByEmail(addr)
	if err != nil {
		t.Fatalf("Error gettings user by email: %v", err)
	}
	if u.Email != addr {
		t.Fatalf("Expecte user email to = %s, got %s", addr, u.Email)
	}
}

func TestRecordGUIDDoesntAddDuplicates(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)

	ids := []string{"one", "one", "two", "one", "three", "one"}
	for _, i := range ids {
		err := d.RecordGUID(1, i)
		if err != nil {
			t.Fatalf("Error recoding guid %s: %v", i, err)
		}
	}
	var items []FeedItem
	err := d.db.Select(&items, "SELECT * FROM feed_item WHERE feed_info_id = ? ORDER BY added_on DESC LIMIT 100", 1)

	if err != nil {
		t.Fatalf("Error getting guids for feed: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(items))
	}
}

func TestGetMostRecentGuidsForFeed(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)

	ids := []string{"123", "1234", "12345"}
	for _, i := range ids {
		err := d.RecordGUID(1, i)
		if err != nil {
			t.Fatalf("Error recoding guid %s: %v", i, err)
		}
	}

	maxGuidsToFetch := 2
	guids, err := d.GetMostRecentGUIDsForFeed(1, maxGuidsToFetch)
	if err != nil {
		t.Fatalf("Error getting uids for feed: %v", err)
	}
	if len(guids) != maxGuidsToFetch {
		t.Fatalf("Expected %d GUIDs got %d", maxGuidsToFetch, len(guids))
	}
	if !reflect.DeepEqual(guids, []string{"12345", "1234"}) {
		t.Fatalf("Unexpected GUIDS: %v", guids)
	}

	guids, err = d.GetMostRecentGUIDsForFeed(1, -1)
	if err != nil {
		t.Fatalf("Error getting guids: %v", err)
	}
	if len(guids) != 3 {
		t.Fatalf("Expected 3 GUIDs, got %d", len(guids))
	}
}

func TestGetMostRecentGuidsForFeedWithNoRecords(t *testing.T) {
	t.Parallel()

	d := NewMemoryDBHandle(NullLogger(), true)

	guids, err := d.GetMostRecentGUIDsForFeed(1, -1)
	if err != nil {
		t.Fatalf("Error getting guids: %v", err)
	}
	if len(guids) != 0 {
		t.Fatalf("Expected 0 guids got %d", len(guids))
	}
}

func TestAddFeedValidation(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	inputs := [][]string{
		{"good name", "bad url"},
		{"good name", "http://"},
		{"good name", ":badurl"},
		{"", ""},
	}

	for i, ins := range inputs {
		_, err := d.AddFeed(ins[0], ins[1])
		if err == nil {
			t.Errorf("Expected error on invalid feed, got non for feed index %d", i)
		}
	}
}
func TestFeedValidation(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	inputs := []FeedInfo{
		{
			Name: "",
			URL:  "bad url",
		},
		{},
		{URL: ":badurl"},
	}

	for _, f := range inputs {
		err := d.SaveFeed(&f)
		if err == nil {
			t.Fatalf("Expected error saving feed, got none.")
		}
		if f.ID != 0 {
			t.Fatalf("Expecte ID to be 0, got %d", f.ID)
		}
	}
}

func TestAddAndDeleteFeed(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	u, err := d.GetUserByID(1)
	if err != nil {
		t.Fatalf("Error getting user: %v", err)
	}
	f, err := d.AddFeed("test feed", "http://valid/url.xml")
	if err != nil {
		t.Fatalf("Error adding feed: %v", err)
	}
	if f.ID == 0 {
		t.Fatalf("Feed ID should not be zero")
	}

	_, err = d.AddFeed("test feed", "http://valid/url.xml")
	if err == nil {
		t.Fatalf("Error should have occurred adding feed.")
	}
	err = d.RecordGUID(f.ID, "testGUID")
	if err != nil {
		t.Fatalf("Error adding GUID to feed: %v", err)
	}
	err = d.AddFeedsToUser(u, []*FeedInfo{f})
	if err != nil {
		t.Fatalf("Error adding feed to user: %v", err)
	}

	feeds, err := d.GetUsersFeedsByName(u, "test")
	if err != nil {
		t.Fatalf("Error getting users feeds: %v", err)
	}
	if len(feeds) < 1 {
		t.Fatalf("Got no feeds from GetUsersFeedsByName")
	}

	err = d.SaveFeed(f)
	if err != nil {
		t.Fatalf("Error saving feed: %v", err)
	}
	zone, _ := f.LastPollTime.Zone()
	if zone != "UTC" {
		t.Fatalf("Timezone should be UTC, got %s", zone)
	}

	f, err = d.GetFeedByID(f.ID)
	if err != nil {
		t.Fatalf("Error saving feed: %v", err)
	}
	zone, _ = f.LastPollTime.Zone()
	if zone != "UTC" {
		t.Fatalf("Timezone should be UTC, got %s", zone)
	}

	err = d.RemoveFeed(f.URL)
	if err != nil {
		t.Fatalf("Error removing feed: %s", err)
	}

	_, err = d.GetFeedByURL(f.URL)
	if err == nil {
		t.Fatalf("Expected error on removing nonexistant feed")
	}

	guids, err := d.GetMostRecentGUIDsForFeed(f.ID, -1)
	if err != nil {
		t.Fatalf("Error when getting guids for feed: %v", err)
	}
	if len(guids) != 0 {
		t.Fatalf("Expected 0 guids, got %d", len(guids))
	}

	dbusers, err := d.GetFeedUsers(f.URL)
	if err != nil {
		t.Fatalf("Unexpected error when getting feed users: %v", err)
	}
	if len(dbusers) != 0 {
		t.Fatalf("Expected 0 users got %d", len(dbusers))
	}
}

func TestGetFeedItemByGuid(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	err := d.RecordGUID(1, "feed0GUID")
	if err != nil {
		t.Fatalf("Error recording GUID: %v", err)
	}
	err = d.RecordGUID(2, "feed1GUID")
	if err != nil {
		t.Fatalf("Error recording GUID: %v", err)
	}

	// Same GUID as another feed
	err = d.RecordGUID(2, "feed0GUID")
	if err != nil {
		t.Fatalf("Error recording GUID: %v", err)
	}

	guid, err := d.GetFeedItemByGUID(1, "feed0GUID")
	if err != nil {
		t.Fatalf("Error getting item by GUID: %v", err)
	}
	if guid.FeedInfoID != 1 {
		t.Fatalf("Expected feed id of 1 got %d", guid.FeedInfoID)
	}
	if guid.GUID != "feed0GUID" {
		t.Fatalf("expected GUID of feed0GUID, got %s", guid.GUID)
	}

	guid, err = d.GetFeedItemByGUID(2, "feed0GUID")
	if err != nil {
		t.Fatalf("Error getting item by GUID: %v", err)
	}
	if guid.FeedInfoID != 2 {
		t.Fatalf("Expected feed id of 1 got %d", guid.FeedInfoID)
	}
	if guid.GUID != "feed0GUID" {
		t.Fatalf("expected GUID of feed0GUID, got %s", guid.GUID)
	}

	// Should return error on no guid
	_, err = d.GetFeedItemByGUID(-1, "feed0GUID")
	if err == nil {
		t.Fatalf("Expected error getting item by GUID, got nothing")
	}
}

func TestRemoveUserByEmail(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	err := d.RemoveUserByEmail("test1@example.com")
	if err != nil {
		t.Fatalf("Error removing users %v", err)
	}
	//TODO: check that users is removed
}

func TestGetStaleFeeds(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	var guidData = []struct {
		id   int64
		guid string
	}{
		{1, "foobar"},
		{2, "foobaz"},
		{3, "foobaz"},
	}
	for _, tdata := range guidData {
		if writeErr := d.RecordGUID(tdata.id, tdata.guid); writeErr != nil {
			t.Fatalf("Error writing test data: %s", writeErr)
		}
	}
	guid, err := d.GetFeedItemByGUID(1, "foobar")
	if err != nil {
		t.Fatalf("Got unexpected error from db: %s", err)
	}

	now := time.Now()
	oneMonthAgo := time.Unix(now.Unix()-(60*60*24*30), 0)

	_, err = d.db.Exec("UPDATE feed_item SET added_on = ? WHERE id = ?", oneMonthAgo, guid.ID)
	if err != nil {
		t.Fatalf("Error saving item: %v", err)
	}
	f, err := d.GetStaleFeeds()
	if err != nil {
		t.Fatalf("Got unexpected error from db: %s", err)
	}
	if f[0].ID != 1 {
		t.Fatalf("Expected ID to be 1 got %d", f[0].ID)
	}
	if f[0].LastPollTime.IsZero() {
		t.Fatalf("Expected non zero time, got %s", f[0].LastPollTime)
	}
}

func TestGetUserStaleFeeds(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	var guidData = []struct {
		id   int64
		guid string
	}{
		{1, "foobar"},
		{2, "foobaz"},
		{3, "foobaz"},
	}
	for _, tdata := range guidData {
		if writeErr := d.RecordGUID(tdata.id, tdata.guid); writeErr != nil {
			t.Fatalf("Error writing test data: %s", writeErr)
		}
	}
	guid, err := d.GetFeedItemByGUID(1, "foobar")
	if err != nil {
		t.Fatalf("Got unexpected error from db: %s", err)
	}

	now := time.Now()
	oneMonthAgo := time.Unix(now.Unix()-(60*60*24*30), 0)

	_, err = d.db.Exec("UPDATE feed_item SET added_on = ? WHERE id = ?", oneMonthAgo, guid.ID)
	if err != nil {
		t.Fatalf("Error saving item: %v", err)
	}

	u, err := d.GetUserByEmail("test1@example.com")
	if err != nil {
		t.Fatalf("Got error getting user %s", err)
	}

	f, err := d.GetUserStaleFeeds(u)
	if err != nil {
		t.Fatalf("Got unexpected error from db: %s", err)
	}
	if f[0].ID != 1 {
		t.Fatalf("Expected ID to be 1 got %d", f[0].ID)
	}
	if f[0].LastPollTime.IsZero() {
		t.Fatalf("Expected non zero time, got %s", f[0].LastPollTime)
	}
}

func TestAddUserValidation(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)

	inputs := [][]string{
		{"test", ".bad@address"},
		{"test", ""},
	}
	for _, ins := range inputs {
		_, err := d.AddUser(ins[0], ins[1], "pass")
		if err == nil {
			t.Fatalf("Expected err, got none")
		}
	}

	_, err := d.AddUser("", "email@address.com", "pass")
	if err == nil {
		t.Fatalf("Expected error, got none")
	}

	u, err := d.AddUser("new user", "newuser@example.com", "pass")
	if err != nil {
		t.Fatalf("Unexpected error on validation: %v", err)
	}
	if u.ID == 0 {
		t.Fatalf("Expected ID to not be 0")
	}
}

func TestAddRemoveUser(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)

	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}

	userName := "test user name"
	userEmail := "testuser_name@example.com"
	u, err := d.AddUser(userName, userEmail, "pass")
	if err != nil {
		t.Fatalf("Got error when adding user: %v", err)
	}
	if u.ID == 0 {
		t.Fatalf("Expected non zero ID")
	}

	dupUser, err := d.AddUser(userName, "extra"+userEmail, "pass")
	if err == nil {
		t.Fatalf("Expected error on save, got none")
	}
	if dupUser != nil {
		t.Fatalf("Expected nil user return, got %v", dupUser)
	}

	_, err = d.AddUser("extra"+userName, userEmail, "pass")
	if err == nil {
		t.Fatalf("Expected error, got none")
	}

	dbUser, err := d.GetUser(u.Name)
	if err != nil {
		t.Fatalf("Got error when getting user: %v", err)
	}

	if !reflect.DeepEqual(dbUser, u) {
		t.Fatalf("Expected %v to equal %v", dbUser, u)
	}

	err = d.AddFeedsToUser(u, []*FeedInfo{feeds[0]})
	if err != nil {
		t.Fatalf("Got error when adding feeds to user: %v", err)
	}

	err = d.RemoveUser(u)
	if err != nil {
		t.Fatalf("Got error when removing user: %v", err)
	}

	dbfeeds, err := d.GetUsersFeeds(u)
	if err != nil {
		t.Fatalf("Got error when getting user feeds: %v", err)
	}
	if len(dbfeeds) > 0 {
		t.Fatalf("Expected empty feed list got: %d", len(dbfeeds))
	}
}

func TestAddRemoveFeedsFromUser(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	newFeed, err := d.AddFeed("new test feed", "http://new/test.feed")
	if err != nil {
		t.Fatalf("Error saving feed: %v", err)
	}
	feeds, err := d.GetUsersFeeds(&users[0])

	if err != nil {
		t.Fatalf("Error getting users feeds: %v", err)
	}

	if len(feeds) != 3 {
		t.Fatalf("Expected 3 feeds got %d", len(feeds))
	}
	err = d.AddFeedsToUser(&users[0], []*FeedInfo{newFeed})
	if err != nil {
		t.Fatalf("Error adding feed to user: %v", err)
	}

	feeds, err = d.GetUsersFeeds(&users[0])
	if err != nil {
		t.Fatalf("Error getting users feeds: %v", err)
	}

	if len(feeds) != 4 {
		t.Fatalf("Expected 4 feeds got %d", len(feeds))
	}

	// Test that we don't add duplicates
	err = d.AddFeedsToUser(&users[0], []*FeedInfo{newFeed})
	if err != nil {
		t.Fatalf("Error adding feeds to user: %v", err)
	}

	feeds, err = d.GetUsersFeeds(&users[0])
	if err != nil {
		t.Fatalf("Error getting users feeds: %v", err)
	}

	if len(feeds) != 4 {
		t.Fatalf("Expected 4 feeds got %d", len(feeds))
	}

	err = d.RemoveFeedsFromUser(&users[0], []*FeedInfo{newFeed})
	if err != nil {
		t.Fatalf("Error removing users feeds: %v", err)
	}

	feeds, err = d.GetUsersFeeds(&users[0])
	if err != nil {
		t.Fatalf("Error getting users feeds: %v", err)
	}

	if len(feeds) != 3 {
		t.Fatalf("Expected 3 feeds got %d", len(feeds))
	}

}

func TestGetUsersFeeds(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}

	userFeeds, err := d.GetUsersFeeds(&users[0])
	if err != nil {
		t.Fatalf("Error getting users feeds: %v", err)
	}
	if len(feeds) != len(userFeeds) {
		t.Fatalf("Expected %d feeds got %d", len(feeds), len(userFeeds))
	}
}

func TestGetFeedUsers(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}
	feedUsers, err := d.GetFeedUsers(feeds[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != len(feedUsers) {
		t.Fatalf("Expected %d users got %d", len(users), len(feedUsers))
	}

	feedUsers, err = d.GetFeedUsers("invalid")
	if err != nil {
		t.Fatal(err)
	}
	if len(feedUsers) != 0 {
		t.Fatalf("Expected %d feeds got %d", 0, len(feedUsers))
	}
}

func TestUpdateUsersFeeds(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}

	dbFeeds, err := d.GetUsersFeeds(&users[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(dbFeeds) == 0 {
		t.Fatal("Expected some feeds got 0")
	}
	err = d.UpdateUsersFeeds(&users[0], []int64{})
	if err != nil {
		t.Fatal(err)
	}

	newFeeds, err := d.GetUsersFeeds(&users[0])
	if err != nil {
		t.Fatal(err)
	}

	if len(newFeeds) != 3 {
		t.Fatalf("Expected 0 feeds, got %d", len(newFeeds))
	}
	feedIDs := make([]int64, len(feeds))
	for i := range feeds {
		feedIDs[i] = feeds[i].ID
	}
	err = d.UpdateUsersFeeds(&users[0], feedIDs)
	if err != nil {
		t.Fatalf("Error update feeds %s", err)
	}

	newFeeds, err = d.GetUsersFeeds(&users[0])
	if err != nil {
		t.Fatal(err)
	}

	if len(newFeeds) != 3 {
		t.Fatalf("Expected 3 feeds, got %d", len(newFeeds))
	}
}

func TestGetUserReport(t *testing.T) {
	t.Parallel()
	d := NewMemoryDBHandle(NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}

	ur, err := d.GetUserReport(&users[0])
	if err != nil {
		t.Fatalf("Error getting UserReport: %s", err)
	}
	if !ur.LastReport.IsZero() {
		t.Fatalf("Expected unset LastReport time, got: %s", ur.LastReport)
	}
	err = d.SetUserReport(&users[0])
	if err != nil {
		t.Fatalf("Error setting user report time: %s", err)
	}
	ur, err = d.GetUserReport(&users[0])
	if err != nil {
		t.Fatalf("Error getting UserReport: %s", err)
	}
	if ur.LastReport.IsZero() {
		t.Fatalf("Expected LastReport to be set, got: %s", ur.LastReport)
	}

}
