package main

import (
	"fmt"
	"flag"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/flagutil"
)

func make_cmd_badfeeds() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       badFeeds,
		UsageLine: "badfeeds",
		Short:     "List all feeds with problems in the database.",
		Long: `
		List all the feeds in the database that have problems.

		* Feeds that got an error on their last poll.
		* Feed who haven't had new content in more than 30 days.
		`,
		Flag: *flag.NewFlagSet("badfeeds", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", default_config, "Config file to use.")
	return cmd
}

func badFeeds(cmd *flagutil.Command, args []string) {
	cfg := loadConfig(
		cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))
	db := db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	feeds, err := db.GetFeedsWithErrors()
	if err != nil {
		printErrorAndExit(err.Error())
	}
	fmt.Println("Feeds With Errors:")
	for _, f := range feeds {
		fmt.Printf("  Name: %s\n", f.Name)
		fmt.Printf("  Url: %s\n", f.Url)
		fmt.Printf("  Error: %s\n", f.LastPollError)
	}

	fmt.Println()
	feeds, err = db.GetStaleFeeds()
	if err != nil {
		printErrorAndExit(err.Error())
	}

	fmt.Println("Feeds With No Updates for 2 Weeks:")
	for _, f := range feeds {
		fmt.Printf("  Name: %s\n", f.Name)
		fmt.Printf("  Url: %s\n", f.Url)
		fmt.Printf("  Last Update: %s\n", f.LastPollTime)
		fmt.Println()
	}

}
