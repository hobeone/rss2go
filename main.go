package main

import (
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/hobeone/rss2go/commands"
	"github.com/hobeone/rss2go/log"
)

func main() {
	log.SetupLogger(false)
	commands.RegisterCommands()
	kingpin.MustParse(commands.App.Parse(os.Args[1:]))
}
