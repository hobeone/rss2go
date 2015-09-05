package commands

import (
	"fmt"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/spf13/cobra"
)

func MakeCmdAddUser() *cobra.Command {
	cmd := &cobra.Command{
		Run:   runAddUser,
		Use:   "adduser username email@address",
		Short: "Add a user to rss2go",
		Long: `
		Adds a new user to the database and optionally subscribes them to the
		given	feeds.

		Example:
		adduser username email@address password
		-or-
		adduser username email@address password http://feed/url ....
		`,
	}
	return cmd
}

type AddUserCommand struct {
	Config *config.Config
	Dbh    *db.Handle
}

func runAddUser(cmd *cobra.Command, args []string) {
	if len(args) < 3 {
		PrintErrorAndExit("Must give: username email@address and_a_password")
	}
	userName := args[0]
	userEmail := args[1]
	pass := args[1]
	var feeds []string
	if len(args) > 3 {
		feeds = args[3:]
	}

	cfg := loadConfig(ConfigFile)
	cfg.Mail.SendMail = false
	cfg.Db.UpdateDb = true

	a := NewAddUserCommand(cfg)
	a.AddUser(userName, userEmail, pass, feeds)
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

func (cmd *AddUserCommand) AddUser(name string, email string, pass string, feedURLs []string) {
	user, err := cmd.Dbh.AddUser(name, email, pass)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}
	fmt.Printf("Added user %s <%s>.\n", name, email)

	// Add given feeds to user
	if len(feedURLs) > 0 {
		feeds := make([]*db.FeedInfo, len(feedURLs))
		for i, feedURL := range feedURLs {
			f, err := cmd.Dbh.GetFeedByURL(feedURL)
			if err != nil {
				PrintErrorAndExit(fmt.Sprintf("Feed %s doesn't exist in db, add it first.", feedURL))
			}
			feeds[i] = f
		}
		err = cmd.Dbh.AddFeedsToUser(user, feeds)
		if err != nil {
			PrintErrorAndExit(fmt.Sprintf("Error adding feeds to user: %s", err))
		}
	}
}
