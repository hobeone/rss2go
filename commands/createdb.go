package commands

import (
	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/spf13/cobra"
)

func MakeCmdCreateDB() *cobra.Command {
	cmd := &cobra.Command{
		Run:   runCreateDB,
		Use:   "createdb",
		Short: "Create or Migrate the database to the most current schema.",
		Long: `
		Create or migrate the database to the most current schema.

		Example:
		createdb
		`,
	}
	return cmd
}

type CreateDBCommand struct {
	Config *config.Config
	DBH    *db.Handle
}

func NewCreateDBCommand(cfg *config.Config) *CreateDBCommand {
	var dbh *db.Handle
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDBHandle(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	}

	return &CreateDBCommand{
		Config: cfg,
		DBH:    dbh,
	}
}

func (cmd *CreateDBCommand) CreateDB() {
	err := cmd.DBH.Migrate("db/migrations/sqlite3")
	if err != nil {
		logrus.Fatalf("Error starting migration: %v", err)
	}
}

func runCreateDB(cmd *cobra.Command, args []string) {
	cfg := loadConfig(ConfigFile)
	cdb := NewCreateDBCommand(cfg)
	cdb.CreateDB()
}
