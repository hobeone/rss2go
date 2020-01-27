package commands

import (
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/log"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

const defaultConfig = "~/.config/rss2go/config.json"

var (
	App        = kingpin.New("rss2go", "A rss watcher and mailer")
	debug      = App.Flag("debug", "Enable Debug mode.").Bool()
	debugdb    = App.Flag("debugdb", "Log Database queries (noisy).").Default("false").Bool()
	quiet      = App.Flag("quiet", "Only log error or higher.").Default("false").Bool()
	configfile = App.Flag("config", "Config file to use").Default(defaultConfig).String()
)

// RegisterCommands registers all sub commands usable by Kingpin.
func RegisterCommands() {
	feedCmd := &feedCommand{}
	feedCmd.configure(App)

	userCmd := &userCommand{}
	userCmd.configure(App)

	opmlCmd := &opmlCommand{}
	opmlCmd.configure(App)

	daemonCmd := &daemonCommand{}
	daemonCmd.configure(App)

	createDBCmd := &createDBCommand{}
	createDBCmd.configure(App)

	showConfig := &showconfigCommand{}
	showConfig.configure(App)
}

func commonInit() (*config.Config, *db.Handle) {
	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	cfg := loadConfig(*configfile)
	logger := logrus.New()
	log.SetupLogger(logger)
	if *debugdb {
		logger.Level = logrus.DebugLevel
	}

	dbh := db.NewDBHandle(cfg.DB.Path, logger)

	return cfg, dbh
}

func loadConfig(cfile string) *config.Config {
	if len(cfile) == 0 {
		logrus.Infof("No --config_file given.  Using default: %s", *configfile)
		cfile = *configfile
	}

	logrus.Infof("Got config file: %s\n", cfile)
	cfg := config.NewConfig()
	err := cfg.ReadConfig(cfile)
	if err != nil {
		logrus.Fatal(err)
	}

	// Override cfg from flags
	cfg.DB.Verbose = *debugdb
	return cfg
}
