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
	_ "github.com/mattn/go-sqlite3"
)

type FeedInfo struct {
	Id            int       `json:"id"`
	Name          string    `sql:"not null;unique" json:"name" binding:"required"`
	Url           string    `sql:"not null;unique" json:"url" binding:"required"`
	LastPollTime  time.Time `json:"lastPollTime"`
	LastPollError string    `json:"lastPollError"`
}

type FeedItem struct {
	Id         int
	FeedInfoId int
	Guid       string
	AddedOn    time.Time
}

type User struct {
	Id       int    `json:"id"`
	Name     string `sql:"not null;unique" json:"name"`
	Email    string `sql:"not null;unique" json:"email"`
	Enabled  bool   `json:"enabled"`
	Password string `json:"-"`
}

type UserFeed struct {
	Id     int
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
		enabled bool not null,
		password text not null
	);
`
const USER_FEED_TABLE = `
  create table user_feed (
		id integer not null primary key,
		user_id integer not null,
		feed_id integer not null
	);
`

type DBHandle struct {
	DB           gorm.DB
	writeUpdates bool
	syncMutex    sync.Mutex
}

func createAndOpenDb(db_path string, verbose bool, memory bool) *DBHandle {
	mode := "rwc"
	if memory {
		mode = "memory"
	}
	constructed_path := fmt.Sprintf("file:%s?mode=%s", db_path, mode)
	glog.Infof("Opening database %s", constructed_path)

	db, err := gorm.Open("sqlite3", constructed_path)
	if err != nil {
		glog.Fatal(err)
	}
	db.SingularTable(true)
	db.LogMode(verbose)
	db.AutoMigrate(User{})
	db.AutoMigrate(UserFeed{})
	db.AutoMigrate(FeedInfo{})
	db.AutoMigrate(FeedItem{})

	return &DBHandle{DB: db}
}

func NewDBHandle(db_path string, verbose bool, write_updates bool) *DBHandle {
	d := createAndOpenDb(db_path, verbose, false)
	d.writeUpdates = write_updates
	return d
}

func NewMemoryDBHandle(verbose bool, write_updates bool) *DBHandle {
	d := createAndOpenDb("in_memory_test", verbose, true)
	d.writeUpdates = write_updates
	return d
}

func (self *DBHandle) AddFeed(name string, feed_url string) (*FeedInfo, error) {
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
	err = self.DB.Save(f).Error
	return f, err
}

func (self *DBHandle) RemoveFeed(url string, purge_guids bool) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	f, err := self.unsafeGetFeedByUrl(url)

	if err != nil {
		return err
	}
	err = self.DB.Delete(f).Error
	if err == nil {
		err = self.DB.Where("feed_info_id = ?", f.Id).Delete(FeedItem{}).Error
		if err != nil {
			return err
		}
		err = self.DB.Where("feed_id = ?", f.Id).Delete(UserFeed{}).Error
		if err != nil {
			return err
		}

	}
	return err
}

func (self *DBHandle) GetAllFeeds() (feeds []FeedInfo, err error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	err = self.DB.Find(&feeds).Error
	return
}

func (self *DBHandle) GetFeedsWithErrors() (feeds []FeedInfo, err error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	err = self.DB.Where(
		"last_poll_error IS NOT NULL and last_poll_error <> ''").Find(&feeds).Error

	return
}

type staleFeedResult struct {
	Id            int
	Name          string
	Url           string
	LastPollTime  time.Time
	LastPollError string
}

func (self *DBHandle) GetStaleFeeds() ([]FeedInfo, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	var res []FeedInfo
	err := self.DB.Raw(`
	select feed_info.id, feed_info.name, feed_info.url,  r.MaxTime, feed_info.last_poll_error FROM (SELECT feed_info_id, MAX(added_on) as MaxTime FROM feed_item GROUP BY feed_info_id) r, feed_info INNER JOIN feed_item f ON f.feed_info_id = r.feed_info_id AND f.added_on = r.MaxTime AND r.MaxTime < datetime('now','-14 days') AND f.feed_info_id = feed_info.id group by f.feed_info_id;
	`).Scan(&res).Error
	if err != nil {
		return nil, err
	}
	/*
		for rows.Next() {
			var id int
			var name, url, last_poll_error string
			var last_poll_time, max_time time.Time
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
	*/
	return res, nil
}

func (self *DBHandle) GetFeedById(id int) (*FeedInfo, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	feed_info := &FeedInfo{}
	err := self.DB.First(feed_info, id).Error
	return feed_info, err
}

func (self *DBHandle) GetFeedByUrl(url string) (*FeedInfo, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.unsafeGetFeedByUrl(url)
}

func (self *DBHandle) unsafeGetFeedByUrl(url string) (*FeedInfo, error) {
	feed := FeedInfo{}
	err := self.DB.Where("url = ?", url).First(&feed).Error
	return &feed, err
}

func (self *DBHandle) GetFeedItemByGuid(f_id int, guid string) (*FeedItem, error) {
	//TODO: see if beedb will handle this correctly and protect against injection
	//attacks.
	fi := FeedItem{}
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	err := self.DB.Where("feed_info_id = ? AND guid = ?", f_id, guid).First(&fi).Error
	return &fi, err
}

func (self *DBHandle) RecordGuid(feed_id int, guid string) (err error) {
	if self.writeUpdates {
		glog.Infof("Adding GUID '%s' for feed %d", guid, feed_id)
		f := FeedItem{
			FeedInfoId: feed_id,
			Guid:       guid,
			AddedOn:    time.Now(),
		}
		self.syncMutex.Lock()
		defer self.syncMutex.Unlock()

		return self.DB.Save(&f).Error
	}
	return nil
}

// Retrieves the most recent GUIDs for a given feed up to max.  GUIDs are
// returned ordered with the most recent first.
func (self *DBHandle) GetMostRecentGuidsForFeed(f_id int, max int) ([]string, error) {
	var items []FeedItem
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	err := self.DB.Where("feed_info_id=?", f_id).Group("guid").Order("added_on DESC").Limit(max).Find(&items).Error
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

func (self *DBHandle) UpdateFeed(f *FeedInfo) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	return self.DB.Save(f).Error
}

func (self *DBHandle) AddUser(name string, email string, pass string) (*User, error) {
	if name == "" || email == "" || pass == "" {
		return nil, errors.New("name, email or pass can't be empty.")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return nil, fmt.Errorf("Invalid email address: %s", err)
	}

	bcrypt_password, err := bcrypt.GenerateFromPassword([]byte(pass), 10)
	if err != nil {
		return nil, err
	}

	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	u := &User{
		Name:     name,
		Email:    addr.Address,
		Enabled:  true,
		Password: string(bcrypt_password[:]),
	}
	err = self.DB.Save(u).Error
	return u, err
}

func (self *DBHandle) SaveUser(u *User) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.DB.Save(u).Error
}

func (self *DBHandle) SaveFeed(f *FeedInfo) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.DB.Save(f).Error
}

func (self *DBHandle) GetUser(name string) (*User, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	u := &User{}
	err := self.DB.Where("name = ?", name).First(u).Error

	return u, err
}
func (self *DBHandle) GetUserById(id int) (*User, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	u := &User{}
	err := self.DB.First(u, id).Error

	return u, err
}

func (self *DBHandle) GetUserByEmail(name string) (*User, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.unsafeGetUserByEmail(name)
}
func (self *DBHandle) unsafeGetUserByEmail(name string) (*User, error) {
	u := &User{}
	err := self.DB.Where("email = ?", name).Find(u).Error
	return u, err
}

func (self *DBHandle) RemoveUser(user *User) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	return self.unsafeRemoveUser(user)
}
func (self *DBHandle) unsafeRemoveUser(user *User) error {
	err := self.DB.Delete(user).Error
	if err == nil {
		err = self.DB.Where("user_id = ?", user.Id).Delete(UserFeed{}).Error
	}
	return err
}

func (self *DBHandle) RemoveUserByEmail(email string) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	u, err := self.unsafeGetUserByEmail(email)
	if err != nil {
		return err
	}

	return self.unsafeRemoveUser(u)
}

func (self *DBHandle) AddFeedsToUser(u *User, feeds []*FeedInfo) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	for _, f := range feeds {
		uf := &UserFeed{
			UserId: u.Id,
			FeedId: f.Id,
		}
		err := self.DB.Save(uf).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *DBHandle) RemoveFeedsFromUser(u *User, feeds []*FeedInfo) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	for _, f := range feeds {
		err := self.DB.Where("feed_id = ? and user_id = ?", f.Id, u.Id).Delete(UserFeed{}).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *DBHandle) GetAllUsers() ([]User, error) {
	var all []User
	err := self.DB.Find(&all).Error
	return all, err
}

func (self *DBHandle) GetUsersFeeds(u *User) ([]FeedInfo, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	var feed_infos []FeedInfo

	err := self.DB.Table("feed_info").
		Select("feed_info.id,feed_info.name,feed_info.url").
		Joins("INNER join user_feed on user_feed.feed_id = feed_info.id").
		Where("user_feed.user_id = ?", u.Id).
		Order("feed_info.name").
		Group("feed_info.id").
		Scan(&feed_infos).Error
	if err == gorm.RecordNotFound {
		err = nil
	}
	/*
		err := self.Orm.SetTable("feed_info").
			Join("INNER", "user_feed", "feed_info.id=user_feed.feed_id").
			Select("feed_info.id,feed_info.name,feed_info.url").
			Where("user_feed.user_id = ?", u.Id).
			OrderBy("feed_info.name").
			GroupBy("feed_info.id").
			FindAll(&all)
	*/
	return feed_infos, err
}

func (self *DBHandle) UpdateUsersFeeds(u *User, feed_ids []int) error {
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

func (self *DBHandle) GetFeedUsers(f_url string) ([]User, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	var all []User
	err := self.DB.Table("user").
		Select("user.id,user.name,user.email,user.enabled").
		Joins("inner join user_feed on user.id=user_feed.user_id inner join feed_info on feed_info.id=user_feed.feed_id").
		Where("feed_info.url = ?", f_url).
		Group("user.id").
		Find(&all).Error
	/*
		err := self.Orm.SetTable("user").
			Join("INNER", "user_feed", "user.id=user_feed.user_id").
			Join("INNER", "feed_info", "feed_info.id=user_feed.feed_id").
			Where("feed_info.url = ?", f_url).
			GroupBy("user.id").
			Select("user.id,user.name,user.email,user.enabled").FindAll(&all)
	*/
	return all, err
}
