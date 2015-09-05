package commands

import (
	"fmt"

	"github.com/hobeone/rss2go/db"
	"github.com/spf13/cobra"
)

func MakeCmdRemoveFeed() *cobra.Command {
	cmd := &cobra.Command{
		Run:   removeFeed,
		Use:   "removefeed URL, URL, ...",
		Short: "Remove feeds from the database.",
		Long: `
Remove a feed URL from the database and optionally purge it's state
from the database.

Example:
removefeed --purge_feed=false http://test/feed.rss http://test/other.rss
		`,
	}
	return cmd
}

func removeFeed(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		PrintErrorAndExit("Must supply at least one feed url to remove")
	}

	cfg := loadConfig(ConfigFile)
	cfg.Mail.SendMail = false
	cfg.Db.UpdateDb = true

	db := db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	hadError := false
	for _, feedURL := range args {
		err := db.RemoveFeed(feedURL)
		if err != nil {
			fmt.Println(err.Error())
			hadError = true
		}
		fmt.Printf("Removed feed %s.\n", feedURL)
	}
	if hadError {
		PrintErrorAndExit("Error trying to remove one or more feeds")
	}
}
