package db

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"sync"
	"time"

	"crypto/rand"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/gomigrate"
	"github.com/jinzhu/gorm"
	"golang.org/x/crypto/bcrypt"

	// Import sqlite3 driver
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	gorm.NowFunc = func() time.Time {
		return time.Now().UTC()
	}
}

// FeedInfo represents a feed (atom, rss or rdf) that rss2go is polling.
type FeedInfo struct {
	ID                int64     `json:"id"`
	Name              string    `sql:"not null;unique" json:"name" binding:"required"`
	URL               string    `sql:"not null;unique" json:"url" binding:"required"`
	LastPollTime      time.Time `json:"lastPollTime"`
	LastPollError     string    `json:"lastPollError"`
	LastErrorResponse string    `json:"-"`
	Users             []User    `gorm:"many2many:user_feeds;" json:"-"`
}

//TableName sets the name of the sql table to use.
func (f FeedInfo) TableName() string {
	return "feed_info"
}

// AfterFind is run after a record is returned from the db, it fixes the fact
// that SQLite driver sets everything to local timezone
func (f *FeedInfo) AfterFind() error {
	f.LastPollTime = f.LastPollTime.UTC()
	return nil
}

// FeedItem represents an individual iteam from a feed.  It only captures the
// Guid for that item and is mainly used to check if a particular item has been
// seen before.
type FeedItem struct {
	ID         int64
	FeedInfoID int64     `sql:"not null"`
	GUID       string    `sql:"not null"`
	AddedOn    time.Time `sql:"not null"`
}

//TableName sets the name of the sql table to use.
func (f FeedItem) TableName() string {
	return "feed_item"
}

// AfterFind fixes the fact that the SQLite driver sets everything to local timezone
func (f *FeedItem) AfterFind() error {
	f.AddedOn = f.AddedOn.UTC()
	return nil
}

// User represents a user/email address that can subscribe to Feeds
type User struct {
	ID       int64      `json:"id"`
	Name     string     `sql:"size:255;not null;unique" json:"name"`
	Email    string     `sql:"size:255;not null;unique" json:"email"`
	Enabled  bool       `json:"enabled"`
	Password string     `json:"-"`
	Feeds    []FeedInfo `gorm:"many2many:user_feeds;" json:"-"`
}

//TableName sets the name of the sql table to use.
func (u User) TableName() string {
	return "user"
}

// Service defines the interface that the RSS2Go database provides.
// Useful for mocking out the databse layer in tests.
type Service interface {
	GetFeedByURL(string) (*FeedInfo, error)
	GetFeedUsers(string) ([]User, error)
	SaveFeed(*FeedInfo) error
	GetMostRecentGUIDsForFeed(int64, int) ([]string, error)
	RecordGUID(int64, string) error
	GetFeedItemByGUID(int64, string) (*FeedItem, error)
}

// Handle controls access to the database and makes sure only one
// operation is in process at a time.
type Handle struct {
	db        *gorm.DB
	logger    logrus.FieldLogger
	syncMutex sync.Mutex
}

func openDB(dbType string, dbArgs string, verbose bool, logger logrus.FieldLogger) *gorm.DB {
	logger.Infof("Opening database %s:%s", dbType, dbArgs)
	// Error only returns from this if it is an unknown driver.
	d, err := gorm.Open(dbType, dbArgs)
	if err != nil {
		panic(fmt.Sprintf("Error connecting to %s database %s: %s", dbType, dbArgs, err.Error()))
	}
	d.SingularTable(true)
	d.SetLogger(logger)
	d.LogMode(verbose)
	// Actually test that we have a working connection
	err = d.DB().Ping()
	if err != nil {
		panic(fmt.Sprintf("Error connecting to database: %s", err.Error()))
	}
	return d
}

func setupDB(db *gorm.DB) error {
	err := db.Exec("PRAGMA journal_mode=WAL;").Error
	if err != nil {
		return err
	}
	err = db.Exec("PRAGMA synchronous = NORMAL;").Error
	if err != nil {
		return err
	}
	err = db.Exec("PRAGMA encoding = \"UTF-8\";").Error
	if err != nil {
		return err
	}

	return nil
}

// NewDBHandle creates a new DBHandle
//	dbPath: the path to the database to use.
//	verbose: when true database accesses are logged to stdout
func NewDBHandle(dbPath string, verbose bool, logger logrus.FieldLogger) *Handle {
	constructedPath := fmt.Sprintf("file:%s?cache=shared&mode=rwc", dbPath)
	db := openDB("sqlite3", constructedPath, verbose, logger)
	err := setupDB(db)
	if err != nil {
		panic(err.Error())
	}
	return &Handle{
		db:     db,
		logger: logger,
	}
}

// NewMemoryDBHandle creates a new in memory database.  Only used for testing.
// The name of the database is a random string so multiple tests can run in
// parallel with their own database.
func NewMemoryDBHandle(verbose bool, logger logrus.FieldLogger, loadFixtures bool) *Handle {
	constructedPath := fmt.Sprintf("file:%s?cache=shared&mode=memory", randString())
	db := openDB("sqlite3", constructedPath, verbose, logger)
	err := setupDB(db)
	if err != nil {
		panic(err.Error())
	}

	d := &Handle{
		db:     db,
		logger: logger,
	}

	err = d.Migrate(migrations)
	if err != nil {
		panic(err)
	}

	if loadFixtures {
		// load Fixtures
		err = d.Migrate(fixtures)
		if err != nil {
			panic(err)
		}
	}

	return d
}

func randString() string {
	rb := make([]byte, 32)
	_, err := rand.Read(rb)
	if err != nil {
		fmt.Println(err)
	}
	return base64.URLEncoding.EncodeToString(rb)
}

// Migrate uses the migrations at the given path to update the database.
func (d *Handle) Migrate(m []*gomigrate.Migration) error {
	migrator, err := gomigrate.NewMigratorWithMigrations(d.db.DB(), gomigrate.Sqlite3{}, m)
	if err != nil {
		return err
	}
	migrator.Logger = d.logger
	err = migrator.Migrate()
	return err
}

/*
*
* FeedInfo related functions
*
 */
func validateFeed(f *FeedInfo) error {
	if f.Name == "" || f.URL == "" {
		return errors.New("name and url can't be empty")
	}
	u, err := url.Parse(f.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %s", err)
	} else if u.Scheme == "" {
		return errors.New("URL has no Scheme")
	} else if u.Host == "" {
		return errors.New("URL has no Host")
	}

	f.URL = u.String()
	return nil
}

// AddFeed creates a new FeedInfo entry in the database and returns it.
func (d *Handle) AddFeed(name string, feedURL string) (*FeedInfo, error) {
	f := &FeedInfo{
		Name: name,
		URL:  feedURL,
	}
	err := validateFeed(f)
	if err != nil {
		return nil, err
	}

	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	return f, d.db.Save(f).Error
}

//SaveFeed creates or updates the given FeedInfo in the database.
func (d *Handle) SaveFeed(f *FeedInfo) error {
	err := validateFeed(f)
	if err != nil {
		return err
	}
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	return d.db.Save(f).Error
}

// RemoveFeed removes a FeedInfo from the database given it's URL.  Optioanlly
// it will also remove all of the GUIDs for that FeedInfo.
func (d *Handle) RemoveFeed(url string) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	f, err := d.unsafeGetFeedByURL(url)

	if err != nil {
		return err
	}
	tx := d.db.Begin()
	err = tx.Delete(f).Error
	if err == nil {
		err = tx.Where("feed_info_id = ?", f.ID).Delete(FeedItem{}).Error
		if err != nil {
			tx.Rollback()
			return err
		}
		err = tx.Model(f).Association("Users").Clear().Error
		if err != nil {
			tx.Rollback()
			return err
		}

	}
	tx.Commit()
	return err
}

// GetAllFeeds returns all feeds from the database.
func (d *Handle) GetAllFeeds() (feeds []FeedInfo, err error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	err = d.db.Find(&feeds).Error
	return
}

// GetFeedsWithErrors returns all feeds that had an error on their last
// check.
func (d *Handle) GetFeedsWithErrors() ([]FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var feeds []FeedInfo
	err := d.db.Where(
		"last_poll_error IS NOT NULL and last_poll_error <> ''").Find(&feeds).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}

	return feeds, err
}

// GetStaleFeeds returns all feeds that haven't gotten new content in more
// than 14 days.
func (d *Handle) GetStaleFeeds() ([]FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var res []FeedInfo
	err := d.db.Raw(`
	select feed_info.id, feed_info.name, feed_info.url,  r.MaxTime, feed_info.last_poll_error FROM (SELECT feed_info_id, MAX(added_on) as MaxTime FROM feed_item GROUP BY feed_info_id) r, feed_info INNER JOIN feed_item f ON f.feed_info_id = r.feed_info_id AND f.added_on = r.MaxTime AND r.MaxTime < datetime('now','-14 days') AND f.feed_info_id = feed_info.id group by f.feed_info_id;
	`).Scan(&res).Error
	return res, err
}

// GetFeedByID returns the FeedInfo for the given id.  It returns a
// gorm.ErrRecordNotFound error if it doesn't exist.
func (d *Handle) GetFeedByID(id int64) (*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var f FeedInfo
	err := d.db.First(&f, id).Error
	return &f, err
}

// GetFeedByURL returns the FeedInfo for the given URL.  It returns a
// gorm.ErrRecordNotFound error if it doesn't exist.
func (d *Handle) GetFeedByURL(url string) (*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.unsafeGetFeedByURL(url)
}

func (d *Handle) unsafeGetFeedByURL(url string) (*FeedInfo, error) {
	feed := &FeedInfo{}
	err := d.db.Where("url = ?", url).First(feed).Error
	return feed, err
}

/*
*
* FeedItem related functions
*
 */

// GetFeedItemByGUID returns a FeedItem for the given FeedInfo.Id and guid.
func (d *Handle) GetFeedItemByGUID(feedID int64, guid string) (*FeedItem, error) {
	fi := FeedItem{}
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	err := d.db.Where("feed_info_id = ? AND guid = ?", feedID, guid).First(&fi).Error
	return &fi, err
}

// RecordGUID adds a FeedItem record for the given feedID and guid.
func (d *Handle) RecordGUID(feedID int64, guid string) error {
	d.logger.Infof("Adding GUID '%s' for feed %d", guid, feedID)
	f := FeedItem{
		FeedInfoID: feedID,
		GUID:       guid,
		AddedOn:    time.Now(),
	}
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	return d.db.Save(&f).Error
}

// GetMostRecentGUIDsForFeed retrieves the most recent GUIDs for a given feed
// up to max.  GUIDs are returned ordered with the most recent first.
func (d *Handle) GetMostRecentGUIDsForFeed(feedID int64, max int) ([]string, error) {
	var items []FeedItem
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	err := d.db.Where("feed_info_id=?", feedID).
		Group("guid").
		Order("added_on DESC").
		Limit(max).
		Find(&items).Error

	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	d.logger.Infof("Got last %d guids for feed_id: %d.", len(items), feedID)
	knownGuids := make([]string, len(items))
	for i, v := range items {
		knownGuids[i] = v.GUID
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
func (d *Handle) AddUser(name string, email string, pass string) (*User, error) {
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
	err = d.db.Save(u).Error
	return u, err
}

//SaveUser creates or updates the given User in the database.
func (d *Handle) SaveUser(u *User) error {
	err := validateUser(u)
	if err != nil {
		return err
	}
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.db.Save(u).Error
}

// GetAllUsers returns all Users from the database.
func (d *Handle) GetAllUsers() ([]User, error) {
	var all []User
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	err := d.db.Find(&all).Error
	return all, err
}

// GetUser returns the user with the given name from the database.
func (d *Handle) GetUser(name string) (*User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	u := &User{}
	err := d.db.Where("name = ?", name).First(u).Error

	return u, err
}

// GetUserByID returns the user with the given id from the database.
func (d *Handle) GetUserByID(id int64) (*User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	u := &User{}
	err := d.db.First(u, id).Error

	return u, err
}

// GetUserByEmail returns the user with the given email from the database.
func (d *Handle) GetUserByEmail(email string) (*User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.unsafeGetUserByEmail(email)
}

func (d *Handle) unsafeGetUserByEmail(email string) (*User, error) {
	u := &User{}
	err := d.db.Where("email = ?", email).Find(u).Error
	return u, err
}

// RemoveUser removes the given user from the database.
func (d *Handle) RemoveUser(user *User) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.unsafeRemoveUser(user)
}

func (d *Handle) unsafeRemoveUser(user *User) error {

	tx := d.db.Begin()
	err := tx.Model(user).Association("Feeds").Clear().Error
	if err != nil {
		tx.Rollback()
		return err
	}
	err = tx.Delete(user).Error
	if err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil
}

// RemoveUserByEmail removes the user with the given email from the database.
func (d *Handle) RemoveUserByEmail(email string) error {
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
func (d *Handle) AddFeedsToUser(u *User, feeds []*FeedInfo) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	for _, f := range feeds {
		err := d.db.Model(u).Association("Feeds").Append(*f).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RemoveFeedsFromUser does the opposite of AddFeedsToUser
func (d *Handle) RemoveFeedsFromUser(u *User, feeds []*FeedInfo) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	for _, f := range feeds {
		err := d.db.Model(u).Association("Feeds").Delete(*f).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// GetUsersFeeds returns all the FeedInfos that a user is subscribed to.
func (d *Handle) GetUsersFeeds(u *User) ([]FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var feedInfos []FeedInfo

	err := d.db.Model(u).Association("Feeds").Find(&feedInfos).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return feedInfos, err
}

// UpdateUsersFeeds replaces a users subscribed feeds with the given
// list of feedIDs
func (d *Handle) UpdateUsersFeeds(u *User, feedIDs []int64) error {
	feeds, err := d.GetUsersFeeds(u)
	if err != nil {
		return err
	}

	existingFeedIDs := make(map[int64]*FeedInfo, len(feeds))
	newFeedIDs := make(map[int64]*FeedInfo, len(feedIDs))

	for i := range feeds {
		existingFeedIDs[feeds[i].ID] = &feeds[i]
	}
	for _, id := range feedIDs {
		feed, err := d.GetFeedByID(id)
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
func (d *Handle) GetFeedUsers(feedURL string) ([]User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var all []User
	feed, err := d.unsafeGetFeedByURL(feedURL)
	if err == gorm.ErrRecordNotFound {
		return all, nil
	}
	if err != nil {
		return all, err
	}
	err = d.db.Model(feed).Association("Users").Find(&all).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return all, err
}
