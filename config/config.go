package config

import "github.com/BurntSushi/toml"

type Config struct {
	Mail  mailConfig
	Crawl crawlConfig
	Db    dbConfig
	WebServer webConfig
}

type webConfig struct {
	ListenAddress string // eg localhost:7000 or 0.0.0.0:8000
}

type mailConfig struct {
	SendNoMail   bool
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
	Path string
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

func NewConfig() Config {
	return Config{
		Mail: mailConfig{
			UseSendmail: true,
			UseSmtp:     false,
			FromAddress: "rss2go@localhost.localdomain",
			SendNoMail:  false,
		},
		Crawl: crawlConfig{
			MaxCrawlers: 10,
			MinInterval: 300,
			MaxInterval: 86400,
		},
		WebServer: webConfig {
			ListenAddress: "localhost:7000",
		},
	}
}

func (self *Config) ReadConfig(config_path string) error {
	_, err := toml.DecodeFile(config_path, &self)
	return err
}
