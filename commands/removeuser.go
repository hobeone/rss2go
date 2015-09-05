package commands

import (
	"fmt"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/spf13/cobra"
)

type RemoveUserCommand struct {
	Config *config.Config
	Dbh    *db.Handle
}

func MakeCmdRemoveUser() *cobra.Command {
	cmd := &cobra.Command{
		Run:   runRemoveUser,
		Use:   "removeuser email@address",
		Short: "Remove a user from rss2go",
		Long: `
		Removes a user from the database including their subscriptions.

		Example:

		removeuser email@address
		`,
	}
	return cmd
}

func runRemoveUser(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		PrintErrorAndExit("Must give an email address to remove")
	}
	userEmail := args[0]

	cfg := loadConfig(ConfigFile)
	ru := NewRemoveUserCommand(cfg)
	ru.RemoveUser(userEmail)
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
