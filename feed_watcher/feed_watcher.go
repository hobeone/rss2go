package feed_watcher

import (
	"errors"
	"fmt"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/mail"
	"log"
	"math/rand"
	"time"
)

// Allow for mocking out in test
var Sleep = func(d time.Duration) {
	time.Sleep(d)
	return
}

//
//FeedCrawlRequest
//
type FeedCrawlRequest struct {
	URI          string
	ResponseChan chan *FeedCrawlResponse
}

//
// FeedCrawlResponse
//
type FeedCrawlResponse struct {
	URI                string
	Body               []byte
	Feed               feed.Feed
	Items              []*feed.Story
	HttpResponseStatus string
	Error              error
}

//
// FEEDWATCHER
//
type FeedWatcher struct {
	FeedInfo          db.FeedInfo
	poll_now_chan     chan int
	exit_now_chan     chan int
	crawler_chan      chan *FeedCrawlRequest
	response_chan     chan *FeedCrawlResponse
	mailer_chan       chan *mail.MailRequest
	polling           bool // make sure only one PollFeed at a time
	crawling          bool // make sure only one crawl outstanding at a time
	min_sleep_seconds int64
	max_sleep_seconds int64
	db                db.FeedDbDispatcher
	KnownGuids        map[string]bool
	LastCrawlResponse FeedCrawlResponse
}

func NewFeedWatcher(
	feed_info db.FeedInfo,
	crawler_chan chan *FeedCrawlRequest,
	response_channel chan *FeedCrawlResponse,
	mail_channel chan *mail.MailRequest,
	db db.FeedDbDispatcher,
	known_guids []string,
	min_sleep int64,
	max_sleep int64,
) *FeedWatcher {
	guids := map[string]bool{}
	for _, i := range known_guids {
		guids[i] = true
	}
	return &FeedWatcher{
		FeedInfo:          feed_info,
		poll_now_chan:     make(chan int),
		exit_now_chan:     make(chan int),
		crawler_chan:      crawler_chan,
		response_chan:     response_channel,
		mailer_chan:       mail_channel,
		polling:           false,
		crawling:          false,
		min_sleep_seconds: min_sleep,
		max_sleep_seconds: max_sleep,
		db:                db,
		KnownGuids:        guids,
		LastCrawlResponse: FeedCrawlResponse{},
	}
}

func (self *FeedWatcher) lockPoll() bool {
	if self.polling {
		return false
	}
	self.polling = true
	return true
}

func (self *FeedWatcher) unlockPoll() bool {
	self.polling = false
	return true
}

func (self *FeedWatcher) Polling() bool {
	return self.polling
}

func (self *FeedWatcher) lockCrawl() bool {
	if self.crawling {
		return false
	}
	self.crawling = true
	return true
}

func (self *FeedWatcher) unlockCrawl() (r bool) {
	self.crawling = false
	return true
}

func (self *FeedWatcher) Crawling() (r bool) {
	return self.crawling
}

func (self *FeedWatcher) UpdateFeed() *FeedCrawlResponse {
	log.Printf("Polling feed %v", self.FeedInfo.Url)
	resp := self.doCrawl()

	if resp.Error != nil {
		log.Printf("Error getting feed %v: %v", self.FeedInfo.Url, resp.Error)
		return resp
	}
	log.Printf("Got response to crawl of %v", resp.URI)
	feed, stories, err := feed.ParseFeed(resp.URI, resp.Body)

	if feed == nil || stories == nil {
		if err != nil {
			log.Printf("Error parsing response from %s: %#v", resp.URI, err)
			resp.Error = err
		} else {
			e := fmt.Errorf("No items found in %s", resp.URI)
			log.Print(e)
			resp.Error = e
		}
		return resp
	}

	// If we don't know about any GUIDs for this feed we haven't been
	// initalized yet.  Check the GUIDs in the feed we just read and see if
	// they exist in the DB.  That should be the maximum we should need to
	// load into memory.
	if len(self.KnownGuids) == 0 {
		var err error
		self.KnownGuids, err = self.LoadGuidsFromDb(stories)
		if err != nil {
			e := fmt.Errorf("Error getting Guids from DB: %s", err)
			log.Print(e)
			resp.Error = e
			return resp
		}
		log.Printf("Loaded %d known guids for Feed %s.",
			len(self.KnownGuids), self.FeedInfo.Url)
	}

	resp.Items = self.filterNewItems(stories, self.KnownGuids)

	log.Printf("Feed %s has new %d items", feed.Title, len(resp.Items))
	for _, item := range resp.Items {
		log.Printf("New Story: %s, sending for mail.", item.Title)
		err := self.sendMail(item)
		if err != nil {
			log.Printf("Error sending mail: %s", err.Error())
		} else {
			err := self.RecordGuid(item.Id)
			if err != nil {
				e := fmt.Errorf("Error writing guid to db: %s", err)
				resp.Error = e
				log.Print(e)
			} else {
				log.Printf("Added guid %s for feed %s", item.Id, self.FeedInfo.Url)
			}
		}
	}
	return resp
}

func (self *FeedWatcher) PollFeed() bool {
	if !self.lockPoll() {
		log.Printf("Called PollLoop on %v when already polling. ignoring.\n",
			self.FeedInfo.Url)
		return false
	}
	defer self.unlockPoll()

	for {
		select {
		case <-self.exit_now_chan:
			log.Printf("Stopping poll of %v", self.FeedInfo.Url)
			return true
		default:
			resp := self.UpdateFeed()
			self.LastCrawlResponse = *resp
			self.response_chan <- resp
			self.Sleep(self.min_sleep_seconds)
		}
	}
}

// Sleep for the given amount of seconds plus a upto 60 extra seconds.
func (self *FeedWatcher) Sleep(tosleep int64) {
	s := tosleep + rand.Int63n(60)
	log.Printf("Sleeping %d seconds for %s", s, self.FeedInfo.Url)
	Sleep(time.Second * time.Duration(s))
}

func (self *FeedWatcher) LoadGuidsFromDb(stories []*feed.Story) (map[string]bool, error) {
	guids := make([]string, len(stories))
	for i, v := range stories {
		guids[i] = v.Id
	}
	known, err := self.db.CheckGuidsForFeed(self.FeedInfo.Id, &guids)
	if err != nil {
		return make(map[string]bool), nil
	}
	ret := make(map[string]bool)
	for _, v := range *known {
		ret[v] = true
	}
	return ret, nil
}

func (self *FeedWatcher) RecordGuid(guid string) error {
	self.KnownGuids[guid] = true
	err := self.db.RecordGuid(self.FeedInfo.Id, guid)
	return err
}

func (self *FeedWatcher) filterNewItems(stories []*feed.Story, guids map[string]bool) []*feed.Story {
	log.Printf("Filtering stories we already know about.")
	new_stories := []*feed.Story{}
	for _, story := range stories {
		if _, found := self.KnownGuids[story.Id]; !found {
			new_stories = append(new_stories, story)
		}
	}
	return new_stories
}

func (self *FeedWatcher) sendMail(item *feed.Story) error {
	req := &mail.MailRequest{
		Item:       item,
		ResultChan: make(chan error),
	}
	self.mailer_chan <- req
	resp := <-req.ResultChan
	return resp
}

func (self *FeedWatcher) doCrawl() (r *FeedCrawlResponse) {
	if self.crawling {
		return &FeedCrawlResponse{
			URI:   self.FeedInfo.Url,
			Error: errors.New("Already crawling " + self.FeedInfo.Url),
		}
	}
	self.lockCrawl()
	defer self.unlockCrawl()

	req := &FeedCrawlRequest{
		URI:          self.FeedInfo.Url,
		ResponseChan: make(chan *FeedCrawlResponse),
	}
	self.crawler_chan <- req
	resp := <-req.ResponseChan
	return resp
}

func (self *FeedWatcher) StopPoll() {
	self.exit_now_chan <- 1
}
