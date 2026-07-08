package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the fully resolved configuration values for rss2go.
type Config struct {
	DBPath       string            `yaml:"db_path"`
	Addr         string            `yaml:"addr"`
	MailerMode   string            `yaml:"mailer_mode"`
	SMTPHost     string            `yaml:"smtp_host"`
	SMTPPort     int               `yaml:"smtp_port"`
	SMTPUser     string            `yaml:"smtp_user"`
	SMTPPass     string            `yaml:"smtp_pass"`
	SMTPFrom     string            `yaml:"smtp_from"`
	SMTPSecurity string            `yaml:"smtp_security"`
	Crawlers     int               `yaml:"crawlers"`
	LogLevel     string            `yaml:"log_level"`
	LogFile      string            `yaml:"log_file"`
	LogLevels    map[string]string `yaml:"log_levels"`
	PollInterval time.Duration     `yaml:"poll_interval"`
	SidecarAddr  string            `yaml:"sidecar_addr"`
}

// Default returns a Config struct initialized with standard default parameters.
func Default() *Config {
	return &Config{
		DBPath:       "rss2go.db",
		Addr:         ":8080",
		MailerMode:   "sendmail",
		SMTPHost:     "localhost",
		SMTPPort:     587,
		SMTPUser:     "",
		SMTPPass:     "",
		SMTPFrom:     "rss2go@localhost",
		SMTPSecurity: "starttls",
		Crawlers:     4,
		LogLevel:     "info",
		LogFile:      "",
		LogLevels:    make(map[string]string),
		PollInterval: 10 * time.Second,
		SidecarAddr:  ":8081",
	}
}

// Load compiles a resolved Config mapping by parsing in layered order:
// Defaults -> YAML Config File -> Environment Variables -> CLI Flags.
func Load(args []string) (*Config, error) {
	cfg := Default()

	// 1. Determine config file path (layered check: CLI flag -> env var -> default)
	preFs := flag.NewFlagSet("pre-config", flag.ContinueOnError)
	preFs.SetOutput(io.Discard)
	preFs.Usage = func() {}
	configPathFlag := preFs.String("config", "rss2go.yaml", "")
	_ = preFs.Parse(args)

	configPath := *configPathFlag
	explicitFile := false

	if envVal, exists := os.LookupEnv("RSS2GO_CONFIG"); exists && envVal != "" {
		configPath = envVal
		explicitFile = true
	}

	preFs.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			explicitFile = true
		}
	})

	// 2. Parse YAML configuration file if it exists (or error if explicitly required but missing)
	fileInfo, statErr := os.Stat(configPath)
	if statErr == nil && !fileInfo.IsDir() {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("config: read file %q: %w", configPath, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: parse yaml %q: %w", configPath, err)
		}
		warnIfWorldReadable(configPath, cfg.SMTPPass)
	} else if explicitFile {
		return nil, fmt.Errorf("config: file not found at %q", configPath)
	}

	// 3. Layer Environment Variable Overrides
	if val, exists := os.LookupEnv("RSS2GO_DB"); exists {
		cfg.DBPath = val
	}
	if val, exists := os.LookupEnv("RSS2GO_ADDR"); exists {
		cfg.Addr = val
	}
	if val, exists := os.LookupEnv("RSS2GO_MAILER"); exists {
		cfg.MailerMode = val
	}
	if val, exists := os.LookupEnv("RSS2GO_SMTP_HOST"); exists {
		cfg.SMTPHost = val
	}
	if val, exists := os.LookupEnv("RSS2GO_SMTP_PORT"); exists {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.SMTPPort = p
		}
	}
	if val, exists := os.LookupEnv("RSS2GO_SMTP_USER"); exists {
		cfg.SMTPUser = val
	}
	if val, exists := os.LookupEnv("RSS2GO_SMTP_PASS"); exists {
		cfg.SMTPPass = val
	}
	if val, exists := os.LookupEnv("RSS2GO_SMTP_FROM"); exists {
		cfg.SMTPFrom = val
	}
	if val, exists := os.LookupEnv("RSS2GO_SMTP_SECURITY"); exists {
		cfg.SMTPSecurity = val
	}
	if val, exists := os.LookupEnv("RSS2GO_CRAWLERS"); exists {
		if c, err := strconv.Atoi(val); err == nil {
			cfg.Crawlers = c
		}
	}
	if val, exists := os.LookupEnv("RSS2GO_LOG_LEVEL"); exists {
		cfg.LogLevel = val
	}
	if val, exists := os.LookupEnv("RSS2GO_LOG_FILE"); exists {
		cfg.LogFile = val
	}
	if val, exists := os.LookupEnv("RSS2GO_LOG_LEVELS"); exists {
		cfg.LogLevels = parseLogLevelsMap(val)
	}
	if val, exists := os.LookupEnv("RSS2GO_POLL_INTERVAL"); exists {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.PollInterval = d
		}
	}
	if val, exists := os.LookupEnv("RSS2GO_SIDECAR_ADDR"); exists {
		cfg.SidecarAddr = val
	}

	// 4. Layer CLI Flag Overrides
	mainFs := flag.NewFlagSet("rss2go", flag.ContinueOnError)
	mainFs.SetOutput(io.Discard)
	mainFs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of rss2go daemon:\n")
		fmt.Fprintf(os.Stderr, "  rss2go [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		mainFs.PrintDefaults()
	}

	dbFlag := mainFs.String("db", "", "SQLite database path (default \"rss2go.db\")")
	addrFlag := mainFs.String("addr", "", "Bind address for API dashboard (default \":8080\")")
	mailerFlag := mainFs.String("mailer", "", "Outbox delivery system ('smtp', 'sendmail', or 'mock'; default \"sendmail\")")
	smtpHostFlag := mainFs.String("smtp-host", "", "SMTP server hostname (default \"localhost\")")
	smtpPortFlag := mainFs.Int("smtp-port", 0, "SMTP server port (default 587)")
	smtpUserFlag := mainFs.String("smtp-user", "", "SMTP authentication username")
	smtpPassFlag := mainFs.String("smtp-pass", "", "SMTP authentication password")
	smtpFromFlag := mainFs.String("smtp-from", "", "Sender email address (default \"rss2go@localhost\")")
	smtpSecurityFlag := mainFs.String("smtp-security", "", "SMTP security transport ('none', 'starttls', or 'ssl'; default \"starttls\")")
	crawlersFlag := mainFs.Int("crawlers", 0, "Maximum concurrent background scrapers (default 4)")
	logLevelFlag := mainFs.String("log-level", "", "Logging level ('debug', 'info', 'warn', 'error'; default \"info\")")
	logFileFlag := mainFs.String("log-file", "", "Log file path (default stderr only)")
	logLevelsFlag := mainFs.String("log-levels", "", "Per-component level overrides, comma-separated e.g. 'server:warn,scheduler:debug'")
	pollIntervalFlag := mainFs.Duration("poll-interval", 0, "Frequency of scheduled feed polling (default 10s)")
	sidecarAddrFlag := mainFs.String("sidecar-addr", "", "Bind address for sidecar scraper server (default \":8081\")")
	_ = mainFs.String("config", "", "Configuration file path (default \"rss2go.yaml\")")

	if err := mainFs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			mainFs.SetOutput(os.Stderr)
			mainFs.Usage()
			return nil, err
		}
		return nil, fmt.Errorf("config: parse flags: %w", err)
	}

	mainFs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "db":
			cfg.DBPath = *dbFlag
		case "addr":
			cfg.Addr = *addrFlag
		case "mailer":
			cfg.MailerMode = *mailerFlag
		case "smtp-host":
			cfg.SMTPHost = *smtpHostFlag
		case "smtp-port":
			cfg.SMTPPort = *smtpPortFlag
		case "smtp-user":
			cfg.SMTPUser = *smtpUserFlag
		case "smtp-pass":
			cfg.SMTPPass = *smtpPassFlag
		case "smtp-from":
			cfg.SMTPFrom = *smtpFromFlag
		case "smtp-security":
			cfg.SMTPSecurity = *smtpSecurityFlag
		case "crawlers":
			cfg.Crawlers = *crawlersFlag
		case "log-level":
			cfg.LogLevel = *logLevelFlag
		case "log-file":
			cfg.LogFile = *logFileFlag
		case "log-levels":
			cfg.LogLevels = parseLogLevelsMap(*logLevelsFlag)
		case "poll-interval":
			cfg.PollInterval = *pollIntervalFlag
		case "sidecar-addr":
			cfg.SidecarAddr = *sidecarAddrFlag
		}
	})

	// 5. Validation Assertions
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: validation failed: %w", err)
	}

	return cfg, nil
}

// Validate asserts that the parsed configuration parameters are valid.
func (c *Config) Validate() error {
	if c.DBPath == "" {
		return fmt.Errorf("db_path cannot be empty")
	}
	if c.Addr == "" {
		return fmt.Errorf("addr cannot be empty")
	}
	if c.SidecarAddr == "" {
		return fmt.Errorf("sidecar_addr cannot be empty")
	}
	if c.MailerMode != "smtp" && c.MailerMode != "sendmail" && c.MailerMode != "mock" {
		return fmt.Errorf("invalid mailer_mode: %q (must be 'smtp', 'sendmail', or 'mock')", c.MailerMode)
	}
	if c.SMTPSecurity != "none" && c.SMTPSecurity != "starttls" && c.SMTPSecurity != "ssl" {
		return fmt.Errorf("invalid smtp_security: %q (must be 'none', 'starttls', or 'ssl')", c.SMTPSecurity)
	}
	if c.Crawlers <= 0 {
		return fmt.Errorf("crawlers must be greater than 0")
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be greater than 0")
	}
	if err := validateLogLevel(c.LogLevel); err != nil {
		return fmt.Errorf("invalid log_level: %w", err)
	}
	for comp, lvl := range c.LogLevels {
		if comp == "" {
			return fmt.Errorf("log_levels component name cannot be empty")
		}
		if lvl == "" {
			return fmt.Errorf("log_levels level override for component %q cannot be empty", comp)
		}
		if err := validateLogLevel(lvl); err != nil {
			return fmt.Errorf("invalid log_level override for component %q: %w", comp, err)
		}
	}
	return nil
}

func validateLogLevel(lvl string) error {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug", "info", "warn", "warning", "error", "off", "":
		return nil
	default:
		return fmt.Errorf("invalid log level: %q", lvl)
	}
}

func parseLogLevelsMap(s string) map[string]string {
	if s == "" {
		return nil
	}
	m := make(map[string]string)
	parts := strings.SplitSeq(s, ",")
	for part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var key, val string
		if idx := strings.IndexAny(part, ":="); idx != -1 {
			key = strings.TrimSpace(part[:idx])
			val = strings.TrimSpace(part[idx+1:])
		} else {
			key = part
			val = ""
		}
		m[key] = val
	}
	return m
}

// warnIfWorldReadable prints a warning if database or mail secrets are exposed to world reads.
func warnIfWorldReadable(path string, hasSMTPPass string) {
	if hasSMTPPass == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "WARNING: config file %q is accessible by group/others; consider chmod 600\n", path)
	}
}
