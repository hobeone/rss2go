package feed_watcher

import (
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"errors"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/mail"
	rss "github.com/jteeuwen/go-pkg-rss"
	"github.com/moovweb/gokogiri"
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
	Feed         rss.Feed
	Error        error
	CrawlTime    time.Time
	LastItemTime time.Time
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
		min_sleep_seconds: 60,
		max_sleep_seconds: 3600,
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

func chanHandler(feed *rss.Feed, newchannels []*rss.Channel) {
}

func itemHandler(feed *rss.Feed, ch *rss.Channel, newitems []*rss.Item) {
	log.Printf("%d new item(s) in %s\n", len(newitems), feed.Url)
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
			} else {
				log.Printf("got response to crawl: %v", resp.URI)
				// Parse with Gokogiri
				doc, err := gokogiri.ParseXml(resp.Body)
				if err != nil {
					log.Printf("Error Parsing feed with Gokogiri: %s", err.Error())
					resp.Error = err
				} else {
					charset_handler := charset.NewReader
					feed := rss.New(10, true, nil, nil)
					err = feed.FetchBytes(resp.URI, []byte(doc.String()), charset_handler)
					if err != nil {
						log.Printf("Error parsing feed response from %s: %s", resp.URI,
							err.Error())
						resp.Error = err
					} else {
						resp.Feed = *feed
						for _, channel := range feed.Channels {
							log.Printf("Channel has %d items", len(channel.Items))
						}

						self.filterNewItems(feed)
						resp.LastItemTime = self.last_item_time
						for _, channel := range feed.Channels {
							log.Printf("Channel %s has new %d items", channel.Title, len(channel.Items))
							for _, item := range channel.Items {
								log.Printf("Got Item: %s, sending for mail.", item.Title)
								err = self.sendMail(item)
								if err != nil {
									log.Printf("Error sending mail: %s", err.Error())
								}
							}
						}
						feed.CanUpdate()
						to_sleep = feed.SecondsTillUpdate()
						self.updateDB()
					}
					self.response_chan <- resp
				}
			}
			log.Printf("Sleeping %v seconds for %v", to_sleep, self.URI)
			Sleep(time.Second * time.Duration(to_sleep))
		}
	}
}

func (self *FeedWatcher) updateDB() error {
	return nil
}

func (self *FeedWatcher) filterNewItems(f *rss.Feed) {
	new_items := []*rss.Item{}
	for _, channel := range f.Channels {
		log.Printf("Got channel: %s", channel.Title)
		handle_all := self.last_item_time.IsZero()
		for _, item := range channel.Items {
			d, err := parseDate(item.PubDate)
			if err != nil {
				log.Printf("Couldn't parse %s, skipping", item.PubDate)
				continue
			}
			if handle_all || d.After(self.last_item_time) {
				if d.After(self.last_item_time) {
					log.Printf("Got newer item date (%v) > (%v)", d, self.last_item_time)
					self.last_item_time = d
				}
				new_items = append(new_items, item)
			}
		}
		channel.Items = new_items
	}
}

func (self *FeedWatcher) sendMail(item *rss.Item) error {
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
