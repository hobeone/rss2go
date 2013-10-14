package main

import (
	"fmt"
	"github.com/hobeone/rss2go/crawler"
	"github.com/hobeone/rss2go/flagutil"
	"github.com/hobeone/rss2go/feed"
	"github.com/hobeone/rss2go/mail"
	"io/ioutil"
	"flag"
)

func make_cmd_test_feed() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       testFeed,
		UsageLine: "test_feed",
		Short:     "Crawl and try to parse a feed from the command line.",
		Long: `
		Test crawl and parse one feed.  Doesn't need to exist in the database.

		Example:

		test_feed http://test/feed.rss
		`,
		Flag: *flag.NewFlagSet("test_feed", flag.ExitOnError),
	}

	return cmd
}

func testFeed(cmd *flagutil.Command, args []string) {
	if len(args) < 1 {
		printErrorAndExit("Need to provide a url to crawl.\n")
	}
	url := args[0]
	resp, err := crawler.GetFeed(url)
	if err != nil {
		printErrorAndExit(err.Error())
	}
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		printErrorAndExit(err.Error())
	}

	feed, stories, err := feed.ParseFeed(url, body)
	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Printf("Found %d items in feed:\n", len(stories))
	fmt.Printf("  Url: %s\n", feed.Url)
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
		m := mail.CreateMailFromItem("From@Address", "To@Address", s)
		b, err := m.Bytes()
		if err != nil {
			fmt.Printf("Error converting %s to mail: %s\n", s.Title, err)
			continue
		}
		fmt.Println(string(b[:]))
	}
}
