package commands

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	feedwatcher "github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/log"
	"github.com/hobeone/rss2go/mail"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"

	"net/http"
	// Enable pprof monitoring
	_ "net/http/pprof"
)

type daemonCommand struct {
	Config      *config.Config
	DBH         *db.Handle
	SendMail    bool
	UpdateDB    bool
	PollFeeds   bool
	ShowMem     bool
	EnablePprof bool
}

func (dc *daemonCommand) init() {
	dc.Config, dc.DBH = commonInit()
}

func (dc *daemonCommand) configure(app *kingpin.Application) {
	daemonCmd := app.Command("daemon", "run rss2go daemon").Action(dc.run)
	daemonCmd.Flag("send_mail", "Send mail on new items").Default("true").BoolVar(&dc.SendMail)
	daemonCmd.Flag("db_updates", "Controls if the database is updated with new items").Default("true").BoolVar(&dc.UpdateDB)
	daemonCmd.Flag("poll_feeds", "Poll the feeds (Disable for testing)").Default("true").BoolVar(&dc.PollFeeds)
	daemonCmd.Flag("show_memory", "Print memory usage ever minute").Default("false").BoolVar(&dc.ShowMem)
	daemonCmd.Flag("pprof", "Enable pprof http server").Default("false").BoolVar(&dc.EnablePprof)
}

func (dc *daemonCommand) run(c *kingpin.ParseContext) error {
	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	if *quiet {
		logrus.SetLevel(logrus.ErrorLevel)
	}
	dc.Config = loadConfig(*configfile)
	dc.Config.Mail.SendMail = dc.SendMail
	dc.Config.DB.UpdateDb = dc.UpdateDB

	d := NewDaemon(dc.Config)
	d.PollFeeds = dc.PollFeeds
	d.ShowMem = dc.ShowMem
	allFeeds, err := d.DBH.GetAllFeedsWithUsers()
	if err != nil {
		logrus.Fatal(err.Error())
	}
	d.CreateAndStartFeedWatchers(allFeeds)

	logrus.Infof("Got %d feeds to watch.", len(allFeeds))

	go d.feedDbUpdateLoop()
	if dc.EnablePprof {
		logrus.Println(http.ListenAndServe("localhost:6060", nil))
	}
	d.pollWG.Wait()
	return nil
}

// Daemon encapsulates all the information about a Daemon instance.
type Daemon struct {
	Config    *config.Config
	CrawlChan chan *feedwatcher.FeedCrawlRequest
	MailChan  chan *mail.Request
	Feeds     map[string]*feedwatcher.FeedWatcher
	DBH       *db.Handle
	PollFeeds bool
	Logger    logrus.FieldLogger
	ShowMem   bool
	pollWG    sync.WaitGroup
}

// NewDaemon returns a pointer to a new Daemon struct with defaults set.
func NewDaemon(cfg *config.Config) *Daemon {
	var dbh *db.Handle
	logger := logrus.New()
	log.SetupLogger(logger)
	if *debugdb {
		logger.Level = logrus.DebugLevel
	}
	if *quiet {
		logger.Level = logrus.ErrorLevel
	}
	if cfg.DB.Type == "memory" {
		dbh = db.NewMemoryDBHandle(logger, false)
	} else {
		dbh = db.NewDBHandle(cfg.DB.Path, logger)
	}
	cc := make(chan *feedwatcher.FeedCrawlRequest, 1)
	mc := mail.CreateAndStartMailer(cfg).OutgoingMail

	return &Daemon{
		Config:    cfg,
		CrawlChan: cc,
		MailChan:  mc,
		DBH:       dbh,
		Feeds:     make(map[string]*feedwatcher.FeedWatcher),
		PollFeeds: true,
		Logger:    logger,
		ShowMem:   false,
		pollWG:    sync.WaitGroup{},
	}
}

// Watch the db config and update feeds based on removal or addition of feeds
func (d *Daemon) feedDbUpdateLoop() {
	ival := time.Duration(d.Config.DB.WatchInterval) * time.Second
	logrus.Infof("Watching the db for changed feeds every %v", ival)
	for {
		time.Sleep(ival)
		d.feedDbUpdate()
		runtime.GC()
		if d.ShowMem {
			PrintMemUsage()
		}
	}
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// PrintMemUsage outputs the current, total and OS memory being used. As well as the number
// of garage collection cycles completed.
func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
	fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
	fmt.Printf("\tHeapAlloc = %v MiB", bToMb(m.HeapAlloc))
	fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
}

func (d *Daemon) feedDbUpdate() {
	dbFeeds, err := d.DBH.GetAllFeedsWithUsers()
	if err != nil {
		logrus.Errorf("Error getting feeds from db: %s", err.Error())
		return
	}
	allFeeds := make(map[string]*db.FeedInfo)
	for _, fi := range dbFeeds {
		allFeeds[fi.URL] = fi
	}
	for k, v := range d.Feeds {
		if _, ok := allFeeds[k]; !ok {
			logrus.Infof("Feed %s removed from db. Stopping poll.", k)
			v.StopPoll()
			delete(d.Feeds, k)
		}
	}
	var feedsToStart []*db.FeedInfo
	for k, v := range allFeeds {
		if _, ok := d.Feeds[k]; !ok {
			feedsToStart = append(feedsToStart, v)
			logrus.Infof("Feed %s added to db. Adding to queue to start.", k)
		}
	}
	if len(feedsToStart) > 0 {
		logrus.Infof("Adding %d feeds to watch.", len(feedsToStart))
		d.startPollers(feedsToStart)
	}
}

func (d *Daemon) startPollers(newFeeds []*db.FeedInfo) {
	// make feeds unique
	for _, f := range newFeeds {
		if _, ok := d.Feeds[f.URL]; ok {
			logrus.Infof("Found duplicate feed: %s", f.URL)
			continue
		}

		d.Feeds[f.URL] = feedwatcher.NewFeedWatcher(
			*f,
			d.CrawlChan,
			d.MailChan,
			d.DBH,
			[]string{},
			d.Config.Crawl.MinInterval,
			d.Config.Crawl.MaxInterval,
		)
		if d.PollFeeds {
			d.pollWG.Add(1)
			go d.Feeds[f.URL].PollFeed()
		}
	}
}

// CreateAndStartFeedWatchers does exactly what it says
func (d *Daemon) CreateAndStartFeedWatchers(feeds []*db.FeedInfo) {
	// start crawler pool
	crawler.StartCrawlerPool(d.Config.Crawl.MaxCrawlers, d.CrawlChan)

	// Start Polling
	d.startPollers(feeds)
}
