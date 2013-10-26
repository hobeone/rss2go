package db

import (
	"database/sql"
	"fmt"
	"github.com/astaxie/beedb"
	"github.com/golang/glog"
	_ "github.com/mattn/go-sqlite3"
	"sync"
	"time"
)

type FeedDbDispatcher interface {
	GetAllFeeds() ([]FeedInfo, error)
	GetFeedItemByGuid(int, string) (*FeedItem, error)
	RecordGuid(int, string) error
	GetMostRecentGuidsForFeed(int, int) ([]string, error)
	GetFeedByUrl(string) (*FeedInfo, error)
	AddFeed(string, string) (*FeedInfo, error)
	RemoveFeed(string, bool) error
	UpdateFeed(*FeedInfo) error
}

type FeedInfo struct {
	Id            int `beedb:"PK"`
	Name          string
	Url           string
	LastPollTime  time.Time
	LastPollError string
}

type FeedItem struct {
	Id         int `beedb:"PK"`
	FeedInfoId int
	Guid       string
	AddedOn    time.Time
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
		glog.Fatal("Couldn't get list of tables from database.")
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

func (self *DbDispatcher) AddFeed(name string, url string) (*FeedInfo, error) {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	f := &FeedInfo{
		Name: name,
		Url:  url,
	}
	err := self.Orm.Save(f)
	return f, err
}

func (self *DbDispatcher) RemoveFeed(url string, purge_guids bool) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	f, err := self.GetFeedByUrl(url)
	if err != nil {
		return err
	}
	_, err = self.Orm.Delete(f)
	self.Orm.SetTable("feed_item").Where("feed_info_id = ?", f.Id).DeleteRow()
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
	select feed_info.name, feed_info.url, feed_info.last_poll_time, feed_info.last_poll_error, r.MaxTime FROM (SELECT feed_info_id, MAX(added_on) as MaxTime FROM feed_item GROUP BY feed_info_id) r, feed_info INNER JOIN feed_item f ON f.feed_info_id = r.feed_info_id AND f.added_on = r.MaxTime AND r.MaxTime < datetime('now','-14 days') AND f.feed_info_id = feed_info.id group by f.feed_info_id;
	`)
	if err != nil {
		return
	}
	for rows.Next() {
		var name, url, last_poll_error, last_poll_time, max_time string
		err = rows.Scan(&name, &url, &last_poll_time, &last_poll_error, &max_time)
		if err != nil {
			return
		}
		ftime, err := time.Parse("2006-01-02 15:04:05", max_time)
		if err != nil {
			return nil, err
		}
		// Sorta hacky: set LastPollTime to time of last item seen, rather than
		// last time feed was polled.
		feeds = append(feeds, FeedInfo{
			Name:          name,
			Url:           url,
			LastPollTime:  ftime,
			LastPollError: last_poll_error,
		})
	}

	return
}

func (self *DbDispatcher) GetFeedByUrl(url string) (*FeedInfo, error) {
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
		var f FeedItem
		f.FeedInfoId = feed_id
		f.Guid = guid
		f.AddedOn = time.Now()
		self.syncMutex.Lock()
		defer self.syncMutex.Unlock()

		return self.Orm.Save(&f)
	}
	return
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
