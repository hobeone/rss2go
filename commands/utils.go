package commands

import (
	"fmt"
	"os"
	"github.com/hobeone/rss2go/config"
	"github.com/golang/glog"

)

const default_config = "~/.config/rss2go/config.toml"

func PrintErrorAndExit(err_string string) {
	fmt.Fprintf(os.Stderr, "ERROR: %s.\n", err_string)
	os.Exit(1)
}

func loadConfig(config_file string) *config.Config {
	if len(config_file) == 0 {
		glog.Infof("No --config_file given.  Using default: %s\n",
			default_config)
		config_file = default_config
	}

	glog.Infof("Got config file: %s\n", config_file)
	config := config.NewConfig()
	err := config.ReadConfig(config_file)
	if err != nil {
		glog.Fatal(err)
	}
	return config
}
