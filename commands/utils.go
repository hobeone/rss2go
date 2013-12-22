package commands

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/hobeone/rss2go/config"
	"os"
	"testing"
)

const default_config = "~/.config/rss2go/config.toml"

var PrintErrorAndExit = func(err_string string) {
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

// Used by tests
func overrideExit() {
	PrintErrorAndExit = func(err_string string) {
		panic(err_string)
	}
}

func assertNoPanic(t *testing.T, err string) {
	if r := recover(); r != nil {
		t.Fatalf("%s: %s", err, r)
	}
}

func assertPanic(t *testing.T, err string) {
	if r := recover(); r == nil {
		t.Fatalf("%s", err)
	}
}
