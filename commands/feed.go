package commands

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"net/http/httputil"

	"github.com/davecgh/go-spew/spew"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"

	"gopkg.in/alecthomas/kingpin.v2"

	netmail "net/mail"
)

type feedCommand struct {
	Config     *config.Config
	DBH        *db.Handle
	Feeds      []string
	FeedName   string
	FeedURL    string
	UserEmails []string
	Loops      int
}

func (fc *feedCommand) init() {
	fc.Config, fc.DBH = commonInit()
}

func (fc *feedCommand) configure(app *kingpin.Application) {
	feedCmd := app.Command("feeds", "manipulate feeds")

	feedCmd.Command("list", "Show all known feeds").Action(fc.list)
	feedCmd.Command("badfeeds", "Show feed with problems").Action(fc.badfeeds)

	add := feedCmd.Command("add", "Add a new feed to watch").Action(fc.addCMD)
	add.Arg("name", "Name of feed to add").Required().StringVar(&fc.FeedName)
	add.Arg("url", "URL of feed to add").Required().StringVar(&fc.FeedURL)
	add.Arg("emails", "List of emails to subscribe to feed.").StringsVar(&fc.UserEmails)

	delete := feedCmd.Command("delete", "Delete a feed from the database").Action(fc.delete)
	delete.Arg("url", "URL of the Feed to delete").Required().StringsVar(&fc.Feeds)

	test := feedCmd.Command("test", "Crawl and try to parse a feed from the command line").Action(fc.test)
	test.Arg("url", "URL of the Feed to delete").Required().StringVar(&fc.FeedURL)

	runone := feedCmd.Command("runone", "Crawl and mail a single feed for debugging").Action(fc.runone)
	runone.Flag("loops", "Numer of times to poll this feed.  -1 == forever.").Default("1").IntVar(&fc.Loops)
	runone.Arg("url", "URL of feed to crawl").Required().StringVar(&fc.FeedURL)
}

func (fc *feedCommand) runone(c *kingpin.ParseContext) error {
	fc.init()
	fc.Config.Mail.SendMail = true
	fc.Config.DB.UpdateDb = false

	mailer := mail.CreateAndStartMailer(fc.Config)

	feed, err := fc.DBH.GetFeedByURL(fc.FeedURL)
	if err != nil {
		return err
	}

	httpCrawlChannel := make(chan *feedwatcher.FeedCrawlRequest, 1)
	responseChannel := make(chan *feedwatcher.FeedCrawlResponse)

	// start crawler pool
	crawler.StartCrawlerPool(1, httpCrawlChannel)

	fw := feedwatcher.NewFeedWatcher(
		*feed,
		httpCrawlChannel,
		responseChannel,
		mailer.OutgoingMail,
		fc.DBH,
		[]string{},
		10,
		100,
	)
	feeds := make(map[string]*feedwatcher.FeedWatcher)
	feeds[fw.FeedInfo.URL] = fw
	if fc.Loops == -1 {
		for {
			resp := fw.CrawlFeed()
			err := fw.UpdateFeed(resp)
			if err != nil {
				fmt.Printf("Error when updating feed: %v\n", err)
			}
			time.Sleep(time.Second * time.Duration(fc.Config.Crawl.MinInterval))
		}
	} else if fc.Loops == 1 {
		resp := fw.CrawlFeed()
		err := fw.UpdateFeed(resp)
		if err != nil {
			fmt.Printf("Error when updating feed: %v\n", err)
		}
	} else {
		for i := 0; i < fc.Loops; i++ {
			resp := fw.CrawlFeed()
			err := fw.UpdateFeed(resp)
			if err != nil {
				fmt.Printf("Error when updating feed: %v\n", err)
			}
			time.Sleep(time.Second * time.Duration(fc.Config.Crawl.MinInterval))
		}
	}
	return nil
}

func (fc *feedCommand) badfeeds(c *kingpin.ParseContext) error {
	fc.init()
	feeds, err := fc.DBH.GetFeedsWithErrors()
	if err != nil {
		return err
	}
	fmt.Println("Feeds With Errors:")
	for _, f := range feeds {
		fmt.Printf("  Name: %s\n", f.Name)
		fmt.Printf("  Url: %s\n", f.URL)
		fmt.Printf("  Last Update: %s\n", f.LastPollTime)
		fmt.Printf("  Error: %s\n", strings.TrimSpace(f.LastPollError))
		blen := len(f.LastErrorResponse)
		if blen > 100 {
			blen = 100
		}
		fmt.Printf("  Last Content: %s...\n", f.LastErrorResponse[0:blen])
		fmt.Println()
	}

	fmt.Println()
	feeds, err = fc.DBH.GetStaleFeeds()
	if err != nil {
		return err
	}

	fmt.Println("Feeds With No Updates for 2 Weeks:")
	for _, f := range feeds {
		fmt.Printf("  Name: %s\n", f.Name)
		fmt.Printf("  Url: %s\n", f.URL)
		fmt.Printf("  Last Update: %s\n", f.LastPollTime)
		fmt.Println()
	}
	return nil
}

func (fc *feedCommand) list(c *kingpin.ParseContext) error {
	_, dbh := commonInit()
	feeds, err := dbh.GetAllFeeds()

	if err != nil {
		return err
	}

	fmt.Printf("Found %d feeds in the database:\n", len(feeds))
	for _, f := range feeds {
		fmt.Printf("Name: %s, Url: %s\n", f.Name, f.URL)
	}
	return nil
}

func (fc *feedCommand) addCMD(c *kingpin.ParseContext) error {
	fc.init()
	return fc.add()
}
func (fc *feedCommand) add() error {
	f, err := fc.DBH.AddFeed(fc.FeedName, fc.FeedURL)
	if err != nil {
		return err
	}
	fmt.Printf("Added feed %s at url %s\n", f.Name, f.URL)

	for _, email := range fc.UserEmails {
		user, err := fc.DBH.GetUserByEmail(email)
		if err != nil {
			return fmt.Errorf("Error looking up user %s, does it exist? (%v)", email, err)
		}
		err = fc.DBH.AddFeedsToUser(user, []*db.FeedInfo{f})
		if err != nil {
			return err
		}
		fmt.Printf("Subscribed %s to %s\n", email, f.Name)
	}
	return nil
}

func (fc *feedCommand) delete(c *kingpin.ParseContext) error {
	fc.init()
	hadError := false
	for _, feedURL := range fc.Feeds {
		err := fc.DBH.RemoveFeed(feedURL)
		if err != nil {
			hadError = true
			fmt.Printf("Error removing %s: %v\n", feedURL, err)
		}
	}
	if hadError {
		return fmt.Errorf("Error removing one or more feeds")
	}

	return nil
}

func (fc *feedCommand) test(c *kingpin.ParseContext) error {
	fc.init()
	url := fc.FeedURL

	resp, err := crawler.GetFeed(url, nil)
	if err != nil {
		return err
	}
	fmt.Println("HTTP Response (First 500 characters): ")
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return err
	}
	fmt.Println(string(dump[0:500]))

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	feed, stories, err := feed.ParseFeed(url, body)
	if err != nil {
		return err
	}

	fmt.Printf("Found %d items in feed:\n", len(stories))
	fmt.Printf("  Url: %s\n", feed.URL)
	fmt.Printf("  Title: %s\n", feed.Title)
	fmt.Printf("  Updated: %s\n", feed.Updated)
	fmt.Printf("  NextUpdate: %s\n", feed.NextUpdate)
	fmt.Printf("  Url: %s\n", feed.Link)
	for i, s := range stories {
		fmt.Printf("%d)  %s\n", i, s.Title)
		fmt.Printf("  Published  %s\n", s.Published)
		fmt.Printf("  Updated  %s\n", s.Updated)
		fmt.Println()
		fmt.Printf("%s\n", s.Content)
		fmt.Println()

		fmt.Printf("Mail Message for %s:\n", s.Title)
		fmt.Println()
		m := mail.CreateMailFromItem("From@Address", netmail.Address{Address: "To@Address"}, s)
		fmt.Println("****** Mail Message *******")
		b := bytes.NewBuffer([]byte{})
		m.WriteTo(b)
		spew.Dump(b)
		fmt.Println("****** ++++++++++++ *******")
	}

	return nil
}
