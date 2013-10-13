/*

FeedWatcher watches a feed.  Duh.

One feed watcher watches one feed.  It doesn't actually do the crawling itself
but will send a FeedCrawlRequest to a crawler.

To watch a feed create a FeedWatcher instance and then call PollFeed() on it
(usually in a goroutine).

FeedWatcher will keep track of the GUIDs it has seen from a feed so it will
only notify on new items.

See bin/runone.go or bin/daemon.go for example usage.
*/
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

// Allow for mocking out in test.
var Sleep = func(d time.Duration) {
	time.Sleep(d)
	return
}

//
// Sent to a crawler instance.
//
type FeedCrawlRequest struct {
	URI          string
	ResponseChan chan *FeedCrawlResponse
}

//
// What we want back from the crawler.
//
// Body, Feed, Items and HttpResponseStatus are only guaranteed to be non zero
// value if Error is non nil.
//
type FeedCrawlResponse struct {
	URI                string
	Body               []byte
	Feed               *feed.Feed
	Items              []*feed.Story
	HttpResponseStatus string
	Error              error
}

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
	LastCrawlResponse *FeedCrawlResponse
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
		LastCrawlResponse: &FeedCrawlResponse{},
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


// Returns true if the FeedWatcher is Polling a feed.
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

// Returns true if the FeedWatcher is currently polling the feed (actually
// waiting for a response from the crawler).
func (self *FeedWatcher) Crawling() (r bool) {
	return self.crawling
}

func (self *FeedWatcher) UpdateFeed() *FeedCrawlResponse {
	resp := self.updateFeed()
	// Update DB Record
	self.FeedInfo.LastPollTime = time.Now()
	self.FeedInfo.LastPollError = ""
	if resp.Error != nil {
		self.FeedInfo.LastPollError = resp.Error.Error()
	}
	self.db.UpdateFeed(&self.FeedInfo)

	return resp
}
// Core logic to poll a feed, find new items, add those to the database, and
// send them for mail.
func (self *FeedWatcher) updateFeed() *FeedCrawlResponse {
	log.Printf("Polling feed %v", self.FeedInfo.Url)
	resp := self.doCrawl()

	if resp.Error != nil {
		log.Printf("Error getting feed %v: %v", self.FeedInfo.Url, resp.Error)
		return resp
	}
	log.Printf("Got response to crawl of %v of length (%d)", resp.URI,
		len(resp.Body))
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
			err := self.recordGuid(item.Id)
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

// Designed to be called as a Goroutine.  Use UpdateFeed to just update once.
func (self *FeedWatcher) PollFeed() bool {
	if !self.lockPoll() {
		log.Printf("Called PollLoop on %v when already polling. ignoring.\n",
			self.FeedInfo.Url)
		return false
	}
	defer self.unlockPoll()

	seconds_since_last_poll := int64(time.Since(self.FeedInfo.LastPollTime).Seconds())
	if seconds_since_last_poll < self.min_sleep_seconds {
		log.Printf("Last poll of %s was only %d seconds ago, sleeping.",
			self.FeedInfo.Url, seconds_since_last_poll)
		self.sleep(self.min_sleep_seconds - seconds_since_last_poll)
	}
	for {
		select {
		case <-self.exit_now_chan:
			log.Printf("Stopping poll of %v", self.FeedInfo.Url)
			return true
		default:
			resp := self.UpdateFeed()
			self.LastCrawlResponse = resp
			self.response_chan <- resp
			to_sleep := self.min_sleep_seconds
			if resp.Error != nil {
				log.Printf("Feed %s had error sleepming maxium allowed time.",
					self.FeedInfo.Url)
				to_sleep = self.max_sleep_seconds
			}
			self.sleep(to_sleep)
		}
	}
}

// Sleep for the given amount of seconds plus a upto 60 extra seconds.
func (self *FeedWatcher) sleep(tosleep int64) {
	s := tosleep + rand.Int63n(60)
	log.Printf("Sleeping %d seconds for %s", s, self.FeedInfo.Url)
	Sleep(time.Second * time.Duration(s))
}

// Populate internal cache of GUIDs from the database.
func (self *FeedWatcher) LoadGuidsFromDb(stories []*feed.Story) (map[string]bool, error) {
	guids := make([]string, len(stories))
	for i, v := range stories {
		guids[i] = v.Id
	}
	known, err := self.db.GetGuidsForFeed(self.FeedInfo.Id, &guids)
	if err != nil {
		return make(map[string]bool), err
	}
	ret := make(map[string]bool)
	for _, v := range *known {
		ret[v] = true
	}
	return ret, nil
}

func (self *FeedWatcher) recordGuid(guid string) error {
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

// Call to have a PollFeed loop exit.
func (self *FeedWatcher) StopPoll() {
	self.exit_now_chan <- 1
}
