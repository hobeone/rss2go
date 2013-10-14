package main

import (
	"fmt"
	"flag"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/flagutil"
)

func make_cmd_listfeeds() *flagutil.Command {
	cmd := &flagutil.Command{
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

func listFeed(cmd *flagutil.Command, args []string) {
	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))
	db := db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	feeds, err := db.GetAllFeeds()

	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Printf("Found %d feeds in the database:\n", len(feeds))
	for _, f := range feeds {
		fmt.Printf("Name: %s, Url: %s\n", f.Name, f.Url)
	}
}
