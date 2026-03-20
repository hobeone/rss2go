package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds the application configuration.
type Config struct {
	DBPath     string `mapstructure:"db_path"`
	LogLevel   string `mapstructure:"log_level"`
	SMTPServer string `mapstructure:"smtp_server"`
	SMTPPort   int    `mapstructure:"smtp_port"`
	SMTPUser   string `mapstructure:"smtp_user"`
	SMTPPass   string `mapstructure:"smtp_pass"`
	SMTPSender string `mapstructure:"smtp_sender"`
	UseTLS     bool   `mapstructure:"use_tls"`
	Sendmail   string `mapstructure:"sendmail"` // Path to sendmail binary

	PollInterval time.Duration `mapstructure:"poll_interval"`
	PollJitter   time.Duration `mapstructure:"poll_jitter"`

	CrawlerPoolSize int           `mapstructure:"crawler_pool_size"`
	CrawlerTimeout  time.Duration `mapstructure:"crawler_timeout"`

	MailerPoolSize int `mapstructure:"mailer_pool_size"`

	MetricsAddr string `mapstructure:"metrics_addr"`
}

// Load loads the configuration from file, environment variables, and flags.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.AddConfigPath(".")
		v.SetConfigName("rss2go")
		v.SetConfigType("yaml")
	}

	v.SetEnvPrefix("RSS2GO")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults
	v.SetDefault("db_path", "rss2go.db")
	v.SetDefault("log_level", "info")
	v.SetDefault("poll_interval", "1h")
	v.SetDefault("poll_jitter", "5m")
	v.SetDefault("crawler_pool_size", 5)
	v.SetDefault("crawler_timeout", "30s")
	v.SetDefault("mailer_pool_size", 2)
	v.SetDefault("smtp_port", 587)
	v.SetDefault("use_tls", true)

	if err := v.ReadInConfig(); err != nil {
		// If a config file was explicitly provided but not found, return error
		if cfgFile != "" {
			return nil, fmt.Errorf("error reading config file %s: %w", cfgFile, err)
		}
		// If using default search paths, and no file is found, we should still error as requested
		return nil, fmt.Errorf("error reading default config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &cfg, nil
}
