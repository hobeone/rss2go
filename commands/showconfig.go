package commands

import (
	"github.com/alecthomas/kingpin/v2"
	"github.com/davecgh/go-spew/spew"
	"github.com/hobeone/rss2go/config"
)

type showconfigCommand struct {
	Config *config.Config
}

func (sc *showconfigCommand) init() {
	sc.Config = loadConfig(*configfile)
}

func (sc *showconfigCommand) configure(app *kingpin.Application) {
	app.Command("showconfig", "create or migrate the database").Action(sc.showconfig)
}

func (sc *showconfigCommand) showconfig(c *kingpin.ParseContext) error {
	sc.init()
	spew.Dump(sc.Config)
	return nil
}
