package main

import (
	"bytes"
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"encoding/xml"
	"fmt"
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"github.com/hobeone/rss2go/mail"
	"github.com/hobeone/rss2go/opml"
	"github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"sync"
)

func make_cmd_importopml() *commander.Command {
	cmd := &commander.Command{
		Run:       importOPML,
		UsageLine: "importopml opmlfile",
		Short:     "Import all feeds from an opml file.",
		Long: `
		Import all feeds from a given OPML file and optionally poll them to add existing
		items to the list of known items.

		Example:
		importopml --update_feeds feeds.opml
		`,
		Flag: *flag.NewFlagSet("importopml", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", "", "Config file to use.")
	cmd.Flag.Bool("update_feeds", false, "Get the current feed contents and add them to the database.")

	return cmd
}

func importOPML(cmd *commander.Command, args []string) {
	if len(args) < 1 {
		printErrorAndExit("Must supply filename to import.")
	}
	opml_file := args[0]

	update_feeds := cmd.Flag.Lookup("update_feeds").Value.Get().(bool)
	cfg := loadConfig(g_cmd.Flag.Lookup("config_file").Value.Get().(string))

	// Override config settings
	cfg.Mail.SendMail = false
	cfg.Db.UpdateDb = true

	mailer := mail.CreateAndStartStubMailer()
	dbh := db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	fr, err := ioutil.ReadFile(opml_file)
	if err != nil {
		log.Fatalf("Error reading OPML file: %s", err.Error())
	}
	o := opml.Opml{}
	d := xml.NewDecoder(bytes.NewReader(fr))
	d.CharsetReader = charset.NewReader
	d.Strict = false

	if err := d.Decode(&o); err != nil {
		log.Fatalf("opml error: %v", err.Error())
	}
	feeds := make(map[string]string)
	var proc func(outlines []*opml.OpmlOutline)
	proc = func(outlines []*opml.OpmlOutline) {
		for _, o := range outlines {
			if o.XmlUrl != "" {
				feeds[o.XmlUrl] = o.Text
			}
			proc(o.Outline)
		}
	}
	proc(o.Outline)

	new_feeds := []*db.FeedInfo{}
	for k, v := range feeds {
		feed, err := dbh.AddFeed(v, k)
		if err != nil {
			fmt.Println(err)
			if err == sqlite3.ErrConstraint {
				fmt.Printf("Feed %s already exists in database, skipping.\n", k)
				continue
			} else {
				printErrorAndExit(err.Error())
			}
		}
		new_feeds = append(new_feeds, feed)
		fmt.Printf("Added feed \"%s\" at url \"%s\"\n", v, k)
	}
	if len(new_feeds) > 0 && update_feeds {
		http_crawl_channel := make(chan *feed_watcher.FeedCrawlRequest)
		response_channel := make(chan *feed_watcher.FeedCrawlResponse)
		crawler.StartCrawlerPool(cfg.Crawl.MaxCrawlers, http_crawl_channel)

		wg := sync.WaitGroup{}
		for _, feed := range new_feeds {
			guids, err := dbh.GetGuidsForFeed(feed.Id, &[]string{})
			if err != nil {
				printErrorAndExit(err.Error())
			}

			fw := feed_watcher.NewFeedWatcher(
				*feed,
				http_crawl_channel,
				response_channel,
				mailer.OutgoingMail,
				dbh,
				*guids,
				10,
				100,
			)

			wg.Add(1)
			go func() {
				defer wg.Done()
				fw.UpdateFeed()
			}()
		}
		wg.Wait()
	}
}
