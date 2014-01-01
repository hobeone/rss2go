package db

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/astaxie/beedb"
	"github.com/golang/glog"
	_ "github.com/mattn/go-sqlite3"
	"net/mail"
	"net/url"
	"sync"
	"time"
)

type FeedInfo struct {
	Id            int       `beedb:"PK" json:"id"`
	Name          string    `json:"name"`
	Url           string    `json:"url"`
	LastPollTime  time.Time `json:"lastPollTime"`
	LastPollError string    `json:"lastPollError"`
}

type FeedItem struct {
	Id         int `beedb:"PK"`
	FeedInfoId int
	Guid       string
	AddedOn    time.Time
}

type User struct {
	Id      int    `beedb:"PK"  json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Enabled bool   `json:"enabled"`
}

type UserFeed struct {
	Id     int `beedb:"PK"`
	UserId int
	FeedId int
}

const FEED_INFO_TABLE = `
	create table feed_info (
		id integer not null primary key,
		name text not null UNIQUE,
		url text not null UNIQUE,
		last_poll_time DATE NULL,
		last_poll_error text NULL
	);
`
const FEED_ITEM_TABLE = `
	create table feed_item (
		id integer not null primary key,
		feed_info_id integer not null,
		guid text not null,
		added_on DATE not NULL
	);
`
const USER_TABLE = `
  create table user (
		id integer not null primary key,
		name text not null UNIQUE,
		email text not null UNIQUE,
		enabled bool not null
	);
`
const USER_FEED_TABLE = `
  create table user_feed (
		id integer not null primary key,
		user_id integer not null,
		feed_id integer not null
	);
`

func createAndOpenDb(db_path string, verbose bool, memory bool) (*sql.DB, beedb.Model) {
	beedb.OnDebug = verbose
	glog.Infof("Opening database %s", db_path)
	mode := "rwc"
	if memory {
		mode = "memory"
	}
	db, err := sql.Open("sqlite3",
		fmt.Sprintf("file:%s?mode=%s", db_path, mode))

	if err != nil {
		glog.Fatal(err)
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table';")
	if err != nil {
		glog.Fatalf("Couldn't get list of tables from database: %s", err.Error())
	}
	tables := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			glog.Fatal(err)
		}
		tables[name] = true
	}

	if _, ok := tables["feed_info"]; !ok {
		createTable(db, FEED_INFO_TABLE)
	}
	if _, ok := tables["feed_item"]; !ok {
		createTable(db, FEED_ITEM_TABLE)
	}
	if _, ok := tables["user"]; !ok {
		createTable(db, USER_TABLE)
	}
	if _, ok := tables["user_feed"]; !ok {
		createTable(db, USER_FEED_TABLE)
	}

	return db, beedb.New(db)
}

func createTable(dbh *sql.DB, table_def string) {
	_, err := dbh.Exec(table_def)
	if err != nil {
		panic(fmt.Sprintf("Error creating table: %s\nSQL: %s", err.Error(),
			table_def))
	}
}

type DbDispatcher struct {
	Orm          beedb.Model
	dbh          *sql.DB
	writeUpdates bool
	syncMutex    sync.Mutex
}

func NewDbDispatcher(db_path string, verbose bool, write_updates bool) *DbDispatcher {
	d := &DbDispatcher{
		writeUpdates: write_updates,
	}
	d.dbh, d.Orm = createAndOpenDb(db_path, verbose, false)
	return d
}

func NewMemoryDbDispatcher(verbose bool, write_updates bool) *DbDispatcher {
	d := &DbDispatcher{
		writeUpdates: write_updates,
	}
	d.dbh, d.Orm = createAndOpenDb("in_memory_test", verbose, true)
	return d
}

func (self *DbDispatcher) AddFeed(name string, feed_url string) (*FeedInfo, error) {
	if name == "" || feed_url == "" {
		return nil, errors.New("Name and url can't be empty.")
	}
	u, err := url.Parse(feed_url)
	if err != nil {
		return nil, fmt.Errorf("Invalid URL: %s", err)
	} else if u.Scheme == "" {
		return nil, errors.New("URL has no Scheme.")
	} else if u.Host == "" {
		return nil, errors.New("URL has no Host.")
	}

	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	f := &FeedInfo{
		Name: name,
		Url:  u.String(),
	}
	err = self.Orm.Save(f)
	return f, err
}

func (self *DbDispatcher) RemoveFeed(url string, purge_guids bool) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	f, err := self.unsafeGetFeedByUrl(url)
	if err != nil {
		return err
	}
	_, err = self.Orm.Delete(f)
	if err == nil {
		_, err := self.Orm.SetTable("feed_item").Where("feed_info_id = ?", f.Id).DeleteRow()
		if err != nil {
			return err
		}
		_, err = self.Orm.SetTable("user_feed").Where("feed_id = ?", f.Id).DeleteRow()
		if err != nil {
			return err
		}

	}
	return err
}

func (self *DbDispatcher) GetAllFeeds() (feeds []FeedInfo, err error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	err = self.Orm.FindAll(&feeds)
	return
}

func (self *DbDispatcher) GetFeedsWithErrors() (feeds []FeedInfo, err error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	err = self.Orm.Where(
		"last_poll_error IS NOT NULL and last_poll_error <> ''").FindAll(&feeds)

	return
}

func (self *DbDispatcher) GetStaleFeeds() (feeds []FeedInfo, err error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	rows, err := self.dbh.Query(`
	select feed_info.id, feed_info.name, feed_info.url, feed_info.last_poll_time, feed_info.last_poll_error, r.MaxTime FROM (SELECT feed_info_id, MAX(added_on) as MaxTime FROM feed_item GROUP BY feed_info_id) r, feed_info INNER JOIN feed_item f ON f.feed_info_id = r.feed_info_id AND f.added_on = r.MaxTime AND r.MaxTime < datetime('now','-14 days') AND f.feed_info_id = feed_info.id group by f.feed_info_id;
	`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name, url, last_poll_error, last_poll_time, max_time string
		err = rows.Scan(&id, &name, &url, &last_poll_time, &last_poll_error, &max_time)
		if err != nil {
			return feeds, err
		}
		ftime, err := time.Parse("2006-01-02 15:04:05", max_time)
		if err != nil {
			return nil, err
		}
		// Sorta hacky: set LastPollTime to time of last item seen, rather than
		// last time feed was polled.
		feeds = append(feeds, FeedInfo{
			Id:            id,
			Name:          name,
			Url:           url,
			LastPollTime:  ftime,
			LastPollError: last_poll_error,
		})
	}

	return
}

func (self *DbDispatcher) GetFeedById(id int) (*FeedInfo, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	feed_info := &FeedInfo{}
	err := self.Orm.Where(id).Find(feed_info)
	return feed_info, err
}

func (self *DbDispatcher) GetFeedByUrl(url string) (*FeedInfo, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.unsafeGetFeedByUrl(url)
}

func (self *DbDispatcher) unsafeGetFeedByUrl(url string) (*FeedInfo, error) {
	feed := FeedInfo{}
	err := self.Orm.Where("url = ?", url).Find(&feed)
	return &feed, err
}

func (self *DbDispatcher) GetFeedItemByGuid(f_id int, guid string) (*FeedItem, error) {
	//TODO: see if beedb will handle this correctly and protect against injection
	//attacks.
	fi := FeedItem{}
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	err := self.Orm.Where("feed_info_id = ? AND guid = ?", f_id, guid).Find(&fi)
	return &fi, err
}

func (self *DbDispatcher) RecordGuid(feed_id int, guid string) (err error) {
	if self.writeUpdates {
		glog.Infof("Adding GUID '%s' for feed %d", guid, feed_id)
		f := FeedItem{
			FeedInfoId: feed_id,
			Guid:       guid,
			AddedOn:    time.Now(),
		}
		self.syncMutex.Lock()
		defer self.syncMutex.Unlock()

		return self.Orm.Save(&f)
	}
	return nil
}

// Retrieves the most recent GUIDs for a given feed up to max.  GUIDs are
// returned ordered with the most recent first.
func (self *DbDispatcher) GetMostRecentGuidsForFeed(f_id int, max int) ([]string, error) {
	var items []FeedItem
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	err := self.Orm.Where("feed_info_id=?", f_id).GroupBy("guid").OrderBy("added_on DESC").Limit(max).FindAll(&items)
	if err != nil {
		return []string{}, err
	}
	glog.Infof("Got last %d guids for feed_id: %d.", len(items), f_id)
	known_guids := make([]string, len(items))
	for i, v := range items {
		known_guids[i] = v.Guid
	}
	return known_guids, nil
}

func (self *DbDispatcher) UpdateFeed(f *FeedInfo) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	return self.Orm.Save(f)
}

func (self *DbDispatcher) AddUser(name string, email string) (*User, error) {
	if name == "" || email == "" {
		return nil, errors.New("Name and Email must be set.")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return nil, fmt.Errorf("Invalid email address: %s", err)
	}
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	u := &User{
		Name:    name,
		Email:   addr.Address,
		Enabled: true,
	}
	err = self.Orm.Save(u)
	return u, err
}

func (self *DbDispatcher) SaveUser(u *User) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.Orm.Save(u)
}

func (self *DbDispatcher) GetUser(name string) (*User, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	u := &User{}
	err := self.Orm.Where("name = ?", name).Find(u)

	return u, err
}
func (self *DbDispatcher) GetUserById(id int) (*User, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	u := &User{}
	err := self.Orm.Where(id).Find(u)

	return u, err
}

func (self *DbDispatcher) GetUserByEmail(name string) (*User, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.unsafeGetUserByEmail(name)
}
func (self *DbDispatcher) unsafeGetUserByEmail(name string) (*User, error) {
	u := &User{}
	err := self.Orm.Where("email = ?", name).Find(u)
	return u, err
}

func (self *DbDispatcher) RemoveUser(user *User) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.unsafeRemoveUser(user)
}
func (self *DbDispatcher) unsafeRemoveUser(user *User) error {
	_, err := self.Orm.Delete(user)
	if err == nil {
		_, err = self.Orm.SetTable("user_feed").Where("user_id = ?", user.Id).DeleteRow()
	}
	return err
}

func (self *DbDispatcher) RemoveUserByEmail(email string) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	u, err := self.unsafeGetUserByEmail(email)
	if err != nil {
		return err
	}

	return self.unsafeRemoveUser(u)
}

func (self *DbDispatcher) AddFeedsToUser(u *User, feeds []*FeedInfo) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	for _, f := range feeds {
		uf := &UserFeed{
			UserId: u.Id,
			FeedId: f.Id,
		}
		err := self.Orm.Save(uf)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *DbDispatcher) RemoveFeedsFromUser(u *User, feeds []*FeedInfo) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	for _, f := range feeds {
		_, err := self.Orm.SetTable("user_feed").
			Where("feed_id = ? and user_id = ?", f.Id, u.Id).
			DeleteRow()
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *DbDispatcher) GetAllUsers() ([]User, error) {
	var all []User
	err := self.Orm.FindAll(&all)
	return all, err
}

func (self *DbDispatcher) GetUsersFeeds(u *User) ([]FeedInfo, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	var all []FeedInfo

	err := self.Orm.SetTable("feed_info").
		Join("INNER", "user_feed", "feed_info.id=user_feed.feed_id").
		Select("feed_info.id,feed_info.name,feed_info.url").
		Where("user_feed.user_id = ?", u.Id).
		OrderBy("feed_info.name").
		GroupBy("feed_info.id").
		FindAll(&all)

	return all, err
}

func (self *DbDispatcher) UpdateUsersFeeds(u *User, feed_ids []int) error {
	feeds, err := self.GetUsersFeeds(u)
	if err != nil {
		return err
	}

	existing_feed_ids := make(map[int]*FeedInfo, len(feeds))
	new_feed_ids := make(map[int]*FeedInfo, len(feed_ids))

	for i := range feeds {
		existing_feed_ids[feeds[i].Id] = &feeds[i]
	}
	for _, id := range feed_ids {
		feed, err := self.GetFeedById(id)
		if err != nil {
			return fmt.Errorf("No feed with id %d found.", id)
		}
		new_feed_ids[id] = feed
	}

	to_add := []*FeedInfo{}
	to_delete := []*FeedInfo{}

	for k, v := range existing_feed_ids {
		if _, ok := new_feed_ids[k]; !ok {
			to_delete = append(to_delete, v)
		}
	}
	for k, v := range new_feed_ids {
		if _, ok := existing_feed_ids[k]; !ok {
			to_add = append(to_add, v)
		}
	}

	err = self.AddFeedsToUser(u, to_add)
	if err != nil {
		return err
	}
	err = self.RemoveFeedsFromUser(u, to_delete)
	if err != nil {
		return err
	}

	return nil
}

func (self *DbDispatcher) GetFeedUsers(f_url string) ([]User, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	var all []User
	err := self.Orm.SetTable("user").
		Join("INNER", "user_feed", "user.id=user_feed.user_id").
		Join("INNER", "feed_info", "feed_info.id=user_feed.feed_id").
		Where("feed_info.url = ?", f_url).
		GroupBy("user.id").
		Select("user.id,user.name,user.email,user.enabled").FindAll(&all)

	return all, err
}
