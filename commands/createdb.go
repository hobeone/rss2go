package commands

import (
	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"gopkg.in/alecthomas/kingpin.v2"
)

type createDBCommand struct {
	Config *config.Config
	DBH    *db.Handle
}

func (cc *createDBCommand) init() {
	cc.Config, cc.DBH = commonInit()
}

func (cc *createDBCommand) configure(app *kingpin.Application) {
	app.Command("createdb", "create or migrate the database").Action(cc.migrateCmd)
}

func (cc *createDBCommand) migrateCmd(c *kingpin.ParseContext) error {
	cc.init()
	return cc.migrate()
}

func (cc *createDBCommand) migrate() error {
	err := cc.DBH.Migrate(db.SchemaMigrations())
	if err != nil {
		logrus.Fatalf("Error starting migration: %v", err)
	}
	return nil
}
