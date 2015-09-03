package commands

import (
	"flag"
	"fmt"

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
		adduser username email@address password
		-or-
		adduser username email@address password http://feed/url ....
		`,
		Flag: *flag.NewFlagSet("adduser", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", default_config, "Config file to use.")

	return cmd
}

type AddUserCommand struct {
	Config *config.Config
	Dbh    *db.Handle
}

func runAddUser(cmd *flagutil.Command, args []string) {
	if len(args) < 3 {
		PrintErrorAndExit("Must give: username email@address and_a_password")
	}
	user_name := args[0]
	user_email := args[1]
	pass := args[1]
	var feeds []string
	if len(args) > 3 {
		feeds = args[3:]
	}

	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))
	cfg.Mail.SendMail = false
	cfg.Db.UpdateDb = true

	a := NewAddUserCommand(cfg)
	a.AddUser(user_name, user_email, pass, feeds)
}

func NewAddUserCommand(cfg *config.Config) *AddUserCommand {
	var dbh *db.Handle
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDBHandle(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	}

	return &AddUserCommand{
		Config: cfg,
		Dbh:    dbh,
	}
}

func (self *AddUserCommand) AddUser(name string, email string, pass string, feed_urls []string) {
	user, err := self.Dbh.AddUser(name, email, pass)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}
	fmt.Printf("Added user %s <%s>.\n", name, email)

	// Add given feeds to user
	if len(feed_urls) > 0 {
		feeds := make([]*db.FeedInfo, len(feed_urls))
		for i, feed_url := range feed_urls {
			f, err := self.Dbh.GetFeedByURL(feed_url)
			if err != nil {
				PrintErrorAndExit(fmt.Sprintf("Feed %s doesn't exist in db, add it first.", feed_url))
			}
			feeds[i] = f
		}
		err = self.Dbh.AddFeedsToUser(user, feeds)
		if err != nil {
			PrintErrorAndExit(fmt.Sprintf("Error adding feeds to user: %s", err))
		}
	}
}
