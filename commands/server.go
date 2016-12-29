package commands

import (
	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/webui"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

type serverCommand struct {
	Config *config.Config
	DBH    *db.Handle
}

func (sc *serverCommand) init() {
	sc.Config, sc.DBH = commonInit()
}

func (sc *serverCommand) configure(app *kingpin.Application) {
	app.Command("server", "run rss2go web server").Action(sc.run)
}

func (sc *serverCommand) run(c *kingpin.ParseContext) error {
	sc.init()
	if *debug || *debugdb {
		logrus.SetLevel(logrus.DebugLevel)
	}
	sc.Config = loadConfig(*configfile)
	d := webui.Dependencies{
		DBH: sc.DBH,
	}
	server := &webui.APIServer{
		Dependencies: d,
	}

	return server.Serve()
}
