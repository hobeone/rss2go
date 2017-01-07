package db

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"sync"
	"time"
	"unicode"

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
	ID                int64     `json:"id" jsonapi:"primary,feeds"`
	Name              string    `sql:"not null;unique" json:"name" binding:"required" jsonapi:"attr,name"`
	URL               string    `sql:"not null;unique" json:"url" binding:"required" jsonapi:"attr,url"`
	SiteURL           string    `jsonapi:"attr,site-url"`
	LastPollTime      time.Time `json:"lastPollTime" jsonapi:"attr,last-poll-time,iso8601"`
	LastPollError     string    `json:"lastPollError" jsonapi:"attr,last-poll-error"`
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
	ID       int64      `json:"id" jsonapi:"primary,feeds"`
	Name     string     `sql:"size:255;not null;unique" json:"name" jsonapi:"attr,name"`
	Email    string     `sql:"size:255;not null;unique" json:"email" jsonapi:"attr,email"`
	Enabled  bool       `json:"enabled"`
	Password string     `json:"-"`
	Feeds    []FeedInfo `gorm:"many2many:user_feeds;" json:"-" jsonapi:"relation,feeds"`
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

type logrusAdapter struct {
	logger logrus.FieldLogger
}

var (
	sqlRegexp = regexp.MustCompile(`(\$\d+)|\?`)
)

func (l logrusAdapter) Print(values ...interface{}) {
	if len(values) > 1 {
		level := values[0]
		source := fmt.Sprintf("\033[35m(%v)\033[0m", values[1])
		messages := []interface{}{source}

		if level == "sql" {
			// duration
			messages = append(messages, fmt.Sprintf(" \033[36;1m[%.2fms]\033[0m ", float64(values[2].(time.Duration).Nanoseconds()/1e4)/100.0))
			// sql
			var sql string
			var formattedValues []string

			for _, value := range values[4].([]interface{}) {
				indirectValue := reflect.Indirect(reflect.ValueOf(value))
				if indirectValue.IsValid() {
					value = indirectValue.Interface()
					if t, ok := value.(time.Time); ok {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", t.Format(time.RFC3339)))
					} else if b, ok := value.([]byte); ok {
						if str := string(b); isPrintable(str) {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", str))
						} else {
							formattedValues = append(formattedValues, "'<binary>'")
						}
					} else if r, ok := value.(driver.Valuer); ok {
						if value, err := r.Value(); err == nil && value != nil {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
						} else {
							formattedValues = append(formattedValues, "NULL")
						}
					} else {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
					}
				} else {
					formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
				}
			}

			var formattedValuesLength = len(formattedValues)
			for index, value := range sqlRegexp.Split(values[3].(string), -1) {
				sql += value
				if index < formattedValuesLength {
					sql += formattedValues[index]
				}
			}

			messages = append(messages, sql)
		} else {
			messages = append(messages, "\033[31;1m")
			messages = append(messages, values[2:]...)
			messages = append(messages, "\033[0m")
		}
		l.logger.Debugln(messages...)
	}
}
func isPrintable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func openDB(dbType string, dbArgs string, verbose bool, logger logrus.FieldLogger) *gorm.DB {
	logger.Infof("db: opening database %s:%s", dbType, dbArgs)
	// Error only returns from this if it is an unknown driver.
	d, err := gorm.Open(dbType, dbArgs)
	if err != nil {
		panic(fmt.Sprintf("Error connecting to %s database %s: %s", dbType, dbArgs, err.Error()))
	}
	d.SingularTable(true)
	d.SetLogger(logrusAdapter{logger})
	d.LogMode(verbose)
	// Actually test that we have a working connection
	err = d.DB().Ping()
	if err != nil {
		panic(fmt.Sprintf("db: error connecting to database: %s", err.Error()))
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
	db := openDB("sqlite3", ":memory:", verbose, logger)

	err := setupDB(db)
	if err != nil {
		panic(err.Error())
	}

	d := &Handle{
		db:     db,
		logger: logger,
	}

	err = d.Migrate(SchemaMigrations())
	if err != nil {
		panic(err)
	}

	if loadFixtures {
		// load Fixtures
		err = d.Migrate(TestFixtures())
		if err != nil {
			panic(err)
		}
	}

	return d
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
func (d *Handle) GetAllFeeds() (feeds []*FeedInfo, err error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	err = d.db.Find(&feeds).Error
	return
}

// GetFeedsByName returns all feeds that contain the given string in their
// name.
func (d *Handle) GetFeedsByName(name string) ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var feeds []*FeedInfo

	err := d.db.Raw(`SELECT feed_info.* FROM feed_info
	INNER JOIN user_feeds ON feed_info.id = user_feeds.feed_info_id
	INNER JOIN user ON user_feeds.user_id = user.id
	WHERE feed_info.name LIKE ?
	ORDER BY feed_info.name;`, fmt.Sprintf("%%%s%%", name)).Scan(&feeds).Error

	return feeds, err
}

// GetUsersFeedsByName returns all feeds that contain the given string in their
// name.
func (d *Handle) GetUsersFeedsByName(user *User, name string) ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var feeds []*FeedInfo

	err := d.db.Raw(`SELECT feed_info.* FROM feed_info
	INNER JOIN user_feeds ON feed_info.id = user_feeds.feed_info_id
	INNER JOIN user ON user_feeds.user_id = user.id
	WHERE user.id = ?
	AND feed_info.name LIKE ?
	ORDER BY feed_info.name;`, user.ID, fmt.Sprintf("%%%s%%", name)).Scan(&feeds).Error

	return feeds, err
}

// GetFeedsWithErrors returns all feeds that had an error on their last
// check.
func (d *Handle) GetFeedsWithErrors() ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var feeds []*FeedInfo
	err := d.db.Where(
		"last_poll_error IS NOT NULL and last_poll_error <> ''").Find(&feeds).Error
	if err == gorm.ErrRecordNotFound {
		err = nil
	}

	return feeds, err
}

// GetStaleFeeds returns all feeds that haven't gotten new content in more
// than 14 days.
func (d *Handle) GetStaleFeeds() ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var res []*FeedInfo
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
	d.logger.Infof("Adding or Updating GUID '%s' for feed %d", guid, feedID)
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	return d.db.Exec("insert or replace INTO feed_item (id, feed_info_id, guid, added_on) VALUES ((select id from feed_item WHERE guid = ?), ?, ?, ?)", guid, feedID, guid, time.Now()).Error
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
		err := d.db.Exec(`INSERT OR REPLACE INTO user_feeds (id, user_id, feed_info_id)
		VALUES((select id from user_feeds WHERE user_id = ? AND feed_info_id = ?), ?, ?);`,
			u.ID, f.ID, u.ID, f.ID).Error
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

	feedIDs := make([]int64, len(feeds))
	for i, f := range feeds {
		feedIDs[i] = f.ID
	}

	err := d.db.Exec(`DELETE FROM user_feeds WHERE user_id = ? AND feed_info_id IN (?);`, u.ID, feedIDs).Error
	if err != nil {
		return err
	}
	return nil
}

// GetUsersFeeds returns all the FeedInfos that a user is subscribed to.
func (d *Handle) GetUsersFeeds(u *User) ([]*FeedInfo, error) {
	feeds, err := d.GetUsersFeedsByName(u, "")
	if err == gorm.ErrRecordNotFound {
		err = nil
	}
	return feeds, err
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
		existingFeedIDs[feeds[i].ID] = feeds[i]
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
	err := d.db.Raw(`SELECT user.* FROM user
	INNER JOIN user_feeds ON user.id = user_feeds.user_id
	INNER JOIN feed_info ON user_feeds.feed_info_id = feed_info.id
	WHERE feed_info.url = ?;`, feedURL).Scan(&all).Error
	if err == gorm.ErrRecordNotFound {
		return all, nil
	}
	if err != nil {
		return all, err
	}

	return all, err
}
