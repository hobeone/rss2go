package config

import (
	"fmt"
	"log/slog"
	"os"
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
	Sendmail   string `mapstructure:"sendmail"` // Path to sendmail binary

	PollInterval time.Duration `mapstructure:"poll_interval"`
	PollJitter   time.Duration `mapstructure:"poll_jitter"`

	CrawlerPoolSize int           `mapstructure:"crawler_pool_size"`
	CrawlerTimeout  time.Duration `mapstructure:"crawler_timeout"`

	MailerPoolSize int `mapstructure:"mailer_pool_size"`

	MaxImageWidth int `mapstructure:"max_image_width"`

	MetricsAddr string `mapstructure:"metrics_addr"`
}

// Load loads the configuration from file, environment variables, and flags.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigFile("./rss2go.yaml")
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
	v.SetDefault("max_image_width", 600)
	v.SetDefault("smtp_port", 587)

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

	if cfg.SMTPPass != "" {
		warnIfWorldReadable(v.ConfigFileUsed())
	}

	return &cfg, nil
}

// warnIfWorldReadable logs a warning if the config file is readable by
// group or others, which is a risk when it contains SMTP credentials.
func warnIfWorldReadable(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		slog.Warn("config file is accessible by group/others; consider chmod 600",
			"path", path,
			"permissions", fmt.Sprintf("%04o", perm),
		)
	}
}
