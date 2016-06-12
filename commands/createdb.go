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
	app.Command("createdb", "create or migrate the database").Action(cc.migrate)
}

func (cc *createDBCommand) migrate(c *kingpin.ParseContext) error {
	cc.init()
	err := cc.DBH.Migrate("db/migrations/sqlite3")
	if err != nil {
		logrus.Fatalf("Error starting migration: %v", err)
	}
	return nil
}
