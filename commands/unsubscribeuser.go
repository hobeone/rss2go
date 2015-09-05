package commands

import (
	"flag"
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/flagutil"
)

func MakeCmdUnsubscribeUser() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       runUnsubscribeUser,
		UsageLine: "unsubscribe email@address http://feed_url",
		Short:     "unsubscribe a User to Feed(s)",
		Long: `
		Unsubscribes a user to feeds.  The feed must already exist in the database

		Example:
		unsubscribeuser email@address http://feed/url.rss ....
		`,
		Flag: *flag.NewFlagSet("unsubscribeuser", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", defaultConfig, "Config file to use.")

	return cmd
}

type UnsubscribeUserCommand struct {
	Config *config.Config
	Dbh    *db.Handle
}

func runUnsubscribeUser(cmd *flagutil.Command, args []string) {
	if len(args) < 2 {
		PrintErrorAndExit("Must give an email address and a feed url")
	}
	userEmail := args[0]
	feedURLs := args[1:]

	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	su := NewUnsubscribeUserCommand(cfg)
	su.UnsubscribeUser(userEmail, feedURLs)
}

func NewUnsubscribeUserCommand(cfg *config.Config) *UnsubscribeUserCommand {
	var dbh *db.Handle
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDBHandle(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	}

	return &UnsubscribeUserCommand{
		Config: cfg,
		Dbh:    dbh,
	}
}

func (self *UnsubscribeUserCommand) UnsubscribeUser(user_email string, feed_urls []string) {
	u, err := self.Dbh.GetUserByEmail(user_email)
	if err != nil {
		PrintErrorAndExit(fmt.Sprintf("Error getting user: %s", err))
	}

	feeds := []*db.FeedInfo{}
	for _, feedURL := range feed_urls {
		f, err := self.Dbh.GetFeedByURL(feedURL)
		if err != nil {
			fmt.Printf("Feed %s doesn't exist in db, skipping.\n", feedURL)
			continue
		}
		feeds = append(feeds, f)
	}
	if len(feeds) > 0 {
		err = self.Dbh.RemoveFeedsFromUser(u, feeds)
		if err != nil {
			PrintErrorAndExit(fmt.Sprintf("Error removing feeds from user: %s", err))
		}
	}
}
