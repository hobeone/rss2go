package main

import (
	"flag"

	"github.com/hobeone/rss2go/commands"
	"github.com/hobeone/rss2go/flagutil"
)

func main() {
	c := flagutil.NewCommands("rss2go command suite",
		commands.MakeCmdShowConfig(),
		commands.MakeCmdTestFeed(),
		commands.MakeCmdDaemon(),
		commands.MakeCmdRunOne(),
		commands.MakeCmdAddFeed(),
		commands.MakeCmdRemoveFeed(),
		commands.MakeCmdListFeeds(),
		commands.MakeCmdImportOpml(),
		commands.MakeCmdExportOPML(),
		commands.MakeCmdBadFeeds(),
		commands.MakeCmdListUsers(),
		commands.MakeCmdAddUser(),
		commands.MakeCmdRemoveUser(),
		commands.MakeCmdSubscribeUser(),
		commands.MakeCmdUnsubscribeUser(),
	)

	flag.Usage = c.Usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		commands.PrintErrorAndExit("No command given.  Use help to see all commands")
	}

	if err := c.Parse(args); err != nil {
		commands.PrintErrorAndExit(err.Error())
	}
}
