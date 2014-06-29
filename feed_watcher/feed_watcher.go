//FeedWatcher watches a feed.  Duh.
//
//One feed watcher watches one feed.  It doesn't actually do the crawling itself
//but will send a FeedCrawlRequest to a crawler.
//
//To watch a feed create a FeedWatcher instance and then call PollFeed() on it
//(usually in a goroutine).
//
//FeedWatcher will keep track of the GUIDs it has seen from a feed so it will
//only notify on new items.
//
//See bin/runone.go or bin/daemon.go for example usage.

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

// FeedCrawlRequest is how to request a URL to be crawled by a crawler instance.
type FeedCrawlRequest struct {
	URI          string
	ResponseChan chan *FeedCrawlResponse
}

// FeedCrawlResponse is we get back from the crawler.
//
// Body, Feed, Items and HTTPResponseStatus are only guaranteed to be non zero
// value if Error is non nil.
//
type FeedCrawlResponse struct {
	URI                string
	Body               []byte
	Feed               *feed.Feed
	Items              []*feed.Story
	HTTPResponseStatus string
	Error              error
}

// FeedWatcher controlls the crawling of a Feed.  It keeps state about the
// GUIDs it's seen, sends crawl requests and deals with responses.
type FeedWatcher struct {
	FeedInfo          db.FeedInfo
	exitChan          chan int
	crawlChan         chan *FeedCrawlRequest
	responseChan      chan *FeedCrawlResponse
	mailerChan        chan *mail.MailRequest
	polling           bool // make sure only one PollFeed at a time
	crawling          bool // make sure only one crawl outstanding at a time
	minSleepTime      time.Duration
	maxSleepTime      time.Duration
	dbh               *db.DBHandle
	KnownGuids        map[string]bool
	LastCrawlResponse *FeedCrawlResponse
}

// NewFeedWatcher returns a new FeedWatcher instance.
func NewFeedWatcher(
	feedInfo db.FeedInfo,
	crawlChan chan *FeedCrawlRequest,
	responseChan chan *FeedCrawlResponse,
	mailChan chan *mail.MailRequest,
	dbh *db.DBHandle,
	knownGUIDs []string,
	minSleep int64,
	maxSleep int64,
) *FeedWatcher {
	guids := map[string]bool{}
	for _, i := range knownGUIDs {
		guids[i] = true
	}
	return &FeedWatcher{
		FeedInfo:          feedInfo,
		exitChan:          make(chan int),
		crawlChan:         crawlChan,
		responseChan:      responseChan,
		mailerChan:        mailChan,
		polling:           false,
		crawling:          false,
		minSleepTime:      time.Duration(minSleep) * time.Second,
		maxSleepTime:      time.Duration(maxSleep) * time.Second,
		dbh:               dbh,
		KnownGuids:        guids,
		LastCrawlResponse: &FeedCrawlResponse{},
	}
}

func (fw *FeedWatcher) lockPoll() bool {
	if fw.polling {
		return false
	}
	fw.polling = true
	return true
}

func (fw *FeedWatcher) unlockPoll() bool {
	fw.polling = false
	return true
}

// Polling returns true if the FeedWatcher is Polling a feed.
func (fw *FeedWatcher) Polling() bool {
	return fw.polling
}

func (fw *FeedWatcher) lockCrawl() bool {
	if fw.crawling {
		return false
	}
	fw.crawling = true
	return true
}

func (fw *FeedWatcher) unlockCrawl() (r bool) {
	fw.crawling = false
	return true
}

// Crawling returns true if the FeedWatcher is crawling a feed (actually
// waiting for a response from the crawler).
func (fw *FeedWatcher) Crawling() (r bool) {
	return fw.crawling
}

// UpdateFeed will crawl the feed, check for new items, mail them out and
// update the database with the new information
func (fw *FeedWatcher) UpdateFeed() *FeedCrawlResponse {
	resp := fw.updateFeed()
	// Update DB Record
	fw.FeedInfo.LastPollTime = time.Now()
	fw.FeedInfo.LastPollError = ""
	if resp.Error != nil {
		fw.FeedInfo.LastPollError = resp.Error.Error()
	}
	err := fw.dbh.SaveFeed(&fw.FeedInfo)
	if err != nil {
		resp.Error = err
	}
	return resp
}

// Core logic to poll a feed, find new items, add those to the database, and
// send them for mail.
func (fw *FeedWatcher) updateFeed() *FeedCrawlResponse {
	glog.Infof("Polling feed %v", fw.FeedInfo.Url)
	resp := fw.doCrawl()

	if resp.Error != nil {
		glog.Infof("Error getting feed %v: %v", fw.FeedInfo.Url, resp.Error)
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
			e := fmt.Errorf("no items found in %s", resp.URI)
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

	guidsToLoad := int(math.Ceil(float64(len(stories)) * 1.1))

	glog.Infof("Got %d stories from feed %s.", len(stories), fw.FeedInfo.Url)

	if len(fw.KnownGuids) == 0 {
		var err error
		fw.KnownGuids, err = fw.LoadGuidsFromDb(guidsToLoad)
		if err != nil {
			e := fmt.Errorf("error getting Guids from DB: %s", err)
			glog.Info(e)
			resp.Error = e
			return resp
		}
		glog.Infof("Loaded %d known guids for Feed %s.",
			len(fw.KnownGuids), fw.FeedInfo.Url)
	}

	resp.Items = fw.filterNewItems(stories)
	glog.Infof("Feed %s has %d new items", feed.Title, len(resp.Items))

	for _, item := range resp.Items {
		item.Title = fmt.Sprintf("%s: %s", fw.FeedInfo.Name, item.Title)
		glog.Infof("New Story: %s, sending for mail.", item.Title)
		err := fw.sendMail(item)
		if err != nil {
			glog.Infof("Error sending mail: %s", err.Error())
		} else {
			err := fw.recordGuid(item.Id)
			if err != nil {
				e := fmt.Errorf("error writing guid to db: %s", err)
				resp.Error = e
				glog.Info(e)
			} else {
				glog.Infof("Added guid %s for feed %s", item.Id, fw.FeedInfo.Url)
			}
		}
	}

	// Prune guids we know about back down to X * 1.1 but only if there were new
	// items otherwise it would be noop.
	if len(resp.Items) > 0 {
		fw.KnownGuids, err = fw.LoadGuidsFromDb(guidsToLoad)
		if err != nil {
			e := fmt.Errorf("error getting Guids from DB: %s", err)
			glog.Info(e)
			resp.Error = e
		}
	}

	return resp
}

// PollFeed is designed to be called as a Goroutine.  Use UpdateFeed to just update once.
func (fw *FeedWatcher) PollFeed() bool {
	if !fw.lockPoll() {
		glog.Infof("Called PollLoop on %v when already polling. ignoring.\n",
			fw.FeedInfo.Url)
		return false
	}
	defer fw.unlockPoll()

	timeSinceLastPoll := time.Since(fw.FeedInfo.LastPollTime)
	toSleep := time.Duration(0)
	if timeSinceLastPoll < fw.minSleepTime {
		toSleep = fw.minSleepTime - timeSinceLastPoll
		glog.Infof("Last poll of %s was only %v ago, waiting at least %v.",
			fw.FeedInfo.Url, timeSinceLastPoll, toSleep)
	}
	for {
		select {
		case <-fw.exitChan:
			glog.Infof("Got exit signal, stopping poll of %v", fw.FeedInfo.Url)
			return true
		case <-fw.afterWithJitter(toSleep):
			fw.LastCrawlResponse = fw.UpdateFeed()
			fw.responseChan <- fw.LastCrawlResponse
			toSleep = fw.minSleepTime
			if fw.LastCrawlResponse.Error != nil {
				toSleep = fw.maxSleepTime
				glog.Infof("Feed %s had error. Waiting maxium allowed time before next poll.",
					fw.FeedInfo.Url)
			}
		}
	}
}

// call time.After for the given amount of seconds plus a up to 60 extra seconds.
func (fw *FeedWatcher) afterWithJitter(d time.Duration) <-chan time.Time {
	s := d + time.Duration(rand.Int63n(60))*time.Second
	glog.Infof("Waiting %v until next poll of %s", s, fw.FeedInfo.Url)
	return After(s)
}

// LoadGuidsFromDb populates the internal cache of GUIDs by getting the most
// recent GUIDs it knows about.  To retrieve all GUIDs set max to -1.
func (fw *FeedWatcher) LoadGuidsFromDb(max int) (map[string]bool, error) {
	guids, err := fw.dbh.GetMostRecentGuidsForFeed(fw.FeedInfo.Id, max)
	if err != nil {
		return nil, err
	}
	ret := make(map[string]bool)
	for _, v := range guids {
		ret[v] = true
	}
	return ret, nil
}

func (fw *FeedWatcher) recordGuid(guid string) error {
	fw.KnownGuids[guid] = true
	return fw.dbh.RecordGuid(fw.FeedInfo.Id, guid)
}

func (fw *FeedWatcher) filterNewItems(stories []*feed.Story) []*feed.Story {
	glog.Infof("Filtering stories we already know about.")
	newStories := []*feed.Story{}
	for _, story := range stories {
		if _, found := fw.KnownGuids[story.Id]; !found {
			newStories = append(newStories, story)
		}
	}
	return newStories
}

func (fw *FeedWatcher) sendMail(item *feed.Story) error {
	_, err := fw.dbh.GetFeedItemByGuid(fw.FeedInfo.Id, item.Id)
	// Guid found, so sending would be a duplicate.
	if err == nil {
		glog.Warningf("Tried to send duplicate GUID: %s for feed %s",
			item.Id, fw.FeedInfo.Url)
		return nil
	}

	req := &mail.MailRequest{
		Item:       item,
		ResultChan: make(chan error),
	}
	fw.mailerChan <- req
	resp := <-req.ResultChan
	return resp
}

func (fw *FeedWatcher) doCrawl() (r *FeedCrawlResponse) {
	if fw.crawling {
		return &FeedCrawlResponse{
			URI:   fw.FeedInfo.Url,
			Error: errors.New("Already crawling " + fw.FeedInfo.Url),
		}
	}
	fw.lockCrawl()
	defer fw.unlockCrawl()

	req := &FeedCrawlRequest{
		URI:          fw.FeedInfo.Url,
		ResponseChan: make(chan *FeedCrawlResponse),
	}
	fw.crawlChan <- req
	resp := <-req.ResponseChan
	return resp
}

// StopPoll will cause the PollFeed loop to exit.
func (fw *FeedWatcher) StopPoll() {
	if fw.Polling() {
		fw.exitChan <- 1
	}
}
