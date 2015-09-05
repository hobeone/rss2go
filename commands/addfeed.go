package commands

import (
	"fmt"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/spf13/cobra"
)

// MakeCmdAddFeed defines the addfeed command line mode
func MakeCmdAddFeed() *cobra.Command {
	cmd := &cobra.Command{
		Run:   runAddFeed,
		Use:   "addfeed FeedName FeedUrl [user email]....",
		Short: "Add a feed to the database. Optionally subscribing the given emails to it.",
		Long: `
		Add a feed URL to the database and optionally subscribe a list of existing users to it.

		Example:
		addfeed TestFeed http://test/feed.rss
		`,
	}
	return cmd
}

// AddFeedCommand encapsulates the functionality for adding a feed from the
// command line.
type AddFeedCommand struct {
	Config *config.Config
	DBH    *db.Handle
}

//NewAddFeedCommand returns a pointer to a newly created AddFeedCommand struct
//with defaults set.
func NewAddFeedCommand(cfg *config.Config) *AddFeedCommand {
	var dbh *db.Handle
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDBHandle(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	}

	return &AddFeedCommand{
		Config: cfg,
		DBH:    dbh,
	}
}

// AddFeed adds the given feed to the database and subscribes any given users
// to it.
func (adder *AddFeedCommand) AddFeed(feedName, feedURL string, userEmails []string) {
	_, err := adder.DBH.AddFeed(feedName, feedURL)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}

	fmt.Printf("Added feed %s at url %s\n", feedName, feedURL)

	subscriber := &SubscribeUserCommand{
		Dbh: adder.DBH,
	}

	for _, email := range userEmails {
		subscriber.SubscribeUser(email, []string{feedURL})
	}
}

func runAddFeed(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		PrintErrorAndExit("Must supply feed name and url")
	}

	feedName := args[0]
	feedURL := args[1]
	userEmails := args[2:]

	cfg := loadConfig(ConfigFile)

	af := NewAddFeedCommand(cfg)
	af.AddFeed(feedName, feedURL, userEmails)
}
