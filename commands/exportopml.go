package commands

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/opml"
	"github.com/spf13/cobra"
)

// ExportOPMLCommand encapsulates the functionality for exporting a users feeds
// into an OPML doc.
type exportOPMLCommand struct {
	Config *config.Config
	DBH    *db.Handle
}

// newExportOPMLCommand returns a pointer to a newly created exportOPMLCommand
// struct with defaults set.
func newExportOPMLCommand(cfg *config.Config) *exportOPMLCommand {
	var dbh *db.Handle
	if cfg.Db.Type == "memory" {
		dbh = db.NewMemoryDBHandle(cfg.Db.Verbose, cfg.Db.UpdateDb)
	} else {
		dbh = db.NewDBHandle(cfg.Db.Path, cfg.Db.Verbose, cfg.Db.UpdateDb)
	}

	return &exportOPMLCommand{
		Config: cfg,
		DBH:    dbh,
	}
}

func (exporter *exportOPMLCommand) ExportOPML(userName, fileName string) {
	user, err := exporter.DBH.GetUserByEmail(userName)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}

	fmt.Printf("Found user %s, getting feeds...\n", userName)

	feeds, err := exporter.DBH.GetUsersFeeds(user)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}

	op := opml.Opml{
		Version: "1.0",
		Title:   "rss2go Export",
	}

	for _, feed := range feeds {
		o := &opml.OpmlOutline{
			Title:   feed.Name,
			Text:    feed.Name,
			XmlUrl:  feed.URL,
			HtmlUrl: feed.URL,
		}
		op.Outline = append(op.Outline, o)
	}
	b, err := xml.MarshalIndent(op, " ", " ")
	if err != nil {
		PrintErrorAndExit(err.Error())
	}

	fmt.Printf("Found %d feeds.\n", len(feeds))
	fmt.Printf("Writing opml to %s\n", fileName)

	err = ioutil.WriteFile(fileName, b, 0644)
	if err != nil {
		PrintErrorAndExit(err.Error())
	}
}

// MakeCmdExportOPML returns a Command that will export a users feeds to an
// OPML file.
func MakeCmdExportOPML() *cobra.Command {
	cmd := &cobra.Command{
		Run:   runExportOPML,
		Use:   "exportopml user@email opmlfile",
		Short: "Export all feeds from an opml file.",
		Long: `
		Export all a users feeds from a given OPML file.

		Example:
		exportopml user@email feeds.opml
		`,
	}
	return cmd
}

func runExportOPML(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		PrintErrorAndExit("Must give username and filename")
	}
	userName := args[0]
	fileName := args[1]

	cfg := loadConfig(ConfigFile)
	expCommand := newExportOPMLCommand(cfg)
	expCommand.ExportOPML(userName, fileName)
}
