package db

import (
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	testdb "github.com/erikstmartin/go-testdb"
	. "github.com/smartystreets/goconvey/convey"
)

func NewTestDBHandle(t *testing.T, verbose bool, w bool) *DBHandle {
	db := openDB("testdb", "", verbose)
	return &DBHandle{db: db}
}

func TestConnectionError(t *testing.T) {
	defer testdb.Reset()

	testdb.SetOpenFunc(func(dsn string) (driver.Conn, error) {
		return testdb.Conn(), errors.New("failed to connect")
	})

	Convey("Given database open error", t, func() {
		So(func() { NewTestDBHandle(t, false, false) }, ShouldPanic)
	})
}

func TestGettingFeed(t *testing.T) {
	Convey("Get All Feeds", t, func() {
		d := NewTestDBHandle(t, false, true)
		Convey("Should get all feeds", func() {
			sql := "SELECT * FROM `feed_info`"
			columns := []string{"id", "name", "url"}
			result := `
			1,testfeed1,http://testfeed1/feed.atom
			2,testfeed2,http://testfeed2/feed.rss
			3,testfeed3,http://testfeed3/feed.rdf
			`
			testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))

			feeds, err := d.GetAllFeeds()
			So(err, ShouldBeNil)
			So(len(feeds), ShouldEqual, 3)
		})
	})
	Convey("GetFeedsWithErrors", t, func() {
		d := NewMemoryDBHandle(false, true)
		fixtureFeeds, _ := LoadFixtures(t, d, "http://localhost")

		feeds, err := d.GetFeedsWithErrors()
		So(err, ShouldBeNil)
		So(len(feeds), ShouldEqual, 0)

		fixtureFeeds[0].LastPollError = "Error"
		err = d.SaveFeed(fixtureFeeds[0])
		So(err, ShouldBeNil)

		feeds, err = d.GetFeedsWithErrors()
		So(err, ShouldBeNil)
		So(len(feeds), ShouldEqual, 1)
	})
}

func TestGettingUsers(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	_, users := LoadFixtures(t, d, "http://localhost")

	Convey("GetAllUsers", t, func() {
		Convey("Should get all users", func() {
			users, err := d.GetAllUsers()
			So(err, ShouldBeNil)
			So(len(users), ShouldEqual, 3)
		})
	})
	Convey("GetUserById", t, func() {
		Convey("Should get user given an Id", func() {
			u, err := d.GetUserById(1)
			So(err, ShouldBeNil)
			So(u.Id, ShouldEqual, 1)
		})
	})
	Convey("GetUserByEmail", t, func() {
		Convey("Should get user given an email", func() {
			u, err := d.GetUserByEmail(users[0].Email)
			So(err, ShouldBeNil)
			So(u.Email, ShouldEqual, users[0].Email)
		})
	})

}

func TestGetMostRecentGuidsForFeed(t *testing.T) {
	Convey("Given new GUIDs", t, func() {
		d := NewMemoryDBHandle(false, true)
		feeds, _ := LoadFixtures(t, d, "http://localhost")

		So(d.RecordGuid(feeds[0].Id, "123"), ShouldBeNil)
		So(d.RecordGuid(feeds[0].Id, "1234"), ShouldBeNil)
		So(d.RecordGuid(feeds[0].Id, "12345"), ShouldBeNil)
		Convey("Only get the 2 most recent", func() {
			maxGuidsToFetch := 2
			guids, err := d.GetMostRecentGuidsForFeed(feeds[0].Id, maxGuidsToFetch)
			So(err, ShouldBeNil)
			So(len(guids), ShouldEqual, maxGuidsToFetch)
			So(guids, ShouldResemble, []string{"12345", "1234"})
		})
		Convey("Get all Guids", func() {
			guids, err := d.GetMostRecentGuidsForFeed(feeds[0].Id, -1)
			So(err, ShouldBeNil)
			So(len(guids), ShouldEqual, 3)
		})
	})
	Convey("Given no Records", t, func() {
		d := NewMemoryDBHandle(false, true)
		feeds, _ := LoadFixtures(t, d, "http://localhost")

		Convey("Get all Guids", func() {
			guids, err := d.GetMostRecentGuidsForFeed(feeds[0].Id, -1)
			So(err, ShouldBeNil)
			So(len(guids), ShouldEqual, 0)
		})
	})
}

func TestFeedValidation(t *testing.T) {
	Convey("FeedInfo should validate attributes before saving", t, func() {
		Convey("When a bad arguments are given it should return an error", func() {
			d := NewMemoryDBHandle(false, true)
			inputs := [][]string{
				{"good name", "bad url"},
				{"good name", "http://"},
				{"good name", ":badurl"},
				{"", ""},
			}

			for _, ins := range inputs {
				_, err := d.AddFeed(ins[0], ins[1])
				So(err, ShouldNotBeNil)
			}
		})
		Convey("When an Invalid FeedInfo is given it should return an error", func() {
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
				So(err, ShouldNotBeNil)
				So(f.Id, ShouldBeZeroValue)
			}
		})
	})
}

func TestAddAndDeleteFeed(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	_, users := LoadFixtures(t, d, "http://localhost")
	f, err := d.AddFeed("test feed", "http://valid/url.xml")
	Convey("Subject: Add and Delete FeedInfo", t, func() {
		Convey("When created", func() {
			Convey("The Feed Should be created", func() {
				So(err, ShouldBeNil)
				So(f.Id, ShouldBeGreaterThan, 1)
			})
			Convey("should fail on duplicate name", func() {
				dupFeed, err := d.AddFeed("test feed", "http://valid/url.xml")
				So(err, ShouldNotBeNil)
				So(dupFeed.Id, ShouldBeZeroValue)
			})
		})
		Convey("When deleted", func() {
			err := d.RecordGuid(f.Id, "testGUID")
			So(err, ShouldBeNil)
			err = d.AddFeedsToUser(users[0], []*FeedInfo{f})
			So(err, ShouldBeNil)
			Convey("should also remove GUIDs and subscriptions", func() {
				err := d.RemoveFeed(f.Url)
				So(err, ShouldBeNil)
				_, err = d.GetFeedByUrl(f.Url)
				So(err, ShouldNotBeNil)
				guids, err := d.GetMostRecentGuidsForFeed(f.Id, -1)
				So(err, ShouldBeNil)
				So(guids, ShouldBeEmpty)
				users, err := d.GetFeedUsers(f.Url)
				So(err, ShouldBeNil)
				So(users, ShouldBeEmpty)
			})
		})
	})
}

func TestGetFeedItemByGuid(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d, "http://localhost")
	Convey("Subject: Get FeedItem by GUID", t, func() {
		Convey("should create GUID", func() {
			err := d.RecordGuid(feeds[0].Id, "feed0GUID")
			So(err, ShouldBeNil)
			err = d.RecordGuid(feeds[1].Id, "feed1GUID")
			So(err, ShouldBeNil)
		})
		Convey("should get by GUID", func() {
			guid, err := d.GetFeedItemByGuid(feeds[0].Id, "feed0GUID")
			So(err, ShouldBeNil)
			So(guid.FeedInfoId, ShouldEqual, 1)
			So(guid.Guid, ShouldEqual, "feed0GUID")
		})
	})
}

func TestRemoveUserByEmail(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	_, users := LoadFixtures(t, d, "http://localhost")
	Convey("Subject: User Model", t, func() {
		Convey("Should be able to delete by Email", func() {
			err := d.RemoveUserByEmail(users[0].Email)
			So(err, ShouldBeNil)
		})
	})
}

func TestGetStaleFeeds(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, _ := LoadFixtures(t, d, "http://localhost")
	Convey("GetStaleFeeds should return stale feed", t, func() {
		d.RecordGuid(feeds[0].Id, "foobar")
		d.RecordGuid(feeds[1].Id, "foobaz")
		d.RecordGuid(feeds[2].Id, "foobaz")
		guid, err := d.GetFeedItemByGuid(feeds[0].Id, "foobar")
		So(err, ShouldBeNil)
		Convey("When all of a feed Items are older than 14 days", func() {
			guid.AddedOn = *new(time.Time)
			err = d.db.Save(guid).Error
			So(err, ShouldBeNil)
			Convey("The feed should be returned by GetStaleFeeds", func() {
				f, err := d.GetStaleFeeds()
				So(err, ShouldBeNil)
				So(&f[0], ShouldResemble, feeds[0])
			})
		})

	})
}

func TestAddUserValidation(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	Convey("User attributes are validates before saving", t, func() {
		Convey("When invalid email address", func() {
			inputs := [][]string{
				{"test", ".bad@address"},
				{"test", ""},
			}
			Convey("AddUser should return an error", func() {
				for _, ins := range inputs {
					_, err := d.AddUser(ins[0], ins[1], "pass")
					So(err, ShouldNotBeNil)
				}
			})
		})
		Convey("When invalid name", func() {
			Convey("AddUser Should return an error", func() {
				_, err := d.AddUser("", "email@address.com", "pass")
				So(err, ShouldNotBeNil)
			})
		})
		Convey("When valid name and email", func() {
			Convey("AddUser should return a new saved User", func() {
				u, err := d.AddUser("new user", "newuser@example.com", "pass")
				So(err, ShouldBeNil)
				So(u.Id, ShouldBeGreaterThanOrEqualTo, 1)
			})
		})
	})
}

func TestAddRemoveUser(t *testing.T) {
	d := NewMemoryDBHandle(true, true)
	feeds, _ := LoadFixtures(t, d, "http://localhost")

	userName := "test user name"
	userEmail := "testuser_name@example.com"
	u, err := d.AddUser(userName, userEmail, "pass")

	Convey("AddUser should create users", t, func() {
		So(err, ShouldBeNil)
		So(u.Id, ShouldBeGreaterThan, 0)
		Convey("When creating a duplicate user name", func() {
			dupUser, err := d.AddUser(userName, "extra"+userEmail, "pass")
			Convey("AddUser should return an error", func() {
				So(err, ShouldNotBeNil)
				So(dupUser.Id, ShouldBeZeroValue)
			})
		})
		Convey("When creating a duplicate email", func() {
			dupUser, err := d.AddUser("extra"+userName, userEmail, "pass")
			Convey("AddUser should return an error", func() {
				So(err, ShouldNotBeNil)
				So(dupUser.Id, ShouldBeZeroValue)
			})
		})
		Convey("When searching by just created Name", func() {
			dbUser, err := d.GetUser(u.Name)
			Convey("GetUser should return created user", func() {
				So(err, ShouldBeNil)
				So(dbUser, ShouldResemble, u)
			})
		})
	})

	Convey("RemoveUser should remove user and relationships", t, func() {
		err = d.AddFeedsToUser(u, []*FeedInfo{feeds[0]})
		So(err, ShouldBeNil)

		err = d.RemoveUser(u)
		So(err, ShouldBeNil)

		Convey("When removed all UserFeeds are also removed", func() {
			feeds, err := d.GetUsersFeeds(u)
			So(err, ShouldBeNil)
			So(feeds, ShouldBeEmpty)
		})
	})
}

func TestAddRemoveFeedsFromUser(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	_, users := LoadFixtures(t, d, "http://localhost")
	newFeed := &FeedInfo{
		Name: "new test feed",
		Url:  "http://new/test.feed",
	}
	Convey("Subject: Feed add and removal from a user", t, func() {
		Convey("When adding a feed", func() {
			err := d.SaveFeed(newFeed)
			So(err, ShouldBeNil)
			feeds, err := d.GetUsersFeeds(users[0])
			So(err, ShouldBeNil)
			So(len(feeds), ShouldEqual, 3)

			err = d.AddFeedsToUser(users[0], []*FeedInfo{newFeed})
			Convey("A feed should be added to a user", func() {
				So(err, ShouldBeNil)
				feeds, err := d.GetUsersFeeds(users[0])
				So(err, ShouldBeNil)
				So(feeds, ShouldNotBeEmpty)
				So(len(feeds), ShouldEqual, 4)
			})
		})

		Convey("When removing a feed", func() {
			err := d.RemoveFeedsFromUser(users[0], []*FeedInfo{newFeed})
			So(err, ShouldBeNil)
			feeds, err := d.GetUsersFeeds(users[0])
			So(err, ShouldBeNil)
			So(feeds, ShouldNotContain, newFeed)
		})

	})
}

func TestGetUsersFeeds(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d, "http://localhost")
	Convey("GetFeedsWithUsers should return all of a users feeds ", t, func() {
		userFeeds, err := d.GetUsersFeeds(users[0])
		So(err, ShouldBeNil)
		So(len(userFeeds), ShouldEqual, len(feeds))
	})
}

func TestGetFeedUsers(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d, "http://localhost")
	Convey("GetFeedUsers should return all users subscribed to a feed", t, func() {
		feedUsers, err := d.GetFeedUsers(feeds[0].Url)
		So(err, ShouldBeNil)
		So(len(feedUsers), ShouldEqual, len(users))
	})
}

func TestUpdateUsersFeeds(t *testing.T) {
	d := NewMemoryDBHandle(false, true)
	feeds, users := LoadFixtures(t, d, "http://localhost")

	dbFeeds, err := d.GetUsersFeeds(users[0])
	Convey("Subject: UpdateUsersFeeds", t, func() {
		So(err, ShouldBeNil)
		Convey("UpdateUsersFeeds should replace the current feeds", func() {
			So(len(dbFeeds), ShouldNotEqual, 0)
			err := d.UpdateUsersFeeds(users[0], []int64{})
			So(err, ShouldBeNil)
			newFeeds, err := d.GetUsersFeeds(users[0])
			So(err, ShouldBeNil)
			So(len(newFeeds), ShouldEqual, 0)
			feedIDs := make([]int64, len(feeds))
			for i := range feeds {
				feedIDs[i] = feeds[i].Id
			}
			d.UpdateUsersFeeds(users[0], feedIDs)

			newFeeds, err = d.GetUsersFeeds(users[0])
			So(err, ShouldBeNil)
			So(len(newFeeds), ShouldEqual, 3)
		})
	})
}
