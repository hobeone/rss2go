package commands

import (
	"flag"
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/flagutil"
)

type ListUsersCommand struct {
	Config *config.Config
	Dbh    *db.DbDispatcher
}

func MakeCmdListUsers() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       runListUsers,
		UsageLine: "listusers",
		Short:     "List all users in rss2go",
		Long: `
		Lists all the users in the database.
		`,
		Flag: *flag.NewFlagSet("listusers", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", default_config, "Config file to use.")

	return cmd
}

func runListUsers(cmd *flagutil.Command, args []string) {
	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))
	lu := NewListUsersCommand(cfg)
	lu.ListUsers()
}

func NewListUsersCommand(cfg *config.Config) *ListUsersCommand {
	var dbh *db.DbDispatcher
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDbDispatcher(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDbDispatcher(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
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
			fmt.Printf("%d: %#v, %#v\n", u.Id, u.Name, u.Email)
			feeds, err := self.Dbh.GetUsersFeeds(&u)
			if err != nil {
				PrintErrorAndExit(fmt.Sprintf("Couldn't find users subscriptions: %s",
					err))
			}
			for _, fi := range feeds {
				fmt.Printf("  %s - %s\n", fi.Name, fi.Url)
			}
		}
	} else {
		fmt.Println("No Users found.")
	}
}
