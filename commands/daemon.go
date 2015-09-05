package commands

import (
	"flag"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/flagutil"
	//set log defaults
	_ "github.com/hobeone/rss2go/log"
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/webui"
)

// MakeCmdDaemon returns a command struct ready to be called.
func MakeCmdDaemon() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       runDaemon,
		UsageLine: "daemon",
		Short:     "Start a daemon to collect feeds and mail items.",
		Long: `
		Starts up as a daemon and will watch feeds and send new items to the configured
		mail address.
		`,
		Flag: *flag.NewFlagSet("daemon", flag.ExitOnError),
	}
	cmd.Flag.Bool("send_mail", true, "Actually send mail or not.")
	cmd.Flag.Bool("db_updates", true, "Don't actually update feed info in the db.")
	cmd.Flag.Bool("poll_feeds", true, "Poll the feeds (Disable for testing).")
	cmd.Flag.Bool("verbose", false, "Log debug information.")

	cmd.Flag.String("config_file", default_config, "Config file to use.")
	return cmd
}

// Daemon encapsulates all the information about a Daemon instance.
type Daemon struct {
	Config    *config.Config
	CrawlChan chan *feedwatcher.FeedCrawlRequest
	RespChan  chan *feedwatcher.FeedCrawlResponse
	MailChan  chan *mail.Request
	Feeds     map[string]*feedwatcher.FeedWatcher
	Dbh       *db.Handle
	PollFeeds bool
}

// NewDaemon returns a pointer to a new Daemon struct with defaults set.
func NewDaemon(cfg *config.Config) *Daemon {
	var dbh *db.Handle
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDBHandle(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	}
	cc := make(chan *feedwatcher.FeedCrawlRequest, 1)
	rc := make(chan *feedwatcher.FeedCrawlResponse)
	mc := mail.CreateAndStartMailer(cfg).OutgoingMail

	return &Daemon{
		Config:    cfg,
		CrawlChan: cc,
		RespChan:  rc,
		MailChan:  mc,
		Dbh:       dbh,
		Feeds:     make(map[string]*feedwatcher.FeedWatcher),
		PollFeeds: true,
	}
}

// Watch the db config and update feeds based on removal or addition of feeds
func (d *Daemon) feedDbUpdateLoop() {
	ival := time.Duration(d.Config.Db.WatchInterval) * time.Second
	logrus.Errorf("Watching the db for changed feeds every %v\n", ival)
	for {
		time.Sleep(ival)
		d.feedDbUpdate()
	}
}

func (d *Daemon) feedDbUpdate() {
	dbFeeds, err := d.Dbh.GetAllFeeds()
	if err != nil {
		logrus.Errorf("Error getting feeds from db: %s\n", err.Error())
		return
	}
	allFeeds := make(map[string]db.FeedInfo)
	for _, fi := range dbFeeds {
		allFeeds[fi.URL] = fi
	}
	for k, v := range d.Feeds {
		if _, ok := allFeeds[k]; !ok {
			logrus.Infof("Feed %s removed from db. Stopping poll.\n", k)
			v.StopPoll()
			delete(d.Feeds, k)
		}
	}
	var feedsToStart []db.FeedInfo
	for k, v := range allFeeds {
		if _, ok := d.Feeds[k]; !ok {
			feedsToStart = append(feedsToStart, v)
			logrus.Infof("Feed %s added to db. Adding to queue to start.\n", k)
		}
	}
	if len(feedsToStart) > 0 {
		logrus.Infof("Adding %d feeds to watch.\n", len(feedsToStart))
		d.startPollers(feedsToStart)
	}
}

func (d *Daemon) startPollers(newFeeds []db.FeedInfo) {
	// make feeds unique
	for _, f := range newFeeds {
		if _, ok := d.Feeds[f.URL]; ok {
			logrus.Infof("Found duplicate feed: %s", f.URL)
			continue
		}

		d.Feeds[f.URL] = feedwatcher.NewFeedWatcher(
			f,
			d.CrawlChan,
			d.RespChan,
			d.MailChan,
			d.Dbh,
			[]string{},
			d.Config.Crawl.MinInterval,
			d.Config.Crawl.MaxInterval,
		)
		if d.PollFeeds {
			go d.Feeds[f.URL].PollFeed()
		}
	}
}

// CreateAndStartFeedWatchers does exactly what it says
func (d *Daemon) CreateAndStartFeedWatchers(feeds []db.FeedInfo) {
	// start crawler pool
	crawler.StartCrawlerPool(d.Config.Crawl.MaxCrawlers, d.CrawlChan)

	// Start Polling
	d.startPollers(feeds)
}

func runDaemon(cmd *flagutil.Command, args []string) {
	sendMail := cmd.Flag.Lookup("send_mail").Value.(flag.Getter).Get().(bool)
	updateDB := cmd.Flag.Lookup("db_updates").Value.(flag.Getter).Get().(bool)
	pollFeeds := cmd.Flag.Lookup("poll_feeds").Value.(flag.Getter).Get().(bool)
	logVerbose := cmd.Flag.Lookup("verbose").Value.(flag.Getter).Get().(bool)

	if logVerbose {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.WarnLevel)
	}

	cfg := loadConfig(
		cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	// Override config settings from flags:
	cfg.Mail.SendMail = sendMail
	cfg.Db.UpdateDb = updateDB

	d := NewDaemon(cfg)
	d.PollFeeds = pollFeeds

	allFeeds, err := d.Dbh.GetAllFeeds()

	if err != nil {
		logrus.Fatal(err.Error())
	}
	d.CreateAndStartFeedWatchers(allFeeds)

	logrus.Infof("Got %d feeds to watch.\n", len(allFeeds))

	go d.feedDbUpdateLoop()

	go webui.RunWebUi(d.Config, d.Dbh, d.Feeds)
	for {
		_ = <-d.RespChan
	}
}
