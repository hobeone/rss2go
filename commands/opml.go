package commands

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"

	"golang.org/x/net/html/charset"

	"github.com/alecthomas/kingpin/v2"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/opml"
	"github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

type opmlCommand struct {
	Config      *config.Config
	DBH         *db.Handle
	Email       string
	File        string
	UpdateFeeds bool
}

func (oc *opmlCommand) init() {
	oc.Config, oc.DBH = commonInit()
}

func (oc *opmlCommand) configure(app *kingpin.Application) {
	opmlCmd := app.Command("opml", "import or export opml")

	export := opmlCmd.Command("export", "Export opml").Action(oc.export)
	export.Arg("email", "email of user to export").Required().StringVar(&oc.Email)
	export.Arg("outfile", "path of file to export to").Required().StringVar(&oc.File)

	importOpml := opmlCmd.Command("import", "Import opml").Action(oc.importOpml)
	importOpml.Arg("infile", "path to file to import").Required().ExistingFileVar(&oc.File)
}

func (oc *opmlCommand) export(c *kingpin.ParseContext) error {
	user, err := oc.DBH.GetUserByEmail(oc.Email)
	if err != nil {
		return err
	}

	fmt.Printf("Found user %s, getting feeds...\n", oc.Email)

	feeds, err := oc.DBH.GetUsersFeeds(user)
	if err != nil {
		return err
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
		return err
	}

	fmt.Printf("Found %d feeds.\n", len(feeds))
	fmt.Printf("Writing opml to %s\n", oc.File)

	err = os.WriteFile(oc.File, b, 0644)
	if err != nil {
		return err
	}
	return nil
}

func (oc *opmlCommand) importOpml(c *kingpin.ParseContext) error {
	oc.init()

	fr, err := os.ReadFile(oc.File)
	if err != nil {
		logrus.Fatalf("Error reading OPML file: %s", err.Error())
	}
	o := opml.Opml{}
	d := xml.NewDecoder(bytes.NewReader(fr))
	d.CharsetReader = charset.NewReaderLabel
	d.Entity = xml.HTMLEntity
	d.Strict = false

	if err := d.Decode(&o); err != nil {
		logrus.Fatalf("opml error: %v", err.Error())
	}
	feeds := make(map[string]string)
	var proc func(outlines []*opml.OpmlOutline)
	proc = func(outlines []*opml.OpmlOutline) {
		for _, o := range outlines {
			if o.XmlUrl != "" {
				feeds[o.XmlUrl] = o.Text
			}
			proc(o.Outline)
		}
	}
	proc(o.Outline)

	for k, v := range feeds {
		_, err := oc.DBH.AddFeed(v, k)
		if err != nil {
			fmt.Println(err)
			if err == sqlite3.ErrConstraint {
				fmt.Printf("Feed %s already exists in database, skipping.\n", k)
				continue
			} else {
				return err
			}
		}
		fmt.Printf("Added feed \"%s\" at url \"%s\"\n", v, k)
	}
	return nil
}
