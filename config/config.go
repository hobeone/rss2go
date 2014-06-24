package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

//Config is the base struct for Rss2Go configuration information.
type Config struct {
	Mail      mailConfig
	Crawl     crawlConfig
	Db        dbConfig
	WebServer webConfig
}

type webConfig struct {
	ListenAddress string // eg localhost:7000 or 0.0.0.0:8000
	EnableAPI     bool
}

type mailConfig struct {
	SendMail     bool
	UseSendmail  bool
	UseSmtp      bool
	MtaPath      string
	SmtpServer   string
	SmtpUsername string
	SmtpPassword string
	FromAddress  string
}

type dbConfig struct {
	Path          string
	Verbose       bool   // turn on verbose db logging
	UpdateDb      bool   // if we should update db items during crawl
	WatchInterval int64  // how often to check for new or deleted feeds
	Type          string // file or memory (for testing)
}

type crawlConfig struct {
	MaxCrawlers int
	MinInterval int64 // Seconds
	MaxInterval int64 // Seconds
}
type feedsConfig struct {
	Urls []string
}

// NewConfig returns a Config struct with reasonable defaults set.
func NewConfig() *Config {
	return &Config{
		Mail: mailConfig{
			UseSendmail: true,
			UseSmtp:     false,
			FromAddress: "rss2go@localhost.localdomain",
			SendMail:    true,
		},
		Crawl: crawlConfig{
			MaxCrawlers: 10,
			MinInterval: 300,
			MaxInterval: 86400,
		},
		WebServer: webConfig{
			ListenAddress: "localhost:7000",
			EnableAPI:     false,
		},
		Db: dbConfig{
			Verbose:       true,
			UpdateDb:      true,
			WatchInterval: 60,
			Type:          "file",
		},
	}
}

//NewTestConfig returns a Config instance suitable for use in testing.
func NewTestConfig() *Config {
	c := NewConfig()
	c.Mail.UseSendmail = false
	c.Mail.UseSmtp = false
	c.Mail.SendMail = false
	c.Db.Type = "memory"
	c.Db.Verbose = false
	return c
}

func replaceTildeInPath(path string) string {
	usr, _ := user.Current()
	dir := usr.HomeDir
	return strings.Replace(path, "~", dir, 1)
}

// ReadConfig Decodes and evaluates a json config file, watching for include cycles.
func (c *Config) ReadConfig(configPath string) error {
	absConfigPath, err := filepath.Abs(replaceTildeInPath(configPath))
	if err != nil {
		return fmt.Errorf("failed to expand absolute path for %s", configPath)
	}

	var f *os.File
	if f, err = os.Open(absConfigPath); err != nil {
		return fmt.Errorf("failed to open config: %v", err)
	}
	defer f.Close()

	filecont, err := ioutil.ReadAll(f)

	if err = json.Unmarshal(filecont, c); err != nil {
		extra := ""
		if serr, ok := err.(*json.SyntaxError); ok {
			if _, serr := f.Seek(0, os.SEEK_SET); serr != nil {
				fmt.Printf("seek error: %v\n", serr)
			}
			line, col, highlight := highlightBytePosition(f, serr.Offset)
			extra = fmt.Sprintf(":\nError at line %d, column %d (file offset %d):\n%s",
				line, col, serr.Offset, highlight)
		}
		return fmt.Errorf("error parsing JSON object in config file %s%s\n%v",
			f.Name(), extra, err)
	}
	return nil
}

// HighlightBytePosition takes a reader and the location in bytes of a parse
// error (for instance, from json.SyntaxError.Offset) and returns the line, column,
// and pretty-printed context around the error with an arrow indicating the exact
// position of the syntax error.
//
// Lifted from camlistore
func highlightBytePosition(f io.Reader, pos int64) (line, col int, highlight string) {
	line = 1
	br := bufio.NewReader(f)
	lastLine := ""
	thisLine := new(bytes.Buffer)
	for n := int64(0); n < pos; n++ {
		b, err := br.ReadByte()
		if err != nil {
			break
		}
		if b == '\n' {
			lastLine = thisLine.String()
			thisLine.Reset()
			line++
			col = 1
		} else {
			col++
			thisLine.WriteByte(b)
		}
	}
	if line > 1 {
		highlight += fmt.Sprintf("%5d: %s\n", line-1, lastLine)
	}
	highlight += fmt.Sprintf("%5d: %s\n", line, thisLine.String())
	highlight += fmt.Sprintf("%s^\n", strings.Repeat(" ", col+5))
	return
}
