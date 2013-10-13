package db

import (
	"database/sql"
	"fmt"
	"github.com/astaxie/beedb"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"sync"
	"time"
)

type FeedDbDispatcher interface {
	GetAllFeeds() ([]FeedInfo, error)
	GetFeedItemByGuid(string) (*FeedItem, error)
	RecordGuid(int, string) error
	GetGuidsForFeed(int, *[]string) (*[]string, error)
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
		added_on DATE NULL
	);
`

func createAndOpenDb(db_path string, verbose bool, memory bool) beedb.Model {
	beedb.OnDebug = verbose
	log.Printf("Opening database %s", db_path)
	mode := "rwc"
	if memory {
		mode = "memory"
	}
	db, err := sql.Open("sqlite3",
		fmt.Sprintf("file:%s?mode=%s", db_path, mode))

	if err != nil {
		log.Fatal(err)
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table';")
	if err != nil {
		log.Fatal("Couldn't get list of tables from database.")
	}
	tables := make(map[string]bool)
	for rows.Next() {
    var name string
    if err := rows.Scan(&name); err != nil {
        log.Fatal(err)
    }
		tables[name] = true
	}

	if _, ok := tables["feed_info"]; !ok {
		createTable(db, FEED_INFO_TABLE)
	}
	if _, ok := tables["feed_item"]; !ok {
		createTable(db, FEED_ITEM_TABLE)
	}
	return beedb.New(db)
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
	writeUpdates bool
	syncMutex    sync.Mutex
}

func NewDbDispatcher(db_path string, verbose bool, write_updates bool) *DbDispatcher {
	return &DbDispatcher{
		Orm:          createAndOpenDb(db_path, verbose, false),
		writeUpdates: write_updates,
	}
}

func NewMemoryDbDispatcher(verbose bool, write_updates bool) *DbDispatcher {
	return &DbDispatcher{
		Orm:          createAndOpenDb("in_memory_test", verbose, true),
		writeUpdates: write_updates,
	}
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

func (self *DbDispatcher) GetFeedByUrl(url string) (*FeedInfo, error) {
	feed := FeedInfo{}
	err := self.Orm.Where("url = ?", url).Find(&feed)
	return &feed, err
}

func (self *DbDispatcher) GetFeedItemByGuid(guid string) (
	feed_item *FeedItem, err error) {
	//TODO: see if beedb will handle this correctly and protect against injection
	//attacks.
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()
	err = self.Orm.Where("guid = ?", guid).Find(&feed_item)
	return
}

func (self *DbDispatcher) RecordGuid(feed_id int, guid string) (err error) {
	if self.writeUpdates {
		log.Printf("Adding GUID '%s' for feed %d", guid, feed_id)
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

func (self *DbDispatcher) GetGuidsForFeed(feed_id int, guids *[]string) (*[]string, error) {
	var allitems []FeedItem
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	err := self.Orm.Where("feed_info_id=?", feed_id).GroupBy("guid").FindAll(&allitems)
	if err != nil {
		return &[]string{}, err
	}
	log.Printf("Got %d results for guids.", len(allitems))
	known_guids := make([]string, len(allitems))
	for i, v := range allitems {
		known_guids[i] = v.Guid
	}
	return &known_guids, nil
}

func (self *DbDispatcher) UpdateFeed(f *FeedInfo) error {
	self.syncMutex.Lock()
	defer self.syncMutex.Unlock()

	return self.Orm.Save(f)
}
