package commands

import (
	"fmt"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/config"
)

const defaultConfig = "~/.config/rss2go/config.json"

// PrintErrorAndEdit prints out the given string to SRDERR and exits
var PrintErrorAndExit = func(err_string string) {
	fmt.Fprintf(os.Stderr, "ERROR: %s.\n", err_string)
	os.Exit(1)
}

func loadConfig(configFile string) *config.Config {
	if len(configFile) == 0 {
		logrus.Infof("No --config_file given.  Using default: %s\n",
			defaultConfig)
		configFile = defaultConfig
	}

	logrus.Infof("Got config file: %s\n", configFile)
	config := config.NewConfig()
	err := config.ReadConfig(configFile)
	if err != nil {
		logrus.Fatal(err)
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
