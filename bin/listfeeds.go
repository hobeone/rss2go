package main

import (
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"github.com/hobeone/rss2go/db"
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
	return cmd
}

func listFeed(cmd *commander.Command, args []string) {
	cfg := loadConfig(g_cmd.Flag.Lookup("config_file").Value.Get().(string))

	db := db.NewDbDispatcher(cfg.Db.Path, false, true)

	feeds, err := db.GetAllFeeds()

	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Printf("Found %d feeds in the database:\n", len(feeds))
	for _, f := range feeds {
		fmt.Printf("Name: %s, Url: %s\n", f.Name, f.Url)
	}
}
