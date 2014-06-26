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
	"math"
	"math/rand"
	"time"

	"github.com/golang/glog"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/mail"
)

// Allow for mocking out in test.
var After = func(d time.Duration) <-chan time.Time {
	return time.After(d)
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
	exit_chan         chan int
	crawl_chan        chan *FeedCrawlRequest
	resp_chan         chan *FeedCrawlResponse
	mailer_chan       chan *mail.MailRequest
	polling           bool // make sure only one PollFeed at a time
	crawling          bool // make sure only one crawl outstanding at a time
	min_sleep_time    time.Duration
	max_sleep_time    time.Duration
	dbh               *db.DBHandle
	KnownGuids        map[string]bool
	LastCrawlResponse *FeedCrawlResponse
}

func NewFeedWatcher(
	feed_info db.FeedInfo,
	crawl_chan chan *FeedCrawlRequest,
	resp_chan chan *FeedCrawlResponse,
	mail_chan chan *mail.MailRequest,
	dbh *db.DBHandle,
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
		exit_chan:         make(chan int),
		crawl_chan:        crawl_chan,
		resp_chan:         resp_chan,
		mailer_chan:       mail_chan,
		polling:           false,
		crawling:          false,
		min_sleep_time:    time.Duration(min_sleep) * time.Second,
		max_sleep_time:    time.Duration(max_sleep) * time.Second,
		dbh:               dbh,
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
	self.dbh.UpdateFeed(&self.FeedInfo)

	return resp
}

// Core logic to poll a feed, find new items, add those to the database, and
// send them for mail.
func (self *FeedWatcher) updateFeed() *FeedCrawlResponse {
	glog.Infof("Polling feed %v", self.FeedInfo.Url)
	resp := self.doCrawl()

	if resp.Error != nil {
		glog.Infof("Error getting feed %v: %v", self.FeedInfo.Url, resp.Error)
		return resp
	}
	glog.Infof("Got response to crawl of %v of length (%d)", resp.URI,
		len(resp.Body))
	feed, stories, err := feed.ParseFeed(resp.URI, resp.Body)

	if feed == nil || stories == nil {
		if err != nil {
			glog.Infof("Error parsing response from %s: %#v", resp.URI, err)
			resp.Error = err
		} else {
			e := fmt.Errorf("No items found in %s", resp.URI)
			glog.Info(e)
			resp.Error = e
		}
		return resp
	}

	/*
	 * Load most recent X * 1.1 Guids from Db
	 * Filter New Stories
	 * Send New Stories for mailing
	 *  Add sent guid to list
	 * prune Guids back to X * 1.1
	 */

	// If we don't know about any GUIDs for this feed we haven't been
	// initalized yet.  Check the GUIDs in the feed we just read and see if
	// they exist in the DB.  That should be the maximum we should need to
	// load into memory.

	guids_to_load := int(math.Ceil(float64(len(stories)) * 1.1))

	glog.Infof("Got %d stories from feed %s.", len(stories), self.FeedInfo.Url)

	if len(self.KnownGuids) == 0 {
		var err error
		self.KnownGuids, err = self.LoadGuidsFromDb(guids_to_load)
		if err != nil {
			e := fmt.Errorf("Error getting Guids from DB: %s", err)
			glog.Info(e)
			resp.Error = e
			return resp
		}
		glog.Infof("Loaded %d known guids for Feed %s.",
			len(self.KnownGuids), self.FeedInfo.Url)
	}

	resp.Items = self.filterNewItems(stories)
	glog.Infof("Feed %s has %d new items", feed.Title, len(resp.Items))

	for _, item := range resp.Items {
		item.Title = fmt.Sprintf("%s: %s", self.FeedInfo.Name, item.Title)
		glog.Infof("New Story: %s, sending for mail.", item.Title)
		err := self.sendMail(item)
		if err != nil {
			glog.Infof("Error sending mail: %s", err.Error())
		} else {
			err := self.recordGuid(item.Id)
			if err != nil {
				e := fmt.Errorf("Error writing guid to db: %s", err)
				resp.Error = e
				glog.Info(e)
			} else {
				glog.Infof("Added guid %s for feed %s", item.Id, self.FeedInfo.Url)
			}
		}
	}

	// Prune guids we know about back down to X * 1.1 but only if there were new
	// items otherwise it would be noop.
	if len(resp.Items) > 0 {
		self.KnownGuids, err = self.LoadGuidsFromDb(guids_to_load)
		if err != nil {
			e := fmt.Errorf("Error getting Guids from DB: %s", err)
			glog.Info(e)
			resp.Error = e
		}
	}

	return resp
}

// Designed to be called as a Goroutine.  Use UpdateFeed to just update once.
func (self *FeedWatcher) PollFeed() bool {
	if !self.lockPoll() {
		glog.Infof("Called PollLoop on %v when already polling. ignoring.\n",
			self.FeedInfo.Url)
		return false
	}
	defer self.unlockPoll()

	time_since_last_poll := time.Since(self.FeedInfo.LastPollTime)
	to_sleep := time.Duration(0)
	if time_since_last_poll < self.min_sleep_time {
		to_sleep = self.min_sleep_time - time_since_last_poll
		glog.Infof("Last poll of %s was only %v ago, waiting at least %v.",
			self.FeedInfo.Url, time_since_last_poll, to_sleep)
	}
	for {
		select {
		case <-self.exit_chan:
			glog.Infof("Got exit signal, stopping poll of %v", self.FeedInfo.Url)
			return true
		case <-self.afterWithJitter(to_sleep):
			self.LastCrawlResponse = self.UpdateFeed()
			self.resp_chan <- self.LastCrawlResponse
			to_sleep = self.min_sleep_time
			if self.LastCrawlResponse.Error != nil {
				to_sleep = self.max_sleep_time
				glog.Infof("Feed %s had error. Waiting maxium allowed time before next poll.",
					self.FeedInfo.Url)
			}
		}
	}
}

// call time.After for the given amount of seconds plus a up to 60 extra seconds.
func (self *FeedWatcher) afterWithJitter(d time.Duration) <-chan time.Time {
	s := d + time.Duration(rand.Int63n(60))*time.Second
	glog.Infof("Waiting %v until next poll of %s", s, self.FeedInfo.Url)
	return After(s)
}

// Populate internal cache of GUIDs by getting the most recent GUIDs it knows
// about.  To retrieve all GUIDs set max to -1.
func (self *FeedWatcher) LoadGuidsFromDb(max int) (map[string]bool, error) {
	guids, err := self.dbh.GetMostRecentGuidsForFeed(self.FeedInfo.Id, max)
	if err != nil {
		return nil, err
	}
	ret := make(map[string]bool)
	for _, v := range guids {
		ret[v] = true
	}
	return ret, nil
}

func (self *FeedWatcher) recordGuid(guid string) error {
	self.KnownGuids[guid] = true
	return self.dbh.RecordGuid(self.FeedInfo.Id, guid)
}

func (self *FeedWatcher) filterNewItems(stories []*feed.Story) []*feed.Story {
	glog.Infof("Filtering stories we already know about.")
	new_stories := []*feed.Story{}
	for _, story := range stories {
		if _, found := self.KnownGuids[story.Id]; !found {
			new_stories = append(new_stories, story)
		}
	}
	return new_stories
}

func (self *FeedWatcher) sendMail(item *feed.Story) error {
	_, err := self.dbh.GetFeedItemByGuid(self.FeedInfo.Id, item.Id)
	// Guid found, so sending would be a duplicate.
	if err == nil {
		glog.Warningf("Tried to send duplicate GUID: %s for feed %s",
			item.Id, self.FeedInfo.Url)
		return nil
	}

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
	self.crawl_chan <- req
	resp := <-req.ResponseChan
	return resp
}

// Call to have a PollFeed loop exit.
func (self *FeedWatcher) StopPoll() {
	if self.Polling() {
		self.exit_chan <- 1
	}
}
