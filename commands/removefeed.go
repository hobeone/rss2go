package commands

import (
	"flag"
	"fmt"

	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/flagutil"
)

func MakeCmdRemoveFeed() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       removeFeed,
		UsageLine: "removefeed URL, URL, ...",
		Short:     "Remove feeds from the database.",
		Long: `
Remove a feed URL from the database and optionally purge it's state
from the database.

Example:
removefeed --purge_feed=false http://test/feed.rss http://test/other.rss
		`,
		Flag: *flag.NewFlagSet("removefeed", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", defaultConfig, "Config file to use.")

	return cmd
}

func removeFeed(cmd *flagutil.Command, args []string) {
	if len(args) < 1 {
		PrintErrorAndExit("Must supply at least one feed url to remove")
	}

	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	cfg.Mail.SendMail = false
	cfg.Db.UpdateDb = true

	db := db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	had_error := false
	for _, feed_url := range args {
		err := db.RemoveFeed(feed_url)
		if err != nil {
			fmt.Println(err.Error())
			had_error = true
		}
		fmt.Printf("Removed feed %s.\n", feed_url)
	}
	if had_error {
		PrintErrorAndExit("Error trying to remove one or more feeds")
	}
}
