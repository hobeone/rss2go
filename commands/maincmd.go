package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Flag controlling if mail should be sent or not
var SendMail bool

// Flag controlling if updates should be written to DB or not
var DBUpdates bool

// Flag controlling if feeds should be polled or not
var PollFeeds bool

// Flag controlling logging verbosity
var Verbose bool

// Flag telling rss2go which config file to use
var ConfigFile string

func init() {
	Rss2goCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false,
		"log verbose information")

	Rss2goCmd.PersistentFlags().StringVar(&ConfigFile, "config_file", defaultConfig, "Config file to use.")

	// Feed Commands
	Rss2goCmd.AddCommand(MakeCmdAddFeed())
	Rss2goCmd.AddCommand(MakeCmdRemoveFeed())
	Rss2goCmd.AddCommand(MakeCmdListFeeds())
	Rss2goCmd.AddCommand(MakeCmdBadFeeds())
	Rss2goCmd.AddCommand(MakeCmdTestFeed())
	// User Commands
	Rss2goCmd.AddCommand(MakeCmdAddUser())
	Rss2goCmd.AddCommand(MakeCmdRemoveUser())
	Rss2goCmd.AddCommand(MakeCmdSubscribeUser())
	Rss2goCmd.AddCommand(MakeCmdUnsubscribeUser())
	Rss2goCmd.AddCommand(MakeCmdListUsers())

	//Other Commands
	Rss2goCmd.AddCommand(MakeCmdDaemon())
	Rss2goCmd.AddCommand(MakeCmdRunOne())

	Rss2goCmd.AddCommand(MakeCmdExportOPML())
	Rss2goCmd.AddCommand(MakeCmdImportOpml())
	Rss2goCmd.AddCommand(MakeCmdShowConfig())
	Rss2goCmd.AddCommand(versionCmd)
}

// Rss2goCmd is the root command, but doesn't do anything.
var Rss2goCmd = &cobra.Command{
	Use:   "rss2go",
	Short: "rss2go scrapes rss and atom feeds and emails them to you",
	Long:  "A workalike to rss2email",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of rss2go",
	Long:  `All software has versions. This is rss2go's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("rss2go v0.9 -- HEAD")
	},
}
