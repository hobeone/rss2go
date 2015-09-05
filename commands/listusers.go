package commands

import (
	"fmt"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/spf13/cobra"
)

type ListUsersCommand struct {
	Config *config.Config
	Dbh    *db.Handle
}

func MakeCmdListUsers() *cobra.Command {
	cmd := &cobra.Command{
		Run:   runListUsers,
		Use:   "listusers",
		Short: "List all users in rss2go",
		Long: `
		Lists all the users in the database.
		`,
	}
	return cmd
}

func runListUsers(cmd *cobra.Command, args []string) {
	cfg := loadConfig(ConfigFile)
	lu := NewListUsersCommand(cfg)
	lu.ListUsers()
}

func NewListUsersCommand(cfg *config.Config) *ListUsersCommand {
	var dbh *db.Handle
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDBHandle(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	}

	return &ListUsersCommand{
		Config: cfg,
		Dbh:    dbh,
	}
}

func (self *ListUsersCommand) ListUsers() {
	users, err := self.Dbh.GetAllUsers()
	if err != nil {
		PrintErrorAndExit(err.Error())
	}
	if len(users) > 0 {
		for _, u := range users {
			fmt.Printf("%d: %#v, %#v\n", u.ID, u.Name, u.Email)
			feeds, err := self.Dbh.GetUsersFeeds(&u)
			if err != nil {
				PrintErrorAndExit(fmt.Sprintf("Couldn't find users subscriptions: %s",
					err))
			}
			for _, fi := range feeds {
				fmt.Printf("  %s - %s\n", fi.Name, fi.URL)
			}
		}
	} else {
		fmt.Println("No Users found.")
	}
}
