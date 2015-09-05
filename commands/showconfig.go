package commands

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/hobeone/rss2go/config"
	"github.com/spf13/cobra"
)

func MakeCmdShowConfig() *cobra.Command {
	cmd := &cobra.Command{
		Run:   runShowConfig,
		Use:   "showconfig",
		Short: "Shows the current config settings",
		Long: `
		Shows the current config settings
		`,
	}
	return cmd
}

type ShowConfigCommand struct {
	Config *config.Config
}

func runShowConfig(cmd *cobra.Command, args []string) {
	cfg := loadConfig(ConfigFile)

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
