package commands

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	feedwatcher "github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"github.com/mmcdole/gofeed"

	"net/http"
	netmail "net/mail"

	// Enable pprof monitoring
	_ "expvar"
)

type daemonCommand struct {
	Config     *config.Config
	DBH        *db.Handle
	SendMail   bool
	UpdateDB   bool
	PollFeeds  bool
	ShowMem    bool
	ExpvarPort int32
}

func (dc *daemonCommand) init() {
	dc.Config, dc.DBH = commonInit()
}

func (dc *daemonCommand) configure(app *kingpin.Application) {
	daemonCmd := app.Command("daemon", "run rss2go daemon").Action(dc.run)
	daemonCmd.Flag("send_mail", "Send mail on new items").Default("true").BoolVar(&dc.SendMail)
	daemonCmd.Flag("db_updates", "Controls if the database is updated with new items").Default("true").BoolVar(&dc.UpdateDB)
	daemonCmd.Flag("poll_feeds", "Poll the feeds (Disable for testing)").Default("true").BoolVar(&dc.PollFeeds)
	daemonCmd.Flag("expvarport", "Port to export Expvar metrics on").Default("0").Int32Var(&dc.ExpvarPort)
}

func (dc *daemonCommand) run(c *kingpin.ParseContext) error {
	dc.Config = loadConfig(*configfile)
	dc.Config.Mail.SendMail = dc.SendMail
	dc.Config.DB.UpdateDb = dc.UpdateDB

	d := NewDaemon(dc.Config)
	d.PollFeeds = dc.PollFeeds
	allFeeds, err := d.DBH.GetAllFeedsWithUsers()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	d.CreateAndStartFeedWatchers(allFeeds)

	slog.Info("Got feeds to watch", "count", len(allFeeds))

	go d.feedDbUpdateLoop()

	// Start Error reporting
	go d.feedStateSummaryLoop()

	if dc.ExpvarPort > 0 {
		host_and_port := fmt.Sprintf("localhost:%d", dc.ExpvarPort)
		slog.Info("Listening on", "address", host_and_port)
		err = http.ListenAndServe(host_and_port, nil)
		if err != nil {
			slog.Error("Error starting expvar server", "error", err)
			os.Exit(1)
		}
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
	Logger    *slog.Logger
	pollWG    sync.WaitGroup
}

// NewDaemon returns a pointer to a new Daemon struct with defaults set.
func NewDaemon(cfg *config.Config) *Daemon {
	var dbh *db.Handle
	logger := slog.Default()

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
		pollWG:    sync.WaitGroup{},
	}
}

// Watch the db config and update feeds based on removal or addition of feeds
func (d *Daemon) feedDbUpdateLoop() {
	ival := time.Duration(d.Config.DB.WatchInterval) * time.Second
	slog.Info("Watching the db for changed feeds", "interval", ival)
	for {
		time.Sleep(ival)
		d.feedDbUpdate()
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
		slog.Error("Error getting feeds from db", "error", err)
		return
	}
	allFeeds := make(map[string]*db.FeedInfo)
	for _, fi := range dbFeeds {
		allFeeds[fi.URL] = fi
	}
	for k, v := range d.Feeds {
		if _, ok := allFeeds[k]; !ok {
			slog.Info("Feed removed from db. Stopping poll.", "url", k)
			v.StopPoll()
			delete(d.Feeds, k)
		}
	}
	var feedsToStart []*db.FeedInfo
	for k, v := range allFeeds {
		if _, ok := d.Feeds[k]; !ok {
			feedsToStart = append(feedsToStart, v)
			slog.Info("Feed added to db. Adding to queue to start.", "url", k)
		}
	}
	if len(feedsToStart) > 0 {
		slog.Info("Adding feeds to watch.", "count", len(feedsToStart))
		d.startPollers(feedsToStart)
	}
}

// Watch the db config and update
func (d *Daemon) feedStateSummaryLoop() {
	ival := time.Duration(d.Config.ReportInterval) * time.Second
	slog.Info("Checking to send reports", "interval", ival)
	for {
		d.feedStateSummary()
		time.Sleep(ival)
	}
}

func (d *Daemon) feedStateSummary() {
	users, err := d.DBH.GetAllUsers()
	if err != nil {
		slog.Error("Error getting users from DB", "error", err)
		return
	}

	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	for _, u := range users {
		lastReport, err := d.DBH.GetUserReport(&u)
		if err != nil {
			slog.Error("Error getting the last time we sent a report to user", "email", u.Email, "error", err)
			return
		}

		if lastReport.LastReport.Before(sevenDaysAgo) {
			slog.Info("Creating Error report", "email", u.Email)
			var sb strings.Builder
			badfeeds := 0
			sb.WriteString(fmt.Sprintf("Failed/Stale Feed report for %s<br>\n", u.Email))
			feeds, err := d.DBH.GetUserFeedsWithErrors(&u)
			if err != nil {
				slog.Error("Error getting users failed feeds", "error", err)
				return
			}
			badfeeds = badfeeds + len(feeds)
			sb.WriteString(fmt.Sprintln("Feeds With Errors: <br>"))
			for _, f := range feeds {
				sb.WriteString("<div>")
				sb.WriteString(fmt.Sprintf("Name: %s<br>\n", f.Name))

				sb.WriteString("<ul>")
				sb.WriteString(fmt.Sprintf("<li>  Url: %s<br>\n", f.URL))
				sb.WriteString(fmt.Sprintf("<li>  Last Update: %s<br>\n", f.LastPollTime))
				sb.WriteString(fmt.Sprintf("<li>  Error: %s<br>\n", strings.TrimSpace(f.LastPollError)))
				sb.WriteString("</ul>")
				sb.WriteString("<br>")
				sb.WriteString("</div>")
			}

			feeds, err = d.DBH.GetUserStaleFeeds(&u)
			if err != nil {
				slog.Error("Error getting stale feeds from DB", "error", err)
				return
			}
			badfeeds = badfeeds + len(feeds)

			sb.WriteString(fmt.Sprintln("<hr />Feeds With No Updates for 2 Weeks: <br>"))
			for _, f := range feeds {
				sb.WriteString("<div>")
				sb.WriteString("<ul>")
				sb.WriteString(fmt.Sprintf("<li>  Name: %s<br>\n", f.Name))
				sb.WriteString(fmt.Sprintf("<li>  Url: %s<br>\n", f.URL))
				sb.WriteString(fmt.Sprintf("<li>  Last Update: %s<br>\n", f.LastPollTime))
				sb.WriteString("</ul>")
				sb.WriteString("<br>")
				sb.WriteString("</div>")
			}
			if badfeeds > 0 {
				item := gofeed.Item{}
				item.Author = &gofeed.Person{
					Name: "ress2go",
				}
				item.Title = "rss2go Failed/Stale Feed report"
				item.Content = sb.String()

				addr := netmail.Address{
					Address: u.Email,
				}
				sendTo := []netmail.Address{addr}
				req := &mail.Request{
					Item:       &item,
					Addresses:  sendTo,
					ResultChan: make(chan error),
				}
				d.MailChan <- req
				resp := <-req.ResultChan
				if resp != nil {
					slog.Error("Error sending mail", "error", resp)
					return
				}
				slog.Info("Sent error report", "to", u.Email)
			} else {
				slog.Info("No bad feeds found", "for", u.Email)
			}
			err = d.DBH.SetUserReport(&u)
			if err != nil {
				slog.Error("Error recording I sent an error report", "to", u.Email, "error", err)
			}

		}
	}
}

func (d *Daemon) startPollers(newFeeds []*db.FeedInfo) {
	// make feeds unique
	for _, f := range newFeeds {
		if _, ok := d.Feeds[f.URL]; ok {
			slog.Info("Found duplicate feed", "url", f.URL)
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
