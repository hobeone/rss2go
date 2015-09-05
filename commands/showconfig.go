package commands

import (
	"flag"

	"github.com/davecgh/go-spew/spew"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/flagutil"
)

func MakeCmdShowConfig() *flagutil.Command {
	cmd := &flagutil.Command{
		Run:       runShowConfig,
		UsageLine: "showconfig",
		Short:     "Shows the current config settings",
		Long: `
		Shows the current config settings
		`,
		Flag: *flag.NewFlagSet("showconfig", flag.ExitOnError),
	}
	cmd.Flag.String("config_file", defaultConfig, "Config file to use.")

	return cmd
}

type ShowConfigCommand struct {
	Config *config.Config
}

func runShowConfig(cmd *flagutil.Command, args []string) {
	cfg := loadConfig(cmd.Flag.Lookup("config_file").Value.(flag.Getter).Get().(string))

	su := NewShowConfigCommand(cfg)
	su.ShowConfig()
}

func NewShowConfigCommand(cfg *config.Config) *ShowConfigCommand {
	return &ShowConfigCommand{
		Config: cfg,
	}
}

func (s *ShowConfigCommand) ShowConfig() {
	spew.Dump(s.Config)
}
