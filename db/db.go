package db

import (
	"database/sql"
	"fmt"
	"github.com/astaxie/beedb"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"time"
)

type FeedDbDispatcher interface {
	GetAllFeeds() ([]FeedInfo, error)
	GetFeedItemByGuid(string) (*FeedItem, error)
	RecordGuid(int, string) error
	CheckGuidsForFeed(int, *[]string) (*[]string, error)
}

type FeedInfo struct {
	Id           int `beedb:"PK"`
	Name         string
	Url          string
	LastPollTime time.Time
	LastItemTime time.Time
}

type FeedItem struct {
	Id         int `beedb:"PK"`
	FeedInfoId int
	Guid       string
}

const FEED_INFO_TABLE = `
	create table feed_info (
		id integer not null primary key,
		name text not null UNIQUE,
		url text not null UNIQUE,
		last_poll_time DATE NULL,
		last_item_time DATE NULL
	);
`
const FEED_ITEM_TABLE = `
	create table feed_item (
		id integer not null primary key,
		feed_info_id integer not null,
		guid text not null
	);
`

func CreateAndOpenDb(db_path string, verbose bool) beedb.Model {
	beedb.OnDebug = verbose
	log.Printf("Opening database %s", db_path)
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=rwc", db_path))

	if err != nil {
		log.Fatal(err)
	}

	db.Exec(FEED_INFO_TABLE)
	db.Exec(FEED_ITEM_TABLE)
	// Always get error if table already exists

	return beedb.New(db)
}

type DbDispatcher struct {
	OrmHandle beedb.Model
	WriteUpdates bool
}

func NewDbDispatcher(db_path string, verbose bool, write_updates bool) *DbDispatcher {
	return &DbDispatcher{
		OrmHandle: CreateAndOpenDb(db_path, verbose),
		WriteUpdates: write_updates,
	}
}

// Not sure if I need to have just one writer but funneling everything through
// the dispatcher for now.
func (self *DbDispatcher) GetAllFeeds() (feeds []FeedInfo, err error) {
	err = self.OrmHandle.FindAll(&feeds)
	return
}

func (self *DbDispatcher) GetFeedItemByGuid(guid string) (
	feed_item *FeedItem, err error) {
	//TODO: see if beedb will handle this correctly and protect against injection
	//attacks.
	err = self.OrmHandle.Where("guid = ?", guid).Find(&feed_item)
	return
}

func (self *DbDispatcher) RecordGuid(feed_id int, guid string) (err error) {
	if self.WriteUpdates {
		log.Printf("Adding GUID '%s' for feed %d", guid, feed_id)
		var f FeedItem
		f.FeedInfoId = feed_id
		f.Guid = guid
		return self.OrmHandle.Save(&f)
	}
	return
}

func (self *DbDispatcher) CheckGuidsForFeed(feed_id int, guids *[]string) (*[]string, error) {
	/*
	s := make([]string, len(*guids))
	args := make([]interface{}, len(*guids)+1)
	args[0] = strconv.Itoa(feed_id)
	for i, v := range *guids {
		s[i] = "?"
		args[i+1] = v
	}
	q := fmt.Sprintf("feed_info_id = ? AND guid IN (%s)", strings.Join(s, ","))
	*/

	var allitems []FeedItem
	err := self.OrmHandle.Where("feed_info_id=?", feed_id).GroupBy("guid").FindAll(&allitems)
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
