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

package feedwatcher

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	netmail "net/mail"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/mail"
)

const guidSaveRatio = 1.5

// ErrCrawlerNotAvailable is when the crawler channel would block
var ErrCrawlerNotAvailable = errors.New("no crawler available")

// ErrAlreadyCrawlingFeed is when you are already Crawling a feed
var ErrAlreadyCrawlingFeed = errors.New("already crawling feed")

// After allows for stubbing out in test
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
	URI                    string
	Body                   []byte
	Feed                   *feed.Feed
	Items                  []*feed.Story
	HTTPResponseStatus     string
	HTTPResponseStatusCode int
	Error                  error
}

// FeedWatcher controlls the crawling of a Feed.  It keeps state about the
// GUIDs it's seen, sends crawl requests and deals with responses.
type FeedWatcher struct {
	FeedInfo          db.FeedInfo
	exitChan          chan int
	crawlChan         chan *FeedCrawlRequest
	responseChan      chan *FeedCrawlResponse
	mailerChan        chan *mail.Request
	pollWG            sync.WaitGroup
	polling           bool // make sure only one PollFeed at a time
	crawling          bool // make sure only one crawl outstanding at a time
	minSleepTime      time.Duration
	maxSleepTime      time.Duration
	dbh               db.Service
	KnownGuids        map[string]bool
	LastCrawlResponse *FeedCrawlResponse
	After             func(d time.Duration) <-chan time.Time // Allow for mocking out in test.
	Logger            logrus.FieldLogger
	PruneGUIDS        bool
}

// NewFeedWatcher returns a new FeedWatcher instance.
func NewFeedWatcher(
	feedInfo db.FeedInfo,
	crawlChan chan *FeedCrawlRequest,
	responseChan chan *FeedCrawlResponse,
	mailChan chan *mail.Request,
	dbh db.Service,
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
		After:             After,
		Logger:            logrus.StandardLogger(),
		PruneGUIDS:        true,
	}
}

func (fw *FeedWatcher) lockPoll() bool {
	if fw.polling {
		return false
	}
	fw.pollWG.Add(1)
	fw.polling = true
	return true
}

func (fw *FeedWatcher) unlockPoll() bool {
	fw.polling = false
	fw.pollWG.Done()
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
func (fw *FeedWatcher) UpdateFeed(resp *FeedCrawlResponse) error {
	fw.FeedInfo.LastPollTime = time.Now()
	fw.FeedInfo.LastPollError = ""
	fw.FeedInfo.LastErrorResponse = ""

	if resp.HTTPResponseStatusCode != http.StatusOK || resp.Error != nil {
		if resp.HTTPResponseStatusCode != http.StatusOK {
			fw.FeedInfo.LastPollError = fmt.Sprintf("Non 200 HTTP Status Code: %d", resp.HTTPResponseStatusCode)
		}
		bod, err := ioutil.ReadAll(bytes.NewReader(resp.Body))
		if err != nil {
			fw.FeedInfo.LastErrorResponse = fmt.Sprintf("rss2go: Couldn't read response: %v", err)
		} else {
			fw.FeedInfo.LastErrorResponse = string(bod)
		}
	} else {
		fw.updateFeed(resp)

		// Update DB Record
		if resp.Error != nil {
			fw.FeedInfo.LastPollError = resp.Error.Error()
			bod, err := ioutil.ReadAll(bytes.NewReader(resp.Body))
			if err != nil {
				fw.FeedInfo.LastErrorResponse = fmt.Sprintf("rss2go: Couldn't read response: %v", err)
			} else {
				fw.FeedInfo.LastErrorResponse = string(bod)
			}
		} else {
			fw.FeedInfo.SiteURL = resp.Feed.Link
		}
	}

	err := fw.dbh.SaveFeed(&fw.FeedInfo)
	if err != nil {
		resp.Error = err
	}
	guids := make([]string, 0, len(fw.KnownGuids))
	for k := range fw.KnownGuids {
		guids = append(guids, k)
	}
	if fw.PruneGUIDS {
		_, err = fw.dbh.SetFeedGUIDS(fw.FeedInfo.ID, guids)
		if err != nil {
			resp.Error = err
		}
	}

	if fw.FeedInfo.LastPollError != "" && resp.Error == nil {
		resp.Error = errors.New(fw.FeedInfo.LastPollError)
	}
	return resp.Error
}

// Core logic to poll a feed, find new items, add those to the database, and
// send them for mail.
//
// Populates fields:
// - Feed with information extracted from the feed
// - Items with the items in the feed
// - Error any errors encountered in parsing or handling the feed
func (fw *FeedWatcher) updateFeed(resp *FeedCrawlResponse) error {
	feed, stories, err := feed.ParseFeed(resp.URI, resp.Body)

	if feed == nil || stories == nil || len(stories) == 0 {
		if err != nil {
			resp.Error = fmt.Errorf("Error parsing response from %s: %#v", resp.URI, err)
			fw.Logger.Error(resp.Error)
		} else {
			resp.Error = fmt.Errorf("no items found in %s", resp.URI)
			fw.Logger.Info(resp.Error)
		}
		return resp.Error
	}

	resp.Feed = feed
	/*
	 * Load most recent X * guidSaveRatio Guids from Db
	 * Filter New Stories
	 * Send New Stories for mailing
	 *  Add sent guid to list
	 * prune Guids back to X * guidSaveRatio
	 */

	// If we don't know about any GUIDs for this feed we haven't been
	// initalized yet.  Check the GUIDs in the feed we just read and see if
	// they exist in the DB.  That should be the maximum we should need to
	// load into memory.

	guidsToLoad := int(math.Ceil(float64(len(stories)) * guidSaveRatio))

	fw.Logger.Infof("Got %d stories from feed %s.", len(stories), fw.FeedInfo.URL)

	if len(fw.KnownGuids) == 0 {
		var err error
		fw.KnownGuids, err = fw.LoadGuidsFromDb(guidsToLoad)
		if err != nil {
			resp.Error = fmt.Errorf("error getting Guids from DB: %s", err)
			return resp.Error
		}
		fw.Logger.Infof("Loaded %d known guids for Feed %s.", len(fw.KnownGuids), fw.FeedInfo.URL)
	}

	resp.Items = fw.filterNewItems(stories)
	fw.Logger.Infof("Feed %s has %d new items", feed.Title, len(resp.Items))

	for _, item := range resp.Items {
		item.Title = fmt.Sprintf("%s: %s", fw.FeedInfo.Name, item.Title)
		fw.Logger.Infof("New Story: %s, sending for mail.", item.Title)
		err := fw.sendMail(item)
		if err != nil {
			fw.Logger.Infof("Error sending mail: %s", err.Error())
			resp.Error = err
		} else {
			err := fw.recordGUID(item.ID)
			if err != nil {
				e := fmt.Errorf("error writing guid to db: %s", err)
				resp.Error = e
				fw.Logger.Info(e)
			} else {
				fw.Logger.Infof("Added guid %s for feed %s", item.ID, fw.FeedInfo.URL)
			}
		}
	}

	// Reload GUIDs to X * guidSaveRatio but only if there were new
	// items otherwise it would be noop.
	if len(resp.Items) > 0 {
		fw.KnownGuids, err = fw.LoadGuidsFromDb(guidsToLoad)
		if err != nil {
			e := fmt.Errorf("error getting Guids from DB: %s", err)
			fw.Logger.Info(e)
			resp.Error = e
		}
	}
	return resp.Error
}

// PollFeed is designed to be called as a Goroutine.  Use UpdateFeed to just update once.
func (fw *FeedWatcher) PollFeed() bool {
	if !fw.lockPoll() {
		fw.Logger.Infof("Called PollLoop on %v when already polling. ignoring.\n",
			fw.FeedInfo.URL)
		return false
	}
	defer fw.unlockPoll()

	timeSinceLastPoll := time.Since(fw.FeedInfo.LastPollTime)
	toSleep := time.Duration(0)
	if timeSinceLastPoll < fw.minSleepTime {
		toSleep = fw.minSleepTime - timeSinceLastPoll
		fw.Logger.Infof("Last poll of %s was only %v ago, waiting at least %v.",
			fw.FeedInfo.URL, timeSinceLastPoll, toSleep)
	}
	for {
		// To have exit signals handled properly non of the other cases can block
		//
		select {
		case <-fw.exitChan:
			fw.Logger.Infof("Got exit signal, stopping poll of %v", fw.FeedInfo.URL)
			return true
		case <-fw.afterWithJitter(toSleep):
			// see if we can crawl
			// if not sleep some small amount of time to try again - assumes that all crawlers are busy
			resp := fw.CrawlFeed()
			if resp.Error == ErrCrawlerNotAvailable {
				toSleep = fw.minSleepTime
				fw.Logger.Infof("No crawler available, sleeping.")
				break
			}
			fw.LastCrawlResponse = resp
			err := fw.UpdateFeed(resp)
			if err != nil {
				fw.Logger.Errorf("Error updating feed information: %s", err)
			}
			fw.responseChan <- fw.LastCrawlResponse
			toSleep = fw.minSleepTime
			if fw.LastCrawlResponse.Error != nil {
				toSleep = fw.maxSleepTime
				fw.Logger.Infof("Feed %s had error. Waiting maxium allowed time before next poll.",
					fw.FeedInfo.URL)
			}
		}
	}
}

// call time.After for the given amount of seconds plus a up to 60 extra seconds.
func (fw *FeedWatcher) afterWithJitter(d time.Duration) <-chan time.Time {
	s := d + time.Duration(rand.Int63n(60))*time.Second
	fw.Logger.Infof("Waiting %v until next poll of %s", s, fw.FeedInfo.URL)
	return fw.After(s)
}

// LoadGuidsFromDb populates the internal cache of GUIDs by getting the most
// recent GUIDs it knows about.  To retrieve all GUIDs set max to -1.
func (fw *FeedWatcher) LoadGuidsFromDb(max int) (map[string]bool, error) {
	guids, err := fw.dbh.GetMostRecentGUIDsForFeed(fw.FeedInfo.ID, max)
	if err != nil {
		return nil, err
	}
	ret := make(map[string]bool)
	for _, v := range guids {
		ret[v] = true
	}
	return ret, nil
}

func (fw *FeedWatcher) recordGUID(guid string) error {
	fw.KnownGuids[guid] = true
	return fw.dbh.RecordGUID(fw.FeedInfo.ID, guid)
}

func (fw *FeedWatcher) filterNewItems(stories []*feed.Story) []*feed.Story {
	fw.Logger.Infof("Filtering stories we already know about.")
	newStories := []*feed.Story{}
	for _, story := range stories {
		if _, found := fw.KnownGuids[story.ID]; !found {
			newStories = append(newStories, story)
		}
	}
	return newStories
}

func (fw *FeedWatcher) sendMail(item *feed.Story) error {
	_, err := fw.dbh.GetFeedItemByGUID(fw.FeedInfo.ID, item.ID)
	// Guid found, so sending would be a duplicate.
	if err == nil {
		fw.Logger.Warningf("Tried to send duplicate GUID: %s for feed %s",
			item.ID, fw.FeedInfo.URL)
		return nil
	}

	users, err := fw.dbh.GetFeedUsers(fw.FeedInfo.URL)
	if err != nil {
		return err
	}

	sendTo := make([]netmail.Address, len(users))
	for i, u := range users {
		sendTo[i] = netmail.Address{Address: u.Email}
	}
	req := &mail.Request{
		Item:       item,
		Addresses:  sendTo,
		ResultChan: make(chan error),
	}
	fw.mailerChan <- req
	resp := <-req.ResultChan
	return resp
}

// CrawlFeed will send a request to crawl a feed over it's crawl channel
func (fw *FeedWatcher) CrawlFeed() (r *FeedCrawlResponse) {
	if fw.crawling {
		return &FeedCrawlResponse{
			URI:   fw.FeedInfo.URL,
			Error: ErrAlreadyCrawlingFeed,
		}
	}
	fw.lockCrawl()
	defer fw.unlockCrawl()

	req := &FeedCrawlRequest{
		URI:          fw.FeedInfo.URL,
		ResponseChan: make(chan *FeedCrawlResponse),
	}
	resp := &FeedCrawlResponse{}
	for {
		select {
		case fw.crawlChan <- req:
			fw.Logger.Infof("Requesting crawl of feed %v", fw.FeedInfo.URL)
			resp = <-req.ResponseChan
			fw.Logger.Infof("Got response to crawl of %v of length (%d)", resp.URI, len(resp.Body))
			return resp
		default:
			resp.Error = ErrCrawlerNotAvailable
			return resp
		}
	}
}

// StopPoll will cause the PollFeed loop to exit.
func (fw *FeedWatcher) StopPoll() {
	if fw.Polling() {
		close(fw.exitChan)
	}
	fw.pollWG.Wait()
}
