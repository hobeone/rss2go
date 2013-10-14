package main

import (
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/flagutil"
	"fmt"
	"flag"
)

func make_cmd_removefeed() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       removeFeed,
		UsageLine: "removefeed FeedUrl",
		Short:     "Remove a feed from the database.",
		Long: `
Remove a feed URL from the database and optionally purge it's state
from the database.

Example:
removefeed --purge_feed http://test/feed.rss
		`,
		Flag: *flag.NewFlagSet("removefeed", flag.ExitOnError),
	}
	cmd.Flag.Bool("purge_feed", true, "Purge known item records for this feed.")
	cmd.Flag.String("config_file", default_config, "Config file to use.")

	return cmd
}

func removeFeed(cmd *flagutil.Command, args []string) {
	if len(args) < 1 {
		printErrorAndExit("Must supply feed name and url")
	}
	feed_url := args[0]

	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))
	purge_feed := cmd.Flag.Lookup("purge_feed").Value.(flag.Getter).Get().(bool)

	cfg.Mail.SendMail = false
	cfg.Db.UpdateDb = true

	db := db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	err := db.RemoveFeed(feed_url, purge_feed)
	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Printf("Removed feed %s.\n", feed_url)
}
