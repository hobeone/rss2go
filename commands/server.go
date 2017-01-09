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
	Port   int32
}

func (sc *serverCommand) init() {
	sc.Config, sc.DBH = commonInit()
}

func (sc *serverCommand) configure(app *kingpin.Application) {
	cmd := app.Command("server", "run rss2go web server").Action(sc.run)
	cmd.Flag("port", "port to listen on").Default("7999").OverrideDefaultFromEnvar("PORT").Int32Var(&sc.Port)
}

func (sc *serverCommand) run(c *kingpin.ParseContext) error {
	sc.init()
	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	sc.Config = loadConfig(*configfile)
	d := webui.Dependencies{
		DBH: sc.DBH,
	}
	server := &webui.APIServer{
		Dependencies: d,
		Port:         sc.Port,
	}
	return server.Serve()
}
