package db
//https://menteslibres.net/gosexy/db/wrappers/sqlite
//
//https://github.com/astaxie/beedb
//

import "github.com/astaxie/beedb"
import "database/sql"
import (
	"log"
	_ "github.com/mattn/go-sqlite3"
	"time"
)

type FeedInfo struct {
		Id int
    Name    string
    Url  string
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
	beedb.OnDebug=verbose
	log.Printf("Opening database %s", db_path)
	db, err := sql.Open("sqlite3", db_path)
	if err != nil {
		log.Print(err)
	}

	db.Exec(FEED_INFO_TABLE)
	// Always get error if table already exists

	return beedb.New(db)
}

func GetAllFeeds(orm beedb.Model) (feeds []FeedInfo, err error) {
	err = orm.FindAll(&feeds)
	return
}

func UpdateFeedPollTime(orm beedb.Model, feed *FeedInfo) error {
	feed.LastPollTime = time.Now()
	return orm.Save(feed)
}
