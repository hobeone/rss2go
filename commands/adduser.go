package commands

import (
  "flag"
  "fmt"
	"net/mail"
  "github.com/hobeone/rss2go/config"
  "github.com/hobeone/rss2go/db"
  "github.com/hobeone/rss2go/flagutil"
)

func MakeCmdAddUser() *flagutil.Command {
  cmd := &flagutil.Command{
    Run:       runAddUser,
    UsageLine: "adduser username email@address",
    Short:     "Add a user to rss2go",
    Long: `
		Adds a new user to the database and optionally subscribes them to the
		given	feeds.

		Example:
		adduser username email@address
		-or-
		adduser username email@address http://feed/url ....
		`,
    Flag: *flag.NewFlagSet("adduser", flag.ExitOnError),
  }
  cmd.Flag.String("config_file", default_config, "Config file to use.")

  return cmd
}

type AddUserCommand struct {
  Config *config.Config
  Dbh    *db.DbDispatcher
}

func runAddUser(cmd *flagutil.Command, args []string) {
  if len(args) < 2 {
    PrintErrorAndExit("Must give a username and email address")
  }
  user_name := args[0]
  user_email := args[1]
  var feeds []string
  if len(args) > 2 {
    feeds = args[2:]
  }

  cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))
  cfg.Mail.SendMail = false
  cfg.Db.UpdateDb = true

  a := NewAddUserCommand(cfg)
  a.AddUser(user_name, user_email, feeds)
}

func NewAddUserCommand(cfg *config.Config) *AddUserCommand {
  var dbh *db.DbDispatcher
  if cfg.Db.Type == "memory" {
    dbh = db.NewMemoryDbDispatcher(cfg.Db.Verbose, cfg.Db.UpdateDb)
  } else {
    dbh = db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
  }

  return &AddUserCommand{
    Config: cfg,
    Dbh:    dbh,
  }
}

func (self *AddUserCommand) AddUser(name string, email string, feed_urls []string) {
  //TODO: validate email address
	addr, err := mail.ParseAddress(email)
	if err != nil {
		PrintErrorAndExit(fmt.Sprintf("Couldn't parse email: %s", err))
	}
  user, err := self.Dbh.AddUser(name, addr.Address)
  if err != nil {
    PrintErrorAndExit(err.Error())
  }
	fmt.Printf("Added user %s <%s>.\n", name, email)

  // Add given feeds to user
  if len(feed_urls) > 0 {
    err = self.Dbh.AddFeedsToUser(user, feed_urls)
    if err != nil {
      PrintErrorAndExit(fmt.Sprintf("Error adding feeds to user: %s", err))
    }
  }
}
