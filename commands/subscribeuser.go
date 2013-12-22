package commands

import (
  "flag"
  "fmt"
  "github.com/hobeone/rss2go/config"
  "github.com/hobeone/rss2go/db"
  "github.com/hobeone/rss2go/flagutil"
)

func MakeCmdSubscribeUser() *flagutil.Command {
  cmd := &flagutil.Command{
    Run:       runSubscribeUser,
		UsageLine: "subscribe email@address http://feed_url",
    Short:     "Subscribe a User to Feed(s)",
    Long: `
		Subscribes a user to feeds.  The feed must already exist in the database

		Example:
		subscribeuser email@address http://feed/url.rss ....
		`,
    Flag: *flag.NewFlagSet("subscribeuser", flag.ExitOnError),
  }
  cmd.Flag.String("config_file", default_config, "Config file to use.")

  return cmd
}

type SubscribeUserCommand struct {
	Config *config.Config
	Dbh *db.DbDispatcher
}

func runSubscribeUser(cmd *flagutil.Command, args []string) {
  if len(args) < 2 {
    PrintErrorAndExit("Must give an email address and a feed url")
  }
  user_email := args[0]
	feed_urls := args[1:]

  cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	su := NewSubscribeUserCommand(cfg)
	su.SubscribeUser(user_email, feed_urls)
}

func NewSubscribeUserCommand(cfg *config.Config) *SubscribeUserCommand {
  var dbh *db.DbDispatcher
  if cfg.Db.Type == "memory" {
    dbh = db.NewMemoryDbDispatcher(cfg.Db.Verbose, cfg.Db.UpdateDb)
  } else {
    dbh = db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
  }

  return &SubscribeUserCommand{
    Config: cfg,
    Dbh:    dbh,
  }
}

func (self *SubscribeUserCommand) SubscribeUser(user_email string, feed_urls []string) {
	u, err := self.Dbh.GetUserByEmail(user_email)
	if err != nil {
		PrintErrorAndExit(fmt.Sprintf("Error getting user: %s" , err))
	}
	err = self.Dbh.AddFeedsToUser(u, feed_urls)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}
	fmt.Printf("Subscribed user %s to %d feed.\n", user_email, len(feed_urls))
}
