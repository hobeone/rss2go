package db

import (
	"github.com/astaxie/beedb"
	"time"
	"log"
)

type DbDispatcher struct {
	OrmHandle beedb.Model
}

func NewDbDispatcher(db_path string) DbDispatcher {
	return DbDispatcher {
		OrmHandle: CreateAndOpenDb(db_path, true),
	}
}

// Not sure if I need to have just one writer but funneling everything through
// the dispatcher for now.
func (self *DbDispatcher) GetAllFeeds() (feeds []FeedInfo, err error) {
	err = self.OrmHandle.FindAll(&feeds)
	return
}

func (self *DbDispatcher) UpdateFeedTimesByUrl(
	url string , poll_time time.Time, item_time time.Time) error {
	t := make(map[string]interface{})
	if !poll_time.IsZero() {
		t["last_poll_time"] = poll_time
	}
	if !item_time.IsZero() {
		t["last_item_time"] = item_time
	}

	log.Printf("Updating %s with last_poll_time: %v, last_item_time: %v",
		url, poll_time, item_time)

	_, err := self.OrmHandle.SetTable("feed_info").Where("url = ?", url).Update(t)
	return err
}
