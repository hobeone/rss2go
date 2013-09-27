package db

import (
	"database/sql"
	"github.com/astaxie/beedb"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"time"
)

type FeedDbDispatcher interface {
	GetAllFeeds() ([]FeedInfo, error)
	UpdateFeedLastItemTimeByUrl(string, time.Time) error
}

type FeedInfo struct {
	Id           int
	Name         string
	Url          string
	LastPollTime time.Time
	LastItemTime time.Time
}

const FEED_INFO_TABLE = `
	create table feed_info (
		id integer not null primary key,
		name text not null,
		url text not null,
		last_poll_time DATE NULL,
		last_item_time DATE NULL
	);
`

func CreateAndOpenDb(db_path string, verbose bool) beedb.Model {
	beedb.OnDebug = verbose
	log.Printf("Opening database %s", db_path)
	db, err := sql.Open("sqlite3", db_path)
	if err != nil {
		log.Print(err)
	}

	db.Exec(FEED_INFO_TABLE)
	// Always get error if table already exists

	return beedb.New(db)
}

type DbDispatcher struct {
	OrmHandle beedb.Model
}

func NewDbDispatcher(db_path string, verbose bool) *DbDispatcher {
	return &DbDispatcher{
		OrmHandle: CreateAndOpenDb(db_path, verbose),
	}
}

// Not sure if I need to have just one writer but funneling everything through
// the dispatcher for now.
func (self *DbDispatcher) GetAllFeeds() (feeds []FeedInfo, err error) {
	err = self.OrmHandle.FindAll(&feeds)
	return
}

func (self *DbDispatcher) UpdateFeedLastItemTimeByUrl(
	url string, item_time time.Time) error {
	t := make(map[string]interface{})
	t["last_item_time"] = item_time

	log.Printf("Updating %s with last_item_time: %v", url, item_time)

	_, err := self.OrmHandle.SetTable("feed_info").Where("url = ?", url).Update(t)
	return err
}
