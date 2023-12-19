package main

import (
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/hobeone/rss2go/commands"
	"github.com/hobeone/rss2go/log"
	"github.com/sirupsen/logrus"
)

func main() {
	log.SetupLogger(logrus.StandardLogger())
	commands.RegisterCommands()
	kingpin.MustParse(commands.App.Parse(os.Args[1:]))
}
