package main

import (
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/flagutil"
	"os"
	"log"
	"flag"
)

const default_config = "~/.config/rss2go/config.toml"

func printErrorAndExit(err_string string) {
	fmt.Fprintf(os.Stderr, "ERROR: %s.\n", err_string)
	os.Exit(1)
}

func loadConfig(config_file string) *config.Config {
	if len(config_file) == 0 {
		log.Printf("No --config_file given.  Using default: %s\n",
			default_config)
		config_file = default_config
	}

	log.Printf("Got config file: %s\n", config_file)
	config := config.NewConfig()
	err := config.ReadConfig(config_file)
	if err != nil {
		log.Fatal(err)
	}
	return config
}

func main() {
	commands := flagutil.NewCommands("rss2go command suite",
		make_cmd_test_feed(),
		make_cmd_daemon(),
		make_cmd_runone(),
		make_cmd_addfeed(),
		make_cmd_removefeed(),
		make_cmd_listfeeds(),
		make_cmd_importopml(),
	)

	flag.Set("logtostderr", "true")

	flag.Usage = commands.Usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		printErrorAndExit("No command given.  Use help to see all commands")
	}

	if err := commands.Parse(args); err != nil {
		printErrorAndExit(err.Error())
	}
}
