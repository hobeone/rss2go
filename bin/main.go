package main

import (
	"fmt"
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"github.com/hobeone/rss2go/config"
	"os"
	"log"
)

const DEFAULT_CONFIG = "~/.config/rss2go/config.toml"

func printErrorAndExit(err_string string) {
	fmt.Fprintf(os.Stderr, "ERROR: %s.\n", err_string)
	os.Exit(1)
}

func loadConfig(config_file string) *config.Config {
	if len(config_file) == 0 {
		log.Printf("No --config_file given.  Using default: %s\n", DEFAULT_CONFIG)
		config_file = DEFAULT_CONFIG
	}

	log.Printf("Got config file: %s\n", config_file)
	config := config.NewConfig()
	err := config.ReadConfig(config_file)
	if err != nil {
		log.Fatal(err)
	}
	return config
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
			make_cmd_removefeed(),
			make_cmd_listfeeds(),
			make_cmd_importopml(),
		},
		Flag: flag.NewFlagSet("rss2go", flag.ExitOnError),
	}
}

func main() {
	err := g_cmd.Flag.Parse(os.Args[1:])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	args := g_cmd.Flag.Args()
	err = g_cmd.Run(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	return
}
