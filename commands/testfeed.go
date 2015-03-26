package commands

import (
	"flag"
	"fmt"
	"io/ioutil"
	netmail "net/mail"

	"github.com/davecgh/go-spew/spew"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/flagutil"
	"github.com/hobeone/rss2go/mail"
)

func MakeCmdTestFeed() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       testFeed,
		UsageLine: "testfeed",
		Short:     "Crawl and try to parse a feed from the command line.",
		Long: `
		Test crawl and parse one feed.  Doesn't need to exist in the database.

		Example:

		test_feed http://test/feed.rss
		`,
		Flag: *flag.NewFlagSet("testfeed", flag.ExitOnError),
	}

	return cmd
}

func testFeed(cmd *flagutil.Command, args []string) {
	if len(args) < 1 {
		PrintErrorAndExit("Need to provide a url to crawl.\n")
	}
	url := args[0]
	resp, err := crawler.GetFeed(url, nil)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		PrintErrorAndExit(err.Error())
	}

	feed, stories, err := feed.ParseFeed(url, body)
	if err != nil {
		PrintErrorAndExit(err.Error())
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
		spew.Dump(m.Export())
		fmt.Println("****** ++++++++++++ *******")
	}
}
