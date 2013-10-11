package main

import (
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"log"
	"fmt"
)

func make_cmd_listfeeds() *commander.Command {
	cmd := &commander.Command{
		Run:       listFeed,
		UsageLine: "listfeeds",
		Short:     "List all the feeds in the database.",
		Long: `
		List all the feeds in the database.

		Example:
		rss2go listfeeds
		`,
		Flag: *flag.NewFlagSet("listfeeds", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", "", "Config file to use.")

	return cmd
}

func listFeed(cmd *commander.Command, args []string) {
	config_file := cmd.Flag.Lookup("config_file").Value.Get().(string)

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

	db := db.NewDbDispatcher(config.Db.Path, false, true)

	feeds, err := db.GetAllFeeds()

	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Printf("Found %d feeds in the database:\n", len(feeds))
	for _, f := range feeds {
		fmt.Printf("Name: %s, Url: %s\n", f.Name, f.Url)
	}
}
