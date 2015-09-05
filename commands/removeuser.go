package commands

import (
	"flag"
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/flagutil"
)

type RemoveUserCommand struct {
	Config *config.Config
	Dbh    *db.Handle
}

func MakeCmdRemoveUser() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       runRemoveUser,
		UsageLine: "removeuser email@address",
		Short:     "Remove a user from rss2go",
		Long: `
		Removes a user from the database including their subscriptions.

		Example:

		removeuser email@address
		`,
		Flag: *flag.NewFlagSet("removeuser", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", defaultConfig, "Config file to use.")

	return cmd
}

func runRemoveUser(cmd *flagutil.Command, args []string) {
	if len(args) < 1 {
		PrintErrorAndExit("Must give an email address to remove")
	}
	user_email := args[0]

	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))
	ru := NewRemoveUserCommand(cfg)
	ru.RemoveUser(user_email)
}

func NewRemoveUserCommand(cfg *config.Config) *RemoveUserCommand {
	var dbh *db.Handle
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDBHandle(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	}

	return &RemoveUserCommand{
		Config: cfg,
		Dbh:    dbh,
	}
}

func (self *RemoveUserCommand) RemoveUser(user_email string) {
	err := self.Dbh.RemoveUserByEmail(user_email)
	if err != nil {
		PrintErrorAndExit(fmt.Sprintf("Error removing user: %s", err))
	}
	fmt.Printf("Removed user %s.\n", user_email)
}
