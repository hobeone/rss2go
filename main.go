package main

import (
	"os"

	"github.com/hobeone/rss2go/commands"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	commands.RegisterCommands()
	kingpin.MustParse(commands.App.Parse(os.Args[1:]))
}
