package db

import (
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	testdb "github.com/erikstmartin/go-testdb"
	. "github.com/onsi/gomega"
)

func NewTestDBHandle(t *testing.T, verbose bool, w bool) *DBHandle {
	db := openDB("testdb", "", verbose)
	return &DBHandle{db: db}
}

func TestConnectionError(t *testing.T) {
	RegisterTestingT(t)
	defer testdb.Reset()

	testdb.SetOpenFunc(func(dsn string) (driver.Conn, error) {
		return testdb.Conn(), errors.New("failed to connect")
	})

	Expect(func() { NewTestDBHandle(t, false, false) }).To(Panic())
}

func TestGettingFeedWithTestDB(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	LoadFixtures(t, d, "http://localhost")

	feeds, err := d.GetAllFeeds()
	Expect(err).ToNot(HaveOccurred(), "Error getting all Feeds: %s", err)
	Expect(feeds).To(HaveLen(3))
}

func TestGettingFeedsWithError(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	fixtureFeeds, _ := LoadFixtures(t, d, "http://localhost")

	feeds, err := d.GetFeedsWithErrors()
	Expect(err).ToNot(HaveOccurred(), "Error gettings feeds: %s", err)
	Expect(feeds).To(HaveLen(0))

	fixtureFeeds[0].LastPollError = "Error"
	err = d.SaveFeed(fixtureFeeds[0])
	Expect(err).ToNot(HaveOccurred(), "Error saving feed: %s", err)

	feeds, err = d.GetFeedsWithErrors()
	Expect(err).ToNot(HaveOccurred(), "Error gettings feeds: %s", err)
	Expect(feeds).To(HaveLen(1))
}

func TestGettingUsers(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	_, users := LoadFixtures(t, d, "http://localhost")

	dbusers, err := d.GetAllUsers()
	Expect(err).ToNot(HaveOccurred(), "Error gettings users: %s", err)
	Expect(dbusers).To(HaveLen(3))

	u, err := d.GetUserById(1)
	Expect(err).ToNot(HaveOccurred(), "Error gettings user by id: %s", err)
	Expect(u.Id).To(BeEquivalentTo(1))

	u, err = d.GetUserByEmail(users[0].Email)
	Expect(err).ToNot(HaveOccurred(), "Error gettings user by email: %s", err)
	Expect(u.Email).To(Equal(users[0].Email))
}

func TestGetMostRecentGuidsForFeed(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d, "http://localhost")

	Expect(d.RecordGuid(feeds[0].Id, "123")).NotTo(HaveOccurred())
	Expect(d.RecordGuid(feeds[0].Id, "1234")).NotTo(HaveOccurred())
	Expect(d.RecordGuid(feeds[0].Id, "12345")).NotTo(HaveOccurred())

	maxGuidsToFetch := 2
	guids, err := d.GetMostRecentGuidsForFeed(feeds[0].Id, maxGuidsToFetch)
	Expect(err).ToNot(HaveOccurred())
	Expect(guids).To(HaveLen(maxGuidsToFetch))
	Expect(guids).To(ConsistOf("12345", "1234"))

	guids, err = d.GetMostRecentGuidsForFeed(feeds[0].Id, -1)
	Expect(err).ToNot(HaveOccurred())
	Expect(guids).To(HaveLen(3))
}

func TestGetMostRecentGuidsForFeedWithNoRecords(t *testing.T) {
	RegisterTestingT(t)

	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d, "http://localhost")

	guids, err := d.GetMostRecentGuidsForFeed(feeds[0].Id, -1)

	Expect(err).ToNot(HaveOccurred())
	Expect(guids).To(HaveLen(0))
}

func TestAddFeedValidation(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	inputs := [][]string{
		{"good name", "bad url"},
		{"good name", "http://"},
		{"good name", ":badurl"},
		{"", ""},
	}

	for _, ins := range inputs {
		_, err := d.AddFeed(ins[0], ins[1])
		Expect(err).To(HaveOccurred())
	}
}
func TestFeedValidation(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	inputs := []FeedInfo{
		{
			Name: "",
			Url:  "bad url",
		},
		{},
		{Url: ":badurl"},
	}

	for _, f := range inputs {
		err := d.SaveFeed(&f)
		Expect(err).ToNot(HaveOccurred())
		Expect(f.Id).To(BeZero())
	}
}

func TestAddAndDeleteFeed(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	_, users := LoadFixtures(t, d, "http://localhost")
	f, err := d.AddFeed("test feed", "http://valid/url.xml")
	Expect(err).ToNot(HaveOccurred())
	Expect(f.Id).ToNot(BeZero())

	dupFeed, err := d.AddFeed("test feed", "http://valid/url.xml")
	Expect(err).To(HaveOccurred())
	Expect(dupFeed.Id).To(BeZero())

	err = d.RecordGuid(f.Id, "testGUID")
	Expect(err).ToNot(HaveOccurred())
	err = d.AddFeedsToUser(users[0], []*FeedInfo{f})
	Expect(err).ToNot(HaveOccurred())

	err = d.RemoveFeed(f.Url)
	Expect(err).ToNot(HaveOccurred())

	_, err = d.GetFeedByUrl(f.Url)
	Expect(err).To(HaveOccurred())

	guids, err := d.GetMostRecentGuidsForFeed(f.Id, -1)
	Expect(err).ToNot(HaveOccurred())
	Expect(guids).To(BeEmpty())

	dbusers, err := d.GetFeedUsers(f.Url)
	Expect(err).ToNot(HaveOccurred())
	Expect(dbusers).To(BeEmpty())
}

func TestGetFeedItemByGuid(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d, "http://localhost")
	err := d.RecordGuid(feeds[0].Id, "feed0GUID")
	Expect(err).ToNot(HaveOccurred())
	err = d.RecordGuid(feeds[1].Id, "feed1GUID")
	Expect(err).ToNot(HaveOccurred())

	guid, err := d.GetFeedItemByGuid(feeds[0].Id, "feed0GUID")
	Expect(err).ToNot(HaveOccurred())
	Expect(guid.FeedInfoId).To(BeEquivalentTo(1))

	Expect(guid.Guid).To(Equal("feed0GUID"))
}

func TestRemoveUserByEmail(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	_, users := LoadFixtures(t, d, "http://localhost")
	err := d.RemoveUserByEmail(users[0].Email)
	Expect(err).ToNot(HaveOccurred())
}

func TestGetStaleFeeds(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d, "http://localhost")
	d.RecordGuid(feeds[0].Id, "foobar")
	d.RecordGuid(feeds[1].Id, "foobaz")
	d.RecordGuid(feeds[2].Id, "foobaz")
	guid, err := d.GetFeedItemByGuid(feeds[0].Id, "foobar")
	Expect(err).ToNot(HaveOccurred())
	guid.AddedOn = *new(time.Time)
	err = d.db.Save(guid).Error
	Expect(err).ToNot(HaveOccurred())
	f, err := d.GetStaleFeeds()
	Expect(err).ToNot(HaveOccurred())
	Expect(f[0].Id).To(BeEquivalentTo(feeds[0].Id))
}

func TestAddUserValidation(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)

	inputs := [][]string{
		{"test", ".bad@address"},
		{"test", ""},
	}
	for _, ins := range inputs {
		_, err := d.AddUser(ins[0], ins[1], "pass")
		Expect(err).To(HaveOccurred())
	}

	_, err := d.AddUser("", "email@address.com", "pass")
	Expect(err).To(HaveOccurred())

	u, err := d.AddUser("new user", "newuser@example.com", "pass")
	Expect(err).ToNot(HaveOccurred())
	Expect(u.Id).ToNot(BeZero())
}

func TestAddRemoveUser(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d, "http://localhost")

	userName := "test user name"
	userEmail := "testuser_name@example.com"
	u, err := d.AddUser(userName, userEmail, "pass")
	Expect(err).ToNot(HaveOccurred())
	Expect(u.Id).ToNot(BeZero())

	dupUser, err := d.AddUser(userName, "extra"+userEmail, "pass")
	Expect(err).To(HaveOccurred())
	Expect(dupUser.Id).To(BeZero())

	dupUser, err = d.AddUser("extra"+userName, userEmail, "pass")
	Expect(err).To(HaveOccurred())
	Expect(dupUser.Id).To(BeZero())

	dbUser, err := d.GetUser(u.Name)
	Expect(err).ToNot(HaveOccurred())
	Expect(dbUser).To(BeEquivalentTo(u))

	err = d.AddFeedsToUser(u, []*FeedInfo{feeds[0]})
	Expect(err).ToNot(HaveOccurred())

	err = d.RemoveUser(u)
	Expect(err).ToNot(HaveOccurred())

	dbfeeds, err := d.GetUsersFeeds(u)
	Expect(err).ToNot(HaveOccurred())
	Expect(dbfeeds).To(BeEmpty())
}

func TestAddRemoveFeedsFromUser(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	_, users := LoadFixtures(t, d, "http://localhost")
	newFeed := &FeedInfo{
		Name: "new test feed",
		Url:  "http://new/test.feed",
	}
	err := d.SaveFeed(newFeed)
	Expect(err).ToNot(HaveOccurred(), "Error saving feed: %s", err)
	feeds, err := d.GetUsersFeeds(users[0])

	Expect(err).ToNot(HaveOccurred(), "error getting users feeds: %s", err)

	Expect(feeds).To(HaveLen(3))
	err = d.AddFeedsToUser(users[0], []*FeedInfo{newFeed})
	Expect(err).ToNot(HaveOccurred(), "error adding feed to user: %s", err)
	feeds, err = d.GetUsersFeeds(users[0])
	Expect(err).ToNot(HaveOccurred(), "error getting users feeds: %s", err)
	Expect(feeds).To(HaveLen(4))

	// Test that we don't add duplicates
	err = d.AddFeedsToUser(users[0], []*FeedInfo{newFeed})
	Expect(err).ToNot(HaveOccurred(), "error adding feed to user: %s", err)
	feeds, err = d.GetUsersFeeds(users[0])
	Expect(err).ToNot(HaveOccurred(), "error getting users feeds: %s", err)
	Expect(feeds).To(HaveLen(4))

	err = d.RemoveFeedsFromUser(users[0], []*FeedInfo{newFeed})
	Expect(err).ToNot(HaveOccurred(), "error removing users feed: %s", err)
	feeds, err = d.GetUsersFeeds(users[0])
	Expect(err).ToNot(HaveOccurred(), "error getting users feeds: %s", err)
	Expect(feeds).To(HaveLen(3))
}

func TestGetUsersFeeds(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d, "http://localhost")
	userFeeds, err := d.GetUsersFeeds(users[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(userFeeds).To(HaveLen(len(feeds)))
}

func TestGetFeedUsers(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d, "http://localhost")
	feedUsers, err := d.GetFeedUsers(feeds[0].Url)
	Expect(err).ToNot(HaveOccurred())
	Expect(feedUsers).To(HaveLen(len(users)))
}

func TestUpdateUsersFeeds(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d, "http://localhost")

	dbFeeds, err := d.GetUsersFeeds(users[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(dbFeeds).ToNot(BeEmpty())

	err = d.UpdateUsersFeeds(users[0], []int64{})
	Expect(err).ToNot(HaveOccurred())
	newFeeds, err := d.GetUsersFeeds(users[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(newFeeds).To(BeEmpty())
	feedIDs := make([]int64, len(feeds))
	for i := range feeds {
		feedIDs[i] = feeds[i].Id
	}
	d.UpdateUsersFeeds(users[0], feedIDs)

	newFeeds, err = d.GetUsersFeeds(users[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(newFeeds).To(HaveLen(3))
}
