package commands

import (
	"fmt"

	"github.com/hobeone/rss2go/db"
	"github.com/spf13/cobra"
)

func MakeCmdListFeeds() *cobra.Command {
	cmd := &cobra.Command{
		Run:   listFeed,
		Use:   "listfeeds",
		Short: "List all the feeds in the database.",
		Long: `
		List all the feeds in the database.

		Example:
		rss2go listfeeds
		`,
	}
	return cmd
}

func listFeed(cmd *cobra.Command, args []string) {
	cfg := loadConfig(ConfigFile)
	db := db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	feeds, err := db.GetAllFeeds()

	if err != nil {
		PrintErrorAndExit(err.Error())
	}

	fmt.Printf("Found %d feeds in the database:\n", len(feeds))
	for _, f := range feeds {
		fmt.Printf("Name: %s, Url: %s\n", f.Name, f.URL)
	}
}
