package config

import (
	"github.com/BurntSushi/toml"
	"os/user"
	"strings"
)

type Config struct {
	Mail      mailConfig
	Crawl     crawlConfig
	Db        dbConfig
	WebServer webConfig
}

type webConfig struct {
	ListenAddress string // eg localhost:7000 or 0.0.0.0:8000
}

type mailConfig struct {
	SendMail     bool
	UseSendmail  bool
	UseSmtp      bool
	MtaPath      string
	SmtpServer   string
	SmtpUsername string
	SmtpPassword string
	ToAddress    string
	FromAddress  string
}

type dbConfig struct {
	Path     string
	Verbose  bool // turn on verbose db logging
	UpdateDb bool // if we should update db items during crawl
}

type crawlConfig struct {
	MaxCrawlers int
	MinInterval int64 // Seconds
	MaxInterval int64 // Seconds
}
type feedsConfig struct {
	Urls []string
}

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
		},
	}
}

func replaceTildeInPath(path string) string {
	usr, _ := user.Current()
	dir := usr.HomeDir
	return strings.Replace(path, "~", dir, 1)
}

func (self *Config) ReadConfig(config_path string) error {
	_, err := toml.DecodeFile(replaceTildeInPath(config_path), &self)
	return err
}
