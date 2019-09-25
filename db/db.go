package db

import (
	"errors"
	"fmt"
	"math/rand"
	"net/mail"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/hobeone/gomigrate"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	// Import sqlite3 driver
	_ "github.com/mattn/go-sqlite3"
)

// FeedInfo represents a feed (atom, rss or rdf) that rss2go is polling.
type FeedInfo struct {
	ID                int64     `jsonapi:"primary,feeds"`
	Name              string    `jsonapi:"attr,name"`
	URL               string    `jsonapi:"attr,url"`
	SiteURL           string    `db:"site_url" jsonapi:"attr,site-url"`
	LastPollTime      time.Time `db:"last_poll_time" jsonapi:"attr,last-poll-time,iso8601"`
	LastPollError     string    `db:"last_poll_error" jsonapi:"attr,last-poll-error"`
	LastErrorResponse string    `db:"last_error_response"`
	Users             []User    ``
}

// FeedItem represents an individual iteam from a feed.  It only captures the
// Guid for that item and is mainly used to check if a particular item has been
// seen before.
type FeedItem struct {
	ID         int64
	FeedInfoID int64 `db:"feed_info_id"`
	GUID       string
	AddedOn    time.Time `db:"added_on"`
}

// User represents a user/email address that can subscribe to Feeds
type User struct {
	ID       int64      `jsonapi:"primary,feeds"`
	Name     string     `jsonapi:"attr,name"`
	Email    string     `jsonapi:"attr,email"`
	Enabled  bool       ``
	Password string     ``
	Feeds    []FeedInfo `jsonapi:"relation,feeds"`
}

// SetPassword takes a plain text password, crypts it and sets the Password
// field to that.
func (u *User) SetPassword(pass string) error {
	bcryptPassword, err := bcrypt.GenerateFromPassword([]byte(pass), 10)
	if err != nil {
		return err
	}
	u.Password = string(bcryptPassword)
	return nil
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
	db        *sqlx.DB
	logger    logrus.FieldLogger
	syncMutex sync.Mutex
	queryer   *queryLogger
}

func openDB(dbType string, dbArgs string, logger logrus.FieldLogger) *sqlx.DB {
	logger.Infof("db: opening database %s:%s", dbType, dbArgs)
	// Error only returns from this if it is an unknown driver.
	d, err := sqlx.Connect(dbType, dbArgs)
	if err != nil {
		panic(fmt.Sprintf("Error connecting to %s database %s: %v", dbType, dbArgs, err))
	}
	d.SetMaxOpenConns(1)
	// Actually test that we have a working connection
	err = d.Ping()
	if err != nil {
		panic(fmt.Sprintf("db: error connecting to database: %v", err))
	}
	return d
}

func setupDB(db *sqlx.DB) error {
	/*	_, err := db.Exec("PRAGMA journal_mode=WAL;")
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA synchronous = NORMAL;")
		if err != nil {
			return err
		}
	*/
	_, err := db.Exec(`PRAGMA encoding = "UTF-8";`)
	if err != nil {
		return err
	}

	return nil
}

func newQueryLogger(db *sqlx.DB, logger logrus.FieldLogger) *queryLogger {
	return &queryLogger{
		queryer: db,
		logger:  logrusAdapter{logger},
	}
}

// NewDBHandle creates a new DBHandle
//	dbPath: the path to the database to use.
//	verbose: when true database accesses are logged to stdout
func NewDBHandle(dbPath string, logger logrus.FieldLogger) *Handle {
	constructedPath := fmt.Sprintf("file:%s?cache=shared&mode=rwc&_busy_timeout=5000", dbPath)
	db := openDB("sqlite3", constructedPath, logger)
	err := setupDB(db)
	if err != nil {
		panic(err.Error())
	}
	return &Handle{
		db:      db,
		logger:  logger,
		queryer: newQueryLogger(db, logger),
	}
}

// NewMemoryDBHandle creates a new in memory database.  Only used for testing.
// The name of the database is a random string so multiple tests can run in
// parallel with their own database.
func NewMemoryDBHandle(logger logrus.FieldLogger, loadFixtures bool) *Handle {
	db := openDB("sqlite3", ":memory:", logger)

	err := setupDB(db)
	if err != nil {
		panic(err.Error())
	}

	d := &Handle{
		db:      db,
		logger:  logger,
		queryer: newQueryLogger(db, logger),
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
	migrator, err := gomigrate.NewMigratorWithMigrations(d.db.DB, gomigrate.Sqlite3{}, m)
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
	// Ensures that time in DB is always stored as UTC
	f.LastPollTime = f.LastPollTime.UTC()
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

	res, err := d.queryer.Exec(`INSERT INTO feed_info (name, url, site_url, last_poll_time, last_poll_error, last_error_response) VALUES(?,?,?,?,?,?);`,
		f.Name, f.URL, "", time.Time{}, "", "")
	if err != nil {
		return nil, err
	}
	f.ID, err = res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return f, nil
}

//SaveFeed updates the given FeedInfo in the database.
func (d *Handle) SaveFeed(f *FeedInfo) error {
	err := validateFeed(f)
	if err != nil {
		return err
	}
	if f.ID == 0 {
		return fmt.Errorf("can't update feed with no id")
	}
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	_, err = d.queryer.Exec(`UPDATE feed_info SET name = ?, url = ?, site_url = ?, last_poll_time = ?, last_poll_error = ?, last_error_response = ? WHERE id = ?`,
		f.Name, f.URL, f.SiteURL, f.LastPollTime, f.LastPollError, f.LastErrorResponse, f.ID,
	)
	return err
}

// RemoveFeed removes a FeedInfo from the database given it's URL
func (d *Handle) RemoveFeed(url string) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	f, err := d.unsafeGetFeedByURL(url)

	if err != nil {
		return err
	}
	tx, err := d.db.Beginx()
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM feed_info WHERE id = ?", f.ID)
	if err == nil {
		_, err = tx.Exec("DELETE FROM feed_item WHERE feed_info_id = ?", f.ID)
		if err != nil {
			tx.Rollback()
			return err
		}
		_, err = tx.Exec("DELETE FROM user_feeds WHERE feed_info_id = ?", f.ID)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}

// GetAllFeeds returns all feeds from the database.
func (d *Handle) GetAllFeeds() ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	feeds := []*FeedInfo{}
	err := sqlx.Select(d.queryer, &feeds, "SELECT * FROM feed_info")
	return feeds, err
}

// GetAllFeedsWithUsers returns all feeds from the database that have
// subscribers
func (d *Handle) GetAllFeedsWithUsers() ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var feeds []*FeedInfo
	err := sqlx.Select(d.queryer, &feeds, `SELECT feed_info.* FROM feed_info
	LEFT JOIN user_feeds ON feed_info.id = user_feeds.feed_info_id
	GROUP BY user_feeds.feed_info_id
	HAVING COUNT(user_feeds.feed_info_id) > 0`)
	return feeds, err
}

// GetFeedsByName returns all feeds that contain the given string in their
// name.
func (d *Handle) GetFeedsByName(name string) ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var feeds []*FeedInfo

	err := sqlx.Select(d.queryer, &feeds, `SELECT feed_info.* FROM feed_info
	INNER JOIN user_feeds ON feed_info.id = user_feeds.feed_info_id
	INNER JOIN user ON user_feeds.user_id = user.id
	WHERE feed_info.name LIKE ?
	ORDER BY feed_info.name`, fmt.Sprintf("%%%s%%", name))

	return feeds, err
}

// GetUsersFeedsByName returns all feeds that contain the given string in their
// name.
func (d *Handle) GetUsersFeedsByName(user *User, name string) ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var feeds []*FeedInfo

	err := sqlx.Select(d.queryer, &feeds, `SELECT feed_info.* FROM feed_info
	INNER JOIN user_feeds ON feed_info.id = user_feeds.feed_info_id
	INNER JOIN user ON user_feeds.user_id = user.id
	WHERE user.id = ?
	AND feed_info.name LIKE ?
	ORDER BY feed_info.name`, user.ID, fmt.Sprintf("%%%s%%", name))

	return feeds, err
}

// GetFeedsWithErrors returns all feeds that had an error on their last
// check.
func (d *Handle) GetFeedsWithErrors() ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var feeds []*FeedInfo
	err := sqlx.Select(d.queryer, &feeds, "SELECT * from feed_info WHERE last_poll_error IS NOT NULL and last_poll_error <> ''")
	return feeds, err
}

// GetStaleFeeds returns all feeds that haven't gotten new content in more
// than 14 days.
func (d *Handle) GetStaleFeeds() ([]*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	var res []*FeedInfo
	err := sqlx.Select(d.queryer, &res, `SELECT feed_info.id, feed_info.name, feed_info.url, feed_info.last_poll_error, f.added_on as last_poll_time FROM (SELECT feed_info_id, MAX(added_on) as MaxTime FROM feed_item GROUP BY feed_info_id) r, feed_info INNER JOIN feed_item f ON f.feed_info_id = r.feed_info_id AND f.added_on = r.MaxTime AND r.MaxTime < datetime('now','-14 days') AND f.feed_info_id = feed_info.id group by f.feed_info_id ORDER BY MaxTime desc;`)
	return res, err
}

// GetFeedByID returns the FeedInfo for the given id.
func (d *Handle) GetFeedByID(id int64) (*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	var f FeedInfo
	//	err := sqlx.Get(d.queryer, &f, "SELECT * from feed_info WHERE id = ? LIMIT 1", id)
	err := sqlx.Get(d.queryer, &f, "SELECT * from feed_info WHERE id = ? LIMIT 1", id)
	return &f, err
}

// GetFeedByURL returns the FeedInfo for the given URL.
func (d *Handle) GetFeedByURL(url string) (*FeedInfo, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.unsafeGetFeedByURL(url)
}

func (d *Handle) unsafeGetFeedByURL(url string) (*FeedInfo, error) {
	feed := &FeedInfo{}
	err := sqlx.Get(d.queryer, feed, `SELECT * FROM feed_info WHERE url = ? LIMIT 1`, url)
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
	err := sqlx.Get(d.queryer, &fi, "SELECT * FROM feed_item WHERE feed_info_id = ? AND guid = ?", feedID, guid)
	return &fi, err
}

// RecordGUID adds a FeedItem record for the given feedID and guid.
func (d *Handle) RecordGUID(feedID int64, guid string) error {
	d.logger.Infof("Adding or Updating GUID '%s' for feed %d", guid, feedID)
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	_, err := d.queryer.Exec(`INSERT OR REPLACE INTO feed_item (id, feed_info_id, guid, added_on)
	VALUES ((SELECT id FROM feed_item WHERE feed_info_id = ? AND guid = ?), ?, ?, ?)`, guid, feedID, feedID, guid, time.Now())
	return err
}

// GetMostRecentGUIDsForFeed retrieves the most recent GUIDs for a given feed
// up to max.  GUIDs are returned ordered with the most recent first.
func (d *Handle) GetMostRecentGUIDsForFeed(feedID int64, max int) ([]string, error) {
	var items []FeedItem
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	err := sqlx.Select(d.queryer, &items, "SELECT guid FROM feed_item WHERE feed_info_id = ? ORDER BY added_on DESC LIMIT ?", feedID, max)
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
	if pass == "" {
		pass = strconv.FormatInt(rand.Int63(), 16) // Just want to set it to something relatively random
	}
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

	err = u.SetPassword(pass)
	if err != nil {
		return u, err
	}

	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	res, err := d.queryer.Exec("INSERT INTO user (name, email, password, enabled) VALUES(?,?,?,?)", u.Name, u.Email, u.Password, u.Enabled)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	u.ID = id
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
	_, err = d.queryer.Exec("UPDATE user SET name = ?, email = ?, password = ?, enabled = ? WHERE id = ?", u.Name, u.Email, u.Password, u.Enabled, u.ID)
	return err
}

// GetAllUsers returns all Users from the database.
func (d *Handle) GetAllUsers() ([]User, error) {
	var all []User
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()

	err := sqlx.Select(d.queryer, &all, "SELECT * FROM user")
	return all, err
}

// GetUser returns the user with the given name from the database.
func (d *Handle) GetUser(name string) (*User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	u := &User{}
	err := sqlx.Get(d.queryer, u, "SELECT * FROM user WHERE name = ?", name)
	return u, err
}

// GetUserByID returns the user with the given id from the database.
func (d *Handle) GetUserByID(id int64) (*User, error) {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	u := &User{}
	err := sqlx.Get(d.queryer, u, "SELECT * FROM user WHERE id = ?", id)
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
	err := sqlx.Get(d.queryer, u, "SELECT * FROM user WHERE email = ?", email)
	return u, err
}

// RemoveUser removes the given user from the database.
func (d *Handle) RemoveUser(user *User) error {
	d.syncMutex.Lock()
	defer d.syncMutex.Unlock()
	return d.unsafeRemoveUser(user)
}

func (d *Handle) unsafeRemoveUser(user *User) error {
	tx, err := d.db.Beginx()
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM user_feeds WHERE user_id = ?", user.ID)
	if err != nil {
		tx.Rollback()
		return err
	}
	_, err = tx.Exec("DELETE FROM user WHERE id = ?", user.ID)
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
		_, err := d.queryer.Exec(`INSERT OR REPLACE INTO user_feeds (id, user_id, feed_info_id)
		VALUES((select id from user_feeds WHERE user_id = ? AND feed_info_id = ?), ?, ?)`,
			u.ID, f.ID, u.ID, f.ID)
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

	q := `DELETE FROM user_feeds WHERE user_id = ? AND feed_info_id IN (?)`
	q, args, err := sqlx.In(q, u.ID, feedIDs)
	if err != nil {
		return err
	}
	_, err = d.queryer.Exec(q, args...)
	if err != nil {
		return err
	}
	return nil
}

// GetUsersFeeds returns all the FeedInfos that a user is subscribed to.
func (d *Handle) GetUsersFeeds(u *User) ([]*FeedInfo, error) {
	feeds, err := d.GetUsersFeedsByName(u, "")
	if len(feeds) == 0 {
		err = nil
	}
	return feeds, err
}

// UpdateUsersFeeds replaces a users subscribed feeds with the given
// list of feedIDs
func (d *Handle) UpdateUsersFeeds(u *User, feedIDs []int64) error {
	if len(feedIDs) < 1 {
		return nil
	}
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
	err := sqlx.Select(d.queryer, &all, `SELECT user.* FROM user
	INNER JOIN user_feeds ON user.id = user_feeds.user_id
	INNER JOIN feed_info ON user_feeds.feed_info_id = feed_info.id
	WHERE feed_info.url = ?`, feedURL)
	return all, err
}
