package db

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"sync"
	"time"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/golang/glog"
	"github.com/jinzhu/gorm"

	// Import sqlite3 driver
	_ "github.com/mattn/go-sqlite3"
)

// FeedInfo represents a feed (atom, rss or rdf) that rss2go is polling.
type FeedInfo struct {
	Id            int64     `json:"id"`
	Name          string    `sql:"not null;unique" json:"name" binding:"required"`
	Url           string    `sql:"not null;unique" json:"url" binding:"required"`
	LastPollTime  time.Time `json:"lastPollTime"`
	LastPollError string    `json:"lastPollError"`
}

// FeedItem represents an individual iteam from a feed.  It only captures the
// Guid for that item and is mainly used to check if a particular item has been
// seen before.
type FeedItem struct {
	Id         int64
	FeedInfoId int64     `sql:"not null"`
	Guid       string    `sql:"not null"`
	AddedOn    time.Time `sql:"not null"`
}

// User represents a user/email address that can subscribe to Feeds
type User struct {
	Id       int64  `json:"id"`
	Name     string `sql:"size:255;not null;unique" json:"name"`
	Email    string `sql:"size:255;not null;unique" json:"email"`
	Enabled  bool   `json:"enabled"`
	Password string `json:"-"`
}

// UserFeed maps between Users and Feeds
type UserFeed struct {
	Id     int64
	UserId int64 `sql:"not null"`
	FeedId int64 `sql:"not null"`
}

// DBHandle controls access to the database and makes sure only one
// operation is in process at a time.
type DBHandle struct {
	DB           gorm.DB
	writeUpdates bool
	syncMutex    sync.Mutex
}

func openDB(dbType string, dbArgs string, verbose bool) gorm.DB {
	glog.Infof("Opening database %s:%s", dbType, dbArgs)
	// Error only returns from this if it is an unknown driver.
	d, err := gorm.Open(dbType, dbArgs)
	if err != nil {
		panic(err.Error())
	}
	d.SingularTable(true)
	d.LogMode(verbose)
	// Actually test that we have a working connection
	err = d.DB().Ping()
	if err != nil {
		panic(err.Error())
	}
	return d
}

func setupDB(db gorm.DB) error {
	models := []interface{}{User{}, UserFeed{}, FeedInfo{}, FeedItem{}}
	tx := db.Begin()
	for _, m := range models {
		err := tx.AutoMigrate(m).Error
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}

func createAndOpenDb(dbPath string, verbose bool, memory bool) *DBHandle {
	mode := "rwc"
	if memory {
		mode = "memory"
	}
	constructedPath := fmt.Sprintf("file:%s?mode=%s", dbPath, mode)
	db := openDB("sqlite3", constructedPath, verbose)
	err := setupDB(db)
	if err != nil {
		panic(err.Error())
	}
	return &DBHandle{DB: db}
}

// NewDBHandle creates a new DBHandle
//	dbPath: the path to the database to use.
//	verbose: when true database accesses are logged to stdout
//	writeUpdates: when true actually write to the databse (useful for testing)
func NewDBHandle(dbPath string, verbose bool, writeUpdates bool) *DBHandle {
	d := createAndOpenDb(dbPath, verbose, false)
	d.writeUpdates = writeUpdates
	return d
}

// NewMemoryDBHandle creates a new in memory database.  Useful for testing.
func NewMemoryDBHandle(verbose bool, writeUpdates bool) *DBHandle {
	d := createAndOpenDb("in_memory_test", verbose, true)
	d.writeUpdates = writeUpdates
	return d
}

/*
*
* FeedInfo related functions
*
 */

func validateFeed(f *FeedInfo) error {
	if f.Name == "" || f.Url == "" {
		return errors.New("name and url can't be empty")
	}
	u, err := url.Parse(f.Url)
	if err != nil {
		return fmt.Errorf("invalid URL: %s", err)
	} else if u.Scheme == "" {
		return errors.New("URL has no Scheme")
	} else if u.Host == "" {
		return errors.New("URL has no Host")
	}

	f.Url = u.String()
	return nil
}

// AddFeed creates a new FeedInfo entry in the database and returns it.
func (d *DBHandle) AddFeed(name string, feedURL string) (*FeedInfo, error) {
	f := &FeedInfo{
		Name: name,
		Url:  feedURL,
	}
	err := validateFeed(f)
	if err != nil {
		return nil, err
	}

	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	err = d.DB.Save(f).Error
	return f, err
}

//SaveFeed creates or updates the given FeedInfo in the database.
func (d *DBHandle) SaveFeed(f *FeedInfo) error {
	err := validateFeed(f)
	if err != nil {
		return err
	}

	if d.writeUpdates {
		return d.DB.Save(f).Error
	}
	return nil
}

// RemoveFeed removes a FeedInfo from the database given it's URL.  Optioanlly
// it will also remove all of the GUIDs for that FeedInfo.
func (d *DBHandle) RemoveFeed(url string) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	f, err := d.unsafeGetFeedByURL(url)

	if err != nil {
		return err
	}
	tx := d.DB.Begin()
	err = tx.Delete(f).Error
	if err == nil {
		err = tx.Where("feed_info_id = ?", f.Id).Delete(FeedItem{}).Error
		if err != nil {
			tx.Rollback()
			return err
		}
		err = tx.Where("feed_id = ?", f.Id).Delete(UserFeed{}).Error
		if err != nil {
			tx.Rollback()
			return err
		}

	}
	tx.Commit()
	return err
}

// GetAllFeeds returns all feeds from the database.
func (d *DBHandle) GetAllFeeds() (feeds []FeedInfo, err error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	err = d.DB.Find(&feeds).Error
	return
}

// GetFeedsWithErrors returns all feeds that had an error on their last
// check.
func (d *DBHandle) GetFeedsWithErrors() ([]FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var feeds []FeedInfo
	err := d.DB.Where(
		"last_poll_error IS NOT NULL and last_poll_error <> ''").Find(&feeds).Error
	if err == gorm.RecordNotFound {
		err = nil
	}

	return feeds, err
}

// GetStaleFeeds returns all feeds that haven't gotten new content in more
// than 14 days.
func (d *DBHandle) GetStaleFeeds() ([]FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var res []FeedInfo
	err := d.DB.Raw(`
	select feed_info.id, feed_info.name, feed_info.url,  r.MaxTime, feed_info.last_poll_error FROM (SELECT feed_info_id, MAX(added_on) as MaxTime FROM feed_item GROUP BY feed_info_id) r, feed_info INNER JOIN feed_item f ON f.feed_info_id = r.feed_info_id AND f.added_on = r.MaxTime AND r.MaxTime < datetime('now','-14 days') AND f.feed_info_id = feed_info.id group by f.feed_info_id;
	`).Scan(&res).Error
	return res, err
}

// GetFeedById returns the FeedInfo for the given id.  It returns a
// gorm.RecordNotFound error if it doesn't exist.
func (d *DBHandle) GetFeedById(id int64) (*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var f FeedInfo
	err := d.DB.First(&f, id).Error
	return &f, err
}

// GetFeedByUrl returns the FeedInfo for the given URL.  It returns a
// gorm.RecordNotFound error if it doesn't exist.
func (d *DBHandle) GetFeedByUrl(url string) (*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.unsafeGetFeedByURL(url)
}

func (d *DBHandle) unsafeGetFeedByURL(url string) (*FeedInfo, error) {
	feed := FeedInfo{}
	err := d.DB.Where("url = ?", url).First(&feed).Error
	return &feed, err
}

/*
*
* FeedItem related functions
*
 */

// GetFeedItemByGuid returns a FeedItem for the given FeedInfo.Id and guid.
func (d *DBHandle) GetFeedItemByGuid(feedID int64, guid string) (*FeedItem, error) {
	//TODO: see if beedb will handle this correctly and protect against injection
	//attacks.
	fi := FeedItem{}
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	err := d.DB.Where("feed_info_id = ? AND guid = ?", feedID, guid).First(&fi).Error
	return &fi, err
}

// RecordGuid adds a FeedItem record for the given feedID and guid.
func (d *DBHandle) RecordGuid(feedID int64, guid string) (err error) {
	if d.writeUpdates {
		glog.Infof("Adding GUID '%s' for feed %d", guid, feedID)
		f := FeedItem{
			FeedInfoId: feedID,
			Guid:       guid,
			AddedOn:    time.Now(),
		}
		d.syncMutex.Lock()
		defer d.syncMutex.Unlock()

		return d.DB.Save(&f).Error
	}
	return nil
}

// GetMostRecentGuidsForFeed retrieves the most recent GUIDs for a given feed
// up to max.  GUIDs are returned ordered with the most recent first.
func (d *DBHandle) GetMostRecentGuidsForFeed(feedID int64, max int) ([]string, error) {
	var items []FeedItem
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	err := d.DB.Where("feed_info_id=?", feedID).
		Group("guid").
		Order("added_on DESC").
		Limit(max).
		Find(&items).Error

	if err == gorm.RecordNotFound {
		err = nil
	}
	glog.Infof("Got last %d guids for feed_id: %d.", len(items), feedID)
	knownGuids := make([]string, len(items))
	for i, v := range items {
		knownGuids[i] = v.Guid
	}
	return knownGuids, err
}

/*
*
* User related functions
*
 */

func validateUser(u *User) error {
	if u.Name == "" || u.Email == "" || u.Password == "" {
		return errors.New("name, email or pass can't be empty")
	}
	addr, err := mail.ParseAddress(u.Email)
	if err != nil {
		return fmt.Errorf("invalid email address: %s", err)
	}
	u.Email = addr.Address
	return nil
}

// AddUser creates a User record in the database.
func (d *DBHandle) AddUser(name string, email string, pass string) (*User, error) {
	u := &User{
		Name:     name,
		Email:    email,
		Enabled:  true,
		Password: pass,
	}
	err := validateUser(u)
	if err != nil {
		return u, err
	}

	bcryptPassword, err := bcrypt.GenerateFromPassword([]byte(pass), 10)
	if err != nil {
		return u, err
	}
	u.Password = string(bcryptPassword)

	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	err = d.DB.Save(u).Error
	return u, err
}

//SaveUser creates or updates the given User in the database.
func (d *DBHandle) SaveUser(u *User) error {
	err := validateUser(u)
	if err != nil {
		return err
	}
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.DB.Save(u).Error
}

// GetAllUsers returns all Users from the database.
func (d *DBHandle) GetAllUsers() ([]User, error) {
	var all []User
	err := d.DB.Find(&all).Error
	return all, err
}

// GetUser returns the user with the given name from the database.
func (d *DBHandle) GetUser(name string) (*User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	u := &User{}
	err := d.DB.Where("name = ?", name).First(u).Error

	return u, err
}

// GetUserById returns the user with the given id from the database.
func (d *DBHandle) GetUserById(id int64) (*User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	u := &User{}
	err := d.DB.First(u, id).Error

	return u, err
}

// GetUserByEmail returns the user with the given email from the database.
func (d *DBHandle) GetUserByEmail(email string) (*User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.unsafeGetUserByEmail(email)
}

func (d *DBHandle) unsafeGetUserByEmail(email string) (*User, error) {
	u := &User{}
	err := d.DB.Where("email = ?", email).Find(u).Error
	return u, err
}

// RemoveUser removes the given user from the database.
func (d *DBHandle) RemoveUser(user *User) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.unsafeRemoveUser(user)
}

func (d *DBHandle) unsafeRemoveUser(user *User) error {
	err := d.DB.Delete(user).Error
	if err == nil {
		err = d.DB.Where("user_id = ?", user.Id).Delete(UserFeed{}).Error
	}
	return err
}

// RemoveUserByEmail removes the user with the given email from the database.
func (d *DBHandle) RemoveUserByEmail(email string) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	u, err := d.unsafeGetUserByEmail(email)
	if err != nil {
		return err
	}

	return d.unsafeRemoveUser(u)
}

// AddFeedsToUser creates a UserFeed entry between the given user and feeds.
// This 'subscribes' the user to those feeds to they will get email updates
// for that feed.
func (d *DBHandle) AddFeedsToUser(u *User, feeds []*FeedInfo) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	for _, f := range feeds {
		uf := &UserFeed{
			UserId: u.Id,
			FeedId: f.Id,
		}
		//err := d.DB.Where(uf).FirstOrCreate(uf).Error
		err := d.DB.Save(uf).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RemoveFeedsFromUser does the opposite of AddFeedsToUser
func (d *DBHandle) RemoveFeedsFromUser(u *User, feeds []*FeedInfo) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	for _, f := range feeds {
		err := d.DB.Where("feed_id = ? and user_id = ?", f.Id, u.Id).Delete(UserFeed{}).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// GetUsersFeeds returns all the FeedInfos that a user is subscribed to.
func (d *DBHandle) GetUsersFeeds(u *User) ([]FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var feedInfos []FeedInfo

	err := d.DB.Table("feed_info").
		Select("feed_info.id,feed_info.name,feed_info.url").
		Joins("INNER join user_feed on user_feed.feed_id = feed_info.id").
		Where("user_feed.user_id = ?", u.Id).
		Order("feed_info.name").
		Group("feed_info.id").
		Scan(&feedInfos).Error
	if err == gorm.RecordNotFound {
		err = nil
	}
	return feedInfos, err
}

// UpdateUsersFeeds replaces a users subscribed feeds with the given
// list of feedIDs
func (d *DBHandle) UpdateUsersFeeds(u *User, feedIDs []int64) error {
	feeds, err := d.GetUsersFeeds(u)
	if err != nil {
		return err
	}

	existingFeedIDs := make(map[int64]*FeedInfo, len(feeds))
	newFeedIDs := make(map[int64]*FeedInfo, len(feedIDs))

	for i := range feeds {
		existingFeedIDs[feeds[i].Id] = &feeds[i]
	}
	for _, id := range feedIDs {
		feed, err := d.GetFeedById(id)
		if err != nil {
			return fmt.Errorf("no feed with id %d found", id)
		}
		newFeedIDs[id] = feed
	}

	toAdd := []*FeedInfo{}
	toDelete := []*FeedInfo{}

	for k, v := range existingFeedIDs {
		if _, ok := newFeedIDs[k]; !ok {
			toDelete = append(toDelete, v)
		}
	}
	for k, v := range newFeedIDs {
		if _, ok := existingFeedIDs[k]; !ok {
			toAdd = append(toAdd, v)
		}
	}

	err = d.AddFeedsToUser(u, toAdd)
	if err != nil {
		return err
	}
	err = d.RemoveFeedsFromUser(u, toDelete)
	if err != nil {
		return err
	}

	return nil
}

// GetFeedUsers returns all the users subscribed to a given feedURL
func (d *DBHandle) GetFeedUsers(feedURL string) ([]User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var all []User
	err := d.DB.Table("user").
		Select("user.id, user.name, user.email, user.enabled").
		Joins("inner join user_feed on user.id=user_feed.user_id inner join feed_info on feed_info.id=user_feed.feed_id").
		Where("feed_info.url = ?", feedURL).
		Group("user.id").Scan(&all).Error
	if err == gorm.RecordNotFound {
		err = nil
	}
	return all, err
}

//
// Exported Testing Functions
// - not sure if there is a better way to do this
//

// TestReporter is a shim interface so we don't need to include the testing
// package in the compiled binary
type TestReporter interface {
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

// LoadFixtures adds a base set of Fixtures to the given database.
func LoadFixtures(t TestReporter, d *DBHandle) ([]*FeedInfo, []*User) {
	users := []*User{
		{
			Name:     "testuser1",
			Email:    "test1@example.com",
			Password: "pass1",
			Enabled:  true,
		},
		{
			Name:     "testuser2",
			Email:    "test2@example.com",
			Password: "pass2",
			Enabled:  true,
		},
		{
			Name:     "testuser3",
			Email:    "test3@example.com",
			Password: "pass3",
			Enabled:  true,
		},
	}
	feeds := []*FeedInfo{
		{
			Name: "testfeed1",
			Url:  "http://testfeed1/feed.atom",
		},
		{
			Name: "testfeed2",
			Url:  "http://testfeed2/feed.atom",
		},
		{
			Name: "testfeed3",
			Url:  "http://testfeed3/feed.atom",
		},
	}
	for _, f := range feeds {
		err := d.SaveFeed(f)
		if err != nil {
			t.Fatalf("Error saving feed fixture to db: %s", err)
		}
	}
	for _, u := range users {
		err := d.SaveUser(u)
		if err != nil {
			t.Fatalf("Error saving user fixture to db: %s", err)
		}
		err = d.AddFeedsToUser(u, feeds)
		if err != nil {
			t.Fatalf("Error adding feeds to fixture user: %s", err)
		}
	}

	return feeds, users
}
