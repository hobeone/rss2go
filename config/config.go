package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// Config is the base struct for Rss2Go configuration information.
type Config struct {
	Mail           mailConfig
	Crawl          crawlConfig
	DB             dbConfig
	WebServer      webConfig
	ReportInterval int64
}

type webConfig struct {
	ListenAddress string // eg localhost:7000 or 0.0.0.0:8000
	EnableAPI     bool
}

type mailConfig struct {
	SendMail    bool
	UseSendmail bool
	UseSMTP     bool
	MtaPath     string
	Hostname    string
	Port        int
	Username    string
	Password    string
	FromAddress string
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

// NewConfig returns a Config struct with reasonable defaults set.
func NewConfig() *Config {
	return &Config{
		Mail: mailConfig{
			UseSendmail: true,
			UseSMTP:     false,
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
		DB: dbConfig{
			Verbose:       true,
			UpdateDb:      true,
			WatchInterval: 60,
			Type:          "file",
		},
		ReportInterval: 60 * 60 * 24 * 7, // 7 days
	}
}

// NewTestConfig returns a Config instance suitable for use in testing.
func NewTestConfig() *Config {
	c := NewConfig()
	c.Mail.UseSendmail = false
	c.Mail.UseSMTP = false
	c.Mail.SendMail = false
	c.Mail.FromAddress = "rss2go@example.com"
	c.DB.Type = "memory"
	c.DB.Verbose = false
	return c
}

// currentUserFunc is a variable that holds the function to get the current user.
// This allows mocking in tests.
var currentUserFunc = user.Current

func replaceTildeInPath(path string) string {
	usr, err := currentUserFunc()
	if err != nil {
		// If we can't get the current user, we can't replace ~
		// Return path unmodified in this case, or handle error as appropriate
		return path
	}
	dir := usr.HomeDir
	if !strings.HasPrefix(path, "~/") && path != "~" {
		return path
	}
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

	filecont, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("Failed reading config: %s", err)
	}

	if err = json.Unmarshal(filecont, c); err != nil {
		extra := ""
		if serr, ok := err.(*json.SyntaxError); ok {
			// Create a new reader for highlightBytePosition as f's cursor is at EOF
			r := bytes.NewReader(filecont)
			line, col, highlight := highlightBytePosition(r, serr.Offset)
			extra = fmt.Sprintf(":\nError at line %d, column %d (file offset %d):\n%s",
				line, col, serr.Offset, highlight)
		}
		return fmt.Errorf("error parsing JSON object in config file %s%s\n%v",
			absConfigPath, extra, err) // Use absConfigPath for error message
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
	currentColInLoop := 0 // 0-indexed for internal loop logic for calculating arrow offset

	br := bufio.NewReader(f)
	lastLine := ""
	thisLine := new(bytes.Buffer) // Holds content of the error line *before* pos

	for n := int64(0); n < pos; n++ {
		b, err := br.ReadByte()
		if err != nil {
			break // EOF or other read error before reaching pos
		}
		if b == '\n' {
			lastLine = thisLine.String()
			thisLine.Reset()
			line++
			currentColInLoop = 0
		} else {
			thisLine.WriteByte(b)
			currentColInLoop++
		}
	}
	// 'currentColInLoop' is now the 0-indexed column of char at pos on its line.
	// Or, if pos is end of line, it's length of line.

	// Convert to 1-based 'col' for the return value, as column numbers are typically 1-based.
	col = currentColInLoop + 1

	// 'thisLine' has content before 'pos'. Read 'restOfLineReader' to get content from 'pos' onwards for the current line.
	restOfLineReader := bufio.NewReader(br) // Use the existing buffered reader
    actualRestOfLineBytes, err := restOfLineReader.ReadBytes('\n')
    var actualRestOfLine string
    if err != nil && err != io.EOF {
        // Handle error or decide how to proceed if reading rest of line fails
        actualRestOfLine = ""
    } else {
		actualRestOfLine = string(bytes.TrimRight(actualRestOfLineBytes, "\n"))
	}
	
	fullErrorLine := thisLine.String() + actualRestOfLine

	if line > 1 {
		highlight += fmt.Sprintf("%5d: %s\n", line-1, lastLine)
	}
	highlight += fmt.Sprintf("%5d: %s\n", line, fullErrorLine)
	// Arrow position: 'currentColInLoop' is the 0-indexed offset on the line for the char at 'pos'.
	// The prefix "%5d: " (e.g., "    1: ") has a length of 7.
	// So, we need 'currentColInLoop' spaces relative to the start of 'fullErrorLine', plus 7 for the prefix.
	highlight += fmt.Sprintf("%s^\n", strings.Repeat(" ", currentColInLoop + 7)) // This was actually correct. The issue is in test expectations.
	return
}
