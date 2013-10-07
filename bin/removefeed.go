package main

import (
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"log"
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
	cmd.Flag.String("config_file", "", "Config file to use.")
	cmd.Flag.Bool("purge_feed", true, "Purge known item records for this feed.")

	return cmd
}

func removeFeed(cmd *commander.Command, args []string) {
	if len(args) < 1 {
		printErrorAndExit("Must supply feed name and url")
	}
	feed_url := args[0]

	config_file := cmd.Flag.Lookup("config_file").Value.Get().(string)
	purge_feed := cmd.Flag.Lookup("purge_feed").Value.Get().(bool)

	if len(config_file) == 0 {
		log.Printf("No --config_file given.  Using default: %s\n", DEFAULT_CONFIG)
		config_file = DEFAULT_CONFIG
	}

	log.Printf("Got config file: %s\n", config_file)
	config := config.NewConfig()
	err := config.ReadConfig(config_file)
	if err != nil {
		log.Fatal(err)
	}

	config.Mail.SendMail = false
	config.Db.UpdateDb = true

	db := db.NewDbDispatcher(config.Db.Path, true, true)

	err = db.RemoveFeed(feed_url, purge_feed)
	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Printf("Removed feed %s.", feed_url)
}
