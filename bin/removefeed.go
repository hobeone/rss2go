package main

import (
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"github.com/hobeone/rss2go/db"
	"fmt"
)

func make_cmd_removefeed() *commander.Command {
	cmd := &commander.Command{
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

	return cmd
}

func removeFeed(cmd *commander.Command, args []string) {
	if len(args) < 1 {
		printErrorAndExit("Must supply feed name and url")
	}
	feed_url := args[0]

	cfg := loadConfig(g_cmd.Flag.Lookup("config_file").Value.Get().(string))
	purge_feed := cmd.Flag.Lookup("purge_feed").Value.Get().(bool)

	cfg.Mail.SendMail = false
	cfg.Db.UpdateDb = true

	db := db.NewDbDispatcher(cfg.Db.Path, true, true)

	err := db.RemoveFeed(feed_url, purge_feed)
	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Printf("Removed feed %s.\n", feed_url)
}
