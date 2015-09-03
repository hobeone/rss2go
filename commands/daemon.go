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
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/webui"
)

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

	cmd.Flag.String("config_file", default_config, "Config file to use.")
	return cmd
}

type Daemon struct {
	Config    *config.Config
	CrawlChan chan *feedwatcher.FeedCrawlRequest
	RespChan  chan *feedwatcher.FeedCrawlResponse
	MailChan  chan *mail.MailRequest
	Feeds     map[string]*feedwatcher.FeedWatcher
	Dbh       *db.Handle
	PollFeeds bool
}

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
	db_feeds, err := d.Dbh.GetAllFeeds()
	if err != nil {
		logrus.Errorf("Error getting feeds from db: %s\n", err.Error())
		return
	}
	all_feeds := make(map[string]db.FeedInfo)
	for _, fi := range db_feeds {
		all_feeds[fi.URL] = fi
	}
	for k, v := range d.Feeds {
		if _, ok := all_feeds[k]; !ok {
			logrus.Infof("Feed %s removed from db. Stopping poll.\n", k)
			v.StopPoll()
			delete(d.Feeds, k)
		}
	}
	feeds_to_start := make([]db.FeedInfo, 0)
	for k, v := range all_feeds {
		if _, ok := d.Feeds[k]; !ok {
			feeds_to_start = append(feeds_to_start, v)
			logrus.Infof("Feed %s added to db. Adding to queue to start.\n", k)
		}
	}
	if len(feeds_to_start) > 0 {
		logrus.Infof("Adding %d feeds to watch.\n", len(feeds_to_start))
		d.startPollers(feeds_to_start)
	}
}

func (d *Daemon) startPollers(new_feeds []db.FeedInfo) {
	// make feeds unique
	for _, f := range new_feeds {
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

func (d *Daemon) CreateAndStartFeedWatchers(feeds []db.FeedInfo) {
	// start crawler pool
	crawler.StartCrawlerPool(d.Config.Crawl.MaxCrawlers, d.CrawlChan)

	// Start Polling
	d.startPollers(feeds)
}

func runDaemon(cmd *flagutil.Command, args []string) {
	send_mail := cmd.Flag.Lookup("send_mail").Value.(flag.Getter).Get().(bool)
	update_db := cmd.Flag.Lookup("db_updates").Value.(flag.Getter).Get().(bool)
	poll_feeds := cmd.Flag.Lookup("poll_feeds").Value.(flag.Getter).Get().(bool)

	cfg := loadConfig(
		cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	// Override config settings from flags:
	cfg.Mail.SendMail = send_mail
	cfg.Db.UpdateDb = update_db

	d := NewDaemon(cfg)
	d.PollFeeds = poll_feeds

	all_feeds, err := d.Dbh.GetAllFeeds()

	if err != nil {
		logrus.Fatal(err.Error())
	}
	d.CreateAndStartFeedWatchers(all_feeds)

	logrus.Infof("Got %d feeds to watch.\n", len(all_feeds))

	go d.feedDbUpdateLoop()

	go webui.RunWebUi(d.Config, d.Dbh, d.Feeds)
	for {
		_ = <-d.RespChan
	}
}
