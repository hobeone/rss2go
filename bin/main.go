package main

import (
	"fmt"
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"os"
)

const DEFAULT_CONFIG = "~/.config/rssgomail/config.toml"

func printErrorAndExit(err_string string) {
	fmt.Fprintf(os.Stderr, "ERROR: %s.\n", err_string)
	os.Exit(1)
}


var g_cmd *commander.Commander

func init() {
	g_cmd = &commander.Commander{
		Name: os.Args[0],
		Commands: []*commander.Command{
			make_cmd_test_feed(),
			make_cmd_daemon(),
			make_cmd_runone(),
			make_cmd_addfeed(),
		},
		Flag: flag.NewFlagSet("rss2go", flag.ExitOnError),
	}
}

func main() {
	err := g_cmd.Flag.Parse(os.Args[1:])
	if err != nil {
		fmt.Printf("**err**: %v\n", err)
		os.Exit(1)
	}

	args := g_cmd.Flag.Args()
	err = g_cmd.Run(args)
	if err != nil {
		fmt.Printf("**err**: %v\n", err)
		os.Exit(1)
	}

	return
}
