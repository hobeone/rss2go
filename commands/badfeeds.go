package commands

import (
	"fmt"

	"github.com/hobeone/rss2go/db"
	"github.com/spf13/cobra"
)

func MakeCmdBadFeeds() *cobra.Command {
	cmd := &cobra.Command{
		Run:   badFeeds,
		Use:   "badfeeds",
		Short: "List all feeds with problems in the database.",
		Long: `
		List all the feeds in the database that have problems.

		* Feeds that got an error on their last poll.
		* Feed who haven't had new content in more than 30 days.
		`,
	}
	return cmd
}

func badFeeds(cmd *cobra.Command, args []string) {
	cfg := loadConfig(ConfigFile)
	db := db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)

	feeds, err := db.GetFeedsWithErrors()
	if err != nil {
		PrintErrorAndExit(err.Error())
	}
	fmt.Println("Feeds With Errors:")
	for _, f := range feeds {
		fmt.Printf("  Name: %s\n", f.Name)
		fmt.Printf("  Url: %s\n", f.URL)
		fmt.Printf("  Last Update: %s\n", f.LastPollTime)
		fmt.Printf("  Error: %s\n", f.LastPollError)
	}

	fmt.Println()
	feeds, err = db.GetStaleFeeds()
	if err != nil {
		PrintErrorAndExit(err.Error())
	}

	fmt.Println("Feeds With No Updates for 2 Weeks:")
	for _, f := range feeds {
		fmt.Printf("  Name: %s\n", f.Name)
		fmt.Printf("  Url: %s\n", f.URL)
		fmt.Printf("  Last Update: %s\n", f.LastPollTime)
		fmt.Println()
	}
}
