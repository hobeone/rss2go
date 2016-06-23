package db

import (
	"database/sql/driver"
	"errors"
	"io/ioutil"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	testdb "github.com/erikstmartin/go-testdb"
	. "github.com/onsi/gomega"
)

func NullLogger() logrus.FieldLogger {
	l := logrus.New()
	l.Out = ioutil.Discard
	return l
}

func NewTestDBHandle(t *testing.T, verbose bool, w bool) *Handle {
	db := openDB("testdb", "", verbose, NullLogger())
	return &Handle{db: db}
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
	d := NewMemoryDBHandle(false, NullLogger(), true)

	feeds, err := d.GetAllFeeds()
	Expect(err).ShouldNot(HaveOccurred(), "Error getting all Feeds: %s", err)
	Expect(feeds).To(HaveLen(3))
}

func TestGettingFeedsWithError(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)

	allFeeds, err := d.GetAllFeeds()
	Expect(err).ToNot(HaveOccurred(), "Error getting all Feeds: %s", err)

	feeds, err := d.GetFeedsWithErrors()
	Expect(err).ToNot(HaveOccurred(), "Error gettings feeds: %s", err)
	Expect(feeds).To(HaveLen(0))

	allFeeds[0].LastPollError = "Error"
	err = d.SaveFeed(&allFeeds[0])
	Expect(err).ToNot(HaveOccurred(), "Error saving feed: %s", err)

	feeds, err = d.GetFeedsWithErrors()
	Expect(err).ToNot(HaveOccurred(), "Error gettings feeds: %s", err)
	Expect(feeds).To(HaveLen(1))
}

func TestGettingUsers(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)

	dbusers, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting all users: %v", err)
	}
	Expect(dbusers).To(HaveLen(3))

	u, err := d.GetUserByID(1)
	Expect(err).ToNot(HaveOccurred(), "Error gettings user by id: %s", err)
	Expect(u.ID).To(BeEquivalentTo(1))

	addr := "test1@example.com"
	u, err = d.GetUserByEmail(addr)
	Expect(err).ToNot(HaveOccurred(), "Error gettings user by email: %s", err)
	Expect(u.Email).To(Equal(addr))
}

func TestGetMostRecentGuidsForFeed(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)

	Expect(d.RecordGUID(1, "123")).NotTo(HaveOccurred())
	Expect(d.RecordGUID(1, "1234")).NotTo(HaveOccurred())
	Expect(d.RecordGUID(1, "12345")).NotTo(HaveOccurred())

	maxGuidsToFetch := 2
	guids, err := d.GetMostRecentGUIDsForFeed(1, maxGuidsToFetch)
	Expect(err).ToNot(HaveOccurred())
	Expect(guids).To(HaveLen(maxGuidsToFetch))
	Expect(guids).To(ConsistOf("12345", "1234"))

	guids, err = d.GetMostRecentGUIDsForFeed(1, -1)
	Expect(err).ToNot(HaveOccurred())
	Expect(guids).To(HaveLen(3))
}

func TestGetMostRecentGuidsForFeedWithNoRecords(t *testing.T) {
	RegisterTestingT(t)

	d := NewMemoryDBHandle(false, NullLogger(), true)

	guids, err := d.GetMostRecentGUIDsForFeed(1, -1)

	Expect(err).ToNot(HaveOccurred())
	Expect(guids).To(HaveLen(0))
}

func TestAddFeedValidation(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
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
	d := NewMemoryDBHandle(false, NullLogger(), true)
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
		Expect(err).ToNot(HaveOccurred())
		Expect(f.ID).To(BeZero())
	}
}

func TestAddAndDeleteFeed(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
	u, err := d.GetUserByID(1)
	if err != nil {
		t.Fatalf("Error getting user: %v", err)
	}
	f, err := d.AddFeed("test feed", "http://valid/url.xml")
	Expect(err).ToNot(HaveOccurred())
	Expect(f.ID).ToNot(BeZero())

	dupFeed, err := d.AddFeed("test feed", "http://valid/url.xml")
	Expect(err).To(HaveOccurred())
	Expect(dupFeed.ID).To(BeZero())

	err = d.RecordGUID(f.ID, "testGUID")
	Expect(err).ToNot(HaveOccurred())
	err = d.AddFeedsToUser(u, []*FeedInfo{f})
	Expect(err).ToNot(HaveOccurred())

	err = d.RemoveFeed(f.URL)
	if err != nil {
		t.Fatalf("Error removing feed: %s", err)
	}

	_, err = d.GetFeedByURL(f.URL)
	Expect(err).To(HaveOccurred())

	guids, err := d.GetMostRecentGUIDsForFeed(f.ID, -1)
	Expect(err).ToNot(HaveOccurred())
	Expect(guids).To(BeEmpty())

	dbusers, err := d.GetFeedUsers(f.URL)
	Expect(err).ToNot(HaveOccurred())
	Expect(dbusers).To(BeEmpty())
}

func TestGetFeedItemByGuid(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
	err := d.RecordGUID(1, "feed0GUID")
	Expect(err).ToNot(HaveOccurred())
	err = d.RecordGUID(2, "feed1GUID")
	Expect(err).ToNot(HaveOccurred())

	guid, err := d.GetFeedItemByGUID(1, "feed0GUID")
	Expect(err).ToNot(HaveOccurred())
	Expect(guid.FeedInfoID).To(BeEquivalentTo(1))

	Expect(guid.GUID).To(Equal("feed0GUID"))
}

func TestRemoveUserByEmail(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
	err := d.RemoveUserByEmail("test1@example.com")
	Expect(err).ToNot(HaveOccurred())
}

func TestGetStaleFeeds(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
	d.RecordGUID(1, "foobar")
	d.RecordGUID(2, "foobaz")
	d.RecordGUID(3, "foobaz")
	guid, err := d.GetFeedItemByGUID(1, "foobar")
	if err != nil {
		t.Fatalf("Got unexpected error from db: %s", err)
	}
	guid.AddedOn = *new(time.Time)
	err = d.db.Save(guid).Error
	Expect(err).ToNot(HaveOccurred())
	f, err := d.GetStaleFeeds()
	if err != nil {
		t.Fatalf("Got unexpected error from db: %s", err)
	}

	Expect(f[0].ID).To(BeEquivalentTo(1))
}

func TestAddUserValidation(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)

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
	Expect(u.ID).ToNot(BeZero())
}

func TestAddRemoveUser(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)

	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}

	userName := "test user name"
	userEmail := "testuser_name@example.com"
	u, err := d.AddUser(userName, userEmail, "pass")
	Expect(err).ToNot(HaveOccurred())
	Expect(u.ID).ToNot(BeZero())

	dupUser, err := d.AddUser(userName, "extra"+userEmail, "pass")
	Expect(err).To(HaveOccurred())
	Expect(dupUser.ID).To(BeZero())

	dupUser, err = d.AddUser("extra"+userName, userEmail, "pass")
	Expect(err).To(HaveOccurred())
	Expect(dupUser.ID).To(BeZero())

	dbUser, err := d.GetUser(u.Name)
	Expect(err).ToNot(HaveOccurred())
	Expect(dbUser).To(BeEquivalentTo(u))

	err = d.AddFeedsToUser(u, []*FeedInfo{&feeds[0]})
	Expect(err).ToNot(HaveOccurred())

	err = d.RemoveUser(u)
	Expect(err).ToNot(HaveOccurred())

	dbfeeds, err := d.GetUsersFeeds(u)
	Expect(err).ToNot(HaveOccurred())
	Expect(dbfeeds).To(BeEmpty())
}

func TestAddRemoveFeedsFromUser(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	newFeed := &FeedInfo{
		Name: "new test feed",
		URL:  "http://new/test.feed",
	}
	err = d.SaveFeed(newFeed)
	Expect(err).ToNot(HaveOccurred(), "Error saving feed: %s", err)
	feeds, err := d.GetUsersFeeds(&users[0])

	Expect(err).ToNot(HaveOccurred(), "error getting users feeds: %s", err)

	Expect(feeds).To(HaveLen(3))
	err = d.AddFeedsToUser(&users[0], []*FeedInfo{newFeed})
	Expect(err).ToNot(HaveOccurred(), "error adding feed to user: %s", err)
	feeds, err = d.GetUsersFeeds(&users[0])
	Expect(err).ToNot(HaveOccurred(), "error getting users feeds: %s", err)
	Expect(feeds).To(HaveLen(4))

	// Test that we don't add duplicates
	err = d.AddFeedsToUser(&users[0], []*FeedInfo{newFeed})
	Expect(err).ToNot(HaveOccurred(), "error adding feed to user: %s", err)
	feeds, err = d.GetUsersFeeds(&users[0])
	Expect(err).ToNot(HaveOccurred(), "error getting users feeds: %s", err)
	Expect(feeds).To(HaveLen(4))

	err = d.RemoveFeedsFromUser(&users[0], []*FeedInfo{newFeed})
	Expect(err).ToNot(HaveOccurred(), "error removing users feed: %s", err)
	feeds, err = d.GetUsersFeeds(&users[0])
	Expect(err).ToNot(HaveOccurred(), "error getting users feeds: %s", err)
	Expect(feeds).To(HaveLen(3))
}

func TestGetUsersFeeds(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}

	userFeeds, err := d.GetUsersFeeds(&users[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(userFeeds).To(HaveLen(len(feeds)))
}

func TestGetFeedUsers(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}
	feedUsers, err := d.GetFeedUsers(feeds[0].URL)
	Expect(err).ToNot(HaveOccurred())
	Expect(feedUsers).To(HaveLen(len(users)))
}

func TestUpdateUsersFeeds(t *testing.T) {
	RegisterTestingT(t)
	d := NewMemoryDBHandle(false, NullLogger(), true)
	users, err := d.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	feeds, err := d.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}

	dbFeeds, err := d.GetUsersFeeds(&users[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(dbFeeds).ToNot(BeEmpty())

	err = d.UpdateUsersFeeds(&users[0], []int64{})
	Expect(err).ToNot(HaveOccurred())
	newFeeds, err := d.GetUsersFeeds(&users[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(newFeeds).To(BeEmpty())
	feedIDs := make([]int64, len(feeds))
	for i := range feeds {
		feedIDs[i] = feeds[i].ID
	}
	d.UpdateUsersFeeds(&users[0], feedIDs)

	newFeeds, err = d.GetUsersFeeds(&users[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(newFeeds).To(HaveLen(3))
}
