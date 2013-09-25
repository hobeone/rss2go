package config

import "github.com/BurntSushi/toml"

type Config struct {
	Mail  mailConfig
	Crawl crawlConfig
	Db    dbConfig
}

type mailConfig struct {
	Sendmail     bool
	Smtp         bool
	SmtpServer   string
	SmtpUsername string
	SmtpPassword string
	ToAddress    string
	FromAddress  string
}

type dbConfig struct {
	Path string
}

type crawlConfig struct {
	MaxCrawlers int
	MinInterval int // Seconds
	MaxInterval int // Seconds
}
type feedsConfig struct {
	Urls []string
}

func NewConfig() Config {
	var c = Config{
		Mail: mailConfig{
			Smtp:        true,
			FromAddress: "rss2go@localhost.localdomain",
		},
		Crawl: crawlConfig{
			MaxCrawlers: 10,
			MinInterval: 300,
			MaxInterval: 86400,
		},
	}
	return c
}

func (self *Config) ReadConfig(config_path string) error {
	_, err := toml.DecodeFile(config_path, self)
	return err
}
