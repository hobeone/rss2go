package feed_watcher

import (
	"errors"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/feed"
	"log"
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
	URI          string
	Body         []byte
	Feed         feed.Feed
	Items         []*feed.Story
	Error        error
}

//
// FEEDWATCHER
//
type FeedWatcher struct {
	URI               string
	poll_now_chan     chan int
	exit_now_chan     chan int
	crawler_chan      chan *FeedCrawlRequest
	response_chan     chan *FeedCrawlResponse
	mailer_chan       chan *mail.MailRequest
	polling           bool // make sure only one PollFeed at a time
	crawling          bool // make sure only one crawl outstanding at a time
	last_item_time    time.Time
	min_sleep_seconds int64
	max_sleep_seconds int64
	db                db.FeedDbDispatcher
}

func NewFeedWatcher(
	uri string,
	crawler_chan chan *FeedCrawlRequest,
	response_channel chan *FeedCrawlResponse,
	mail_channel chan *mail.MailRequest,
	db db.FeedDbDispatcher,
	last_item_time time.Time,
	min_sleep int64,
	max_sleep int64,
) *FeedWatcher {
	return &FeedWatcher{
		URI:               uri,
		poll_now_chan:     make(chan int),
		exit_now_chan:     make(chan int),
		crawler_chan:      crawler_chan,
		response_chan:     response_channel,
		mailer_chan:       mail_channel,
		polling:           false,
		crawling:          false,
		min_sleep_seconds: min_sleep,
		max_sleep_seconds: max_sleep,
		last_item_time:    last_item_time,
		db:                db,
	}
}

func (self *FeedWatcher) lockPoll() (r bool) {
	if self.polling {
		return false
	}
	self.polling = true
	return true
}

func (self *FeedWatcher) unlockPoll() (r bool) {
	self.polling = false
	return true
}

func (self *FeedWatcher) Polling() (r bool) {
	return self.polling
}

func (self *FeedWatcher) lockCrawl() (r bool) {
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

func (self *FeedWatcher) PollFeed() bool {
	if !self.lockPoll() {
		log.Printf("Called PollLoop on %v when already polling. ignoring.\n",
			self.URI)
		return false
	}
	defer self.unlockPoll()

	for {
		select {
		case <-self.exit_now_chan:
			log.Printf("Stopping poll of %v", self.URI)
			return true
		default:
			log.Printf("Polling feed %v", self.URI)
			resp := self.doCrawl()
			to_sleep := self.min_sleep_seconds
			if resp.Error != nil {
				log.Printf("Error getting feed %v: %v", self.URI, resp.Error)
				self.response_chan <- resp
				continue
			}
			log.Printf("Got response to crawl of %v", resp.URI)
			feed, stories, errors := feed.ParseFeed(resp.URI, resp.Body)

			if feed == nil || stories == nil {
				log.Printf("Error parsing response from %s: %s", resp.URI, errors[0])
				resp.Error = errors[0]
				self.response_chan <- resp
				continue
			}

			resp.Items = self.filterNewItems(stories, self.last_item_time)

			log.Printf("Feed %s has new %d items", feed.Title, len(resp.Items))
			for _, item := range resp.Items {
				log.Printf("New Story: %s, sending for mail.", item.Title)
				err := self.sendMail(item)
				if err != nil {
					log.Printf("Error sending mail: %s", err.Error())
				} else {
					// Now that the item is safely away set the last updated time so we
					// don't send a duplicate.
					if item.Published.After(self.last_item_time) {
						self.last_item_time = item.Published
					}
				}
			}
			self.updateDB(self.last_item_time)

			self.response_chan <- resp

			// TODO extract proper amount of time to sleep.

			log.Printf("Sleeping %v seconds for %v", to_sleep, self.URI)
			Sleep(time.Second * time.Duration(to_sleep))
		}
	}
	return false
}

func (self *FeedWatcher) updateDB(last_update time.Time) {
	self.db.UpdateFeedLastItemTimeByUrl(self.URI, last_update)
}

func (self *FeedWatcher) filterNewItems(stories []*feed.Story, last_date time.Time) []*feed.Story {
	log.Printf("Filtering stories older than %v", last_date)
	new_stories := []*feed.Story{}
	for _, story := range stories {
		if story.Published.After(self.last_item_time) {
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
			URI:   self.URI,
			Error: errors.New("Already crawling " + self.URI),
		}
	}
	self.lockCrawl()
	defer self.unlockCrawl()

	req := &FeedCrawlRequest{
		URI:          self.URI,
		ResponseChan: make(chan *FeedCrawlResponse),
	}
	self.crawler_chan <- req
	resp := <-req.ResponseChan
	return resp
}

func (self *FeedWatcher) StopPoll() {
	self.exit_now_chan <- 1
}
