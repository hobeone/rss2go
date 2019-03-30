package main

import (
	"os"

	"github.com/hobeone/rss2go/commands"
	"github.com/hobeone/rss2go/log"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	log.SetupLogger(logrus.StandardLogger())
	commands.RegisterCommands()
	kingpin.MustParse(commands.App.Parse(os.Args[1:]))
}
