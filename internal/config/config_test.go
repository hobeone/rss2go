package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfig_DefaultValues(t *testing.T) {
	cfg, err := Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error loading defaults: %v", err)
	}

	if cfg.DBPath != "rss2go.db" {
		t.Errorf("expected DBPath 'rss2go.db', got %q", cfg.DBPath)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("expected Addr ':8080', got %q", cfg.Addr)
	}
	if cfg.Crawlers != 4 {
		t.Errorf("expected Crawlers 4, got %d", cfg.Crawlers)
	}
	if cfg.PollInterval != 10*time.Second {
		t.Errorf("expected PollInterval 10s, got %v", cfg.PollInterval)
	}
}

func TestConfig_YAMLOverlay(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	yamlContent := `
db_path: "/tmp/rss2go_test.db"
addr: ":9999"
mailer_mode: "smtp"
smtp_port: 465
crawlers: 12
poll_interval: 1m
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("failed to write test YAML config: %v", err)
	}

	cfg, err := Load([]string{"-config", configPath})
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	if cfg.DBPath != "/tmp/rss2go_test.db" {
		t.Errorf("expected DBPath '/tmp/rss2go_test.db', got %q", cfg.DBPath)
	}
	if cfg.Addr != ":9999" {
		t.Errorf("expected Addr ':9999', got %q", cfg.Addr)
	}
	if cfg.MailerMode != "smtp" {
		t.Errorf("expected MailerMode 'smtp', got %q", cfg.MailerMode)
	}
	if cfg.SMTPPort != 465 {
		t.Errorf("expected SMTPPort 465, got %d", cfg.SMTPPort)
	}
	if cfg.Crawlers != 12 {
		t.Errorf("expected Crawlers 12, got %d", cfg.Crawlers)
	}
	if cfg.PollInterval != 1*time.Minute {
		t.Errorf("expected PollInterval 1m, got %v", cfg.PollInterval)
	}
}

func TestConfig_EnvOverlay(t *testing.T) {
	t.Setenv("RSS2GO_DB", "/env/path.db")
	t.Setenv("RSS2GO_ADDR", ":7777")
	t.Setenv("RSS2GO_CRAWLERS", "9")

	cfg, err := Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DBPath != "/env/path.db" {
		t.Errorf("expected DBPath '/env/path.db', got %q", cfg.DBPath)
	}
	if cfg.Addr != ":7777" {
		t.Errorf("expected Addr ':7777', got %q", cfg.Addr)
	}
	if cfg.Crawlers != 9 {
		t.Errorf("expected Crawlers 9, got %d", cfg.Crawlers)
	}
}

func TestConfig_CLIOverlay(t *testing.T) {
	t.Setenv("RSS2GO_DB", "/env/path.db")

	cfg, err := Load([]string{
		"-db", "/cli/path.db",
		"-addr", ":1234",
		"-crawlers", "15",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CLI flag should override env var
	if cfg.DBPath != "/cli/path.db" {
		t.Errorf("expected DBPath '/cli/path.db', got %q", cfg.DBPath)
	}
	if cfg.Addr != ":1234" {
		t.Errorf("expected Addr ':1234', got %q", cfg.Addr)
	}
	if cfg.Crawlers != 15 {
		t.Errorf("expected Crawlers 15, got %d", cfg.Crawlers)
	}
}

func TestConfig_ExplicitFileMissing(t *testing.T) {
	_, err := Load([]string{"-config", "nonexistent_config_file.yaml"})
	if err == nil {
		t.Fatalf("expected error loading nonexistent config file, got nil")
	}
}

func TestConfig_ValidationErrors(t *testing.T) {
	_, err := Load([]string{"-mailer", "invalid_mode"})
	if err == nil {
		t.Errorf("expected validation error for invalid mailer_mode, got nil")
	}

	_, err = Load([]string{"-crawlers", "-1"})
	if err == nil {
		t.Errorf("expected validation error for negative crawlers, got nil")
	}

	_, err = Load([]string{"-crawlers", "0"})
	if err == nil {
		t.Errorf("expected validation error for 0 crawlers, got nil")
	}

	_, err = Load([]string{"-poll-interval", "0s"})
	if err == nil {
		t.Errorf("expected validation error for 0s poll interval, got nil")
	}

	_, err = Load([]string{"-smtp-security", "invalid_security"})
	if err == nil {
		t.Errorf("expected validation error for invalid smtp-security, got nil")
	}
}

func TestConfig_InvalidEnvFallback(t *testing.T) {
	t.Setenv("RSS2GO_SMTP_PORT", "not_a_number")
	t.Setenv("RSS2GO_CRAWLERS", "not_a_number")
	t.Setenv("RSS2GO_POLL_INTERVAL", "not_a_duration")

	cfg, err := Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error loading with invalid env: %v", err)
	}

	if cfg.SMTPPort != 587 {
		t.Errorf("expected SMTPPort 587, got %d", cfg.SMTPPort)
	}
	if cfg.Crawlers != 4 {
		t.Errorf("expected Crawlers 4, got %d", cfg.Crawlers)
	}
	if cfg.PollInterval != 10*time.Second {
		t.Errorf("expected PollInterval 10s, got %v", cfg.PollInterval)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	fn()
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestConfig_WarnIfWorldReadable(t *testing.T) {
	tmpDir := t.TempDir()
	configPathSecure := filepath.Join(tmpDir, "secure.yaml")
	configPathInsecure := filepath.Join(tmpDir, "insecure.yaml")

	yamlSecure := "smtp_pass: \"secret\"\n"
	yamlInsecure := "smtp_pass: \"secret\"\n"

	if err := os.WriteFile(configPathSecure, []byte(yamlSecure), 0o600); err != nil {
		t.Fatalf("failed to write secure config: %v", err)
	}
	if err := os.WriteFile(configPathInsecure, []byte(yamlInsecure), 0o644); err != nil {
		t.Fatalf("failed to write insecure config: %v", err)
	}

	// 1. Secure configuration (0600) -> No warning
	output := captureStderr(t, func() {
		_, err := Load([]string{"-config", configPathSecure})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if output != "" {
		t.Errorf("expected no warning for secure config, got %q", output)
	}

	// 2. Insecure configuration (0644) -> WARNING expected
	output = captureStderr(t, func() {
		_, err := Load([]string{"-config", configPathInsecure})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(output, "WARNING:") {
		t.Errorf("expected warning for insecure config, got %q", output)
	}

	// 3. Password empty -> No warning
	yamlNoPass := "smtp_pass: \"\"\n"
	configPathNoPass := filepath.Join(tmpDir, "nopass.yaml")
	if err := os.WriteFile(configPathNoPass, []byte(yamlNoPass), 0o644); err != nil {
		t.Fatalf("failed to write nopass config: %v", err)
	}
	output = captureStderr(t, func() {
		_, err := Load([]string{"-config", configPathNoPass})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if output != "" {
		t.Errorf("expected no warning for empty password, got %q", output)
	}

	// 4. Nonexistent file path stat check -> No warning (handled inside os.Stat logic fallback)
	output = captureStderr(t, func() {
		warnIfWorldReadable(filepath.Join(tmpDir, "nonexistent.yaml"), "secret")
	})
	if output != "" {
		t.Errorf("expected no warning for nonexistent file, got %q", output)
	}
}

func TestConfig_EmptyEnvConfig(t *testing.T) {
	t.Setenv("RSS2GO_CONFIG", "")
	cfg, err := Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBPath != "rss2go.db" {
		t.Errorf("expected DBPath 'rss2go.db', got %q", cfg.DBPath)
	}
}

func TestConfig_LogSettings(t *testing.T) {
	// 1. Check parsing of log_level, log_file, and log_levels map in YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_log_config.yaml")
	yamlContent := `
log_level: "debug"
log_file: "/var/log/rss2go.log"
log_levels:
  server: "warn"
  scheduler: "info"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("failed to write test YAML config: %v", err)
	}

	cfg, err := Load([]string{"-config", configPath})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel 'debug', got %q", cfg.LogLevel)
	}
	if cfg.LogFile != "/var/log/rss2go.log" {
		t.Errorf("expected LogFile '/var/log/rss2go.log', got %q", cfg.LogFile)
	}
	if len(cfg.LogLevels) != 2 || cfg.LogLevels["server"] != "warn" || cfg.LogLevels["scheduler"] != "info" {
		t.Errorf("unexpected LogLevels: %v", cfg.LogLevels)
	}

	// 2. Check override via environment variables
	t.Setenv("RSS2GO_LOG_LEVEL", "warn")
	t.Setenv("RSS2GO_LOG_FILE", "/env/rss2go.log")
	t.Setenv("RSS2GO_LOG_LEVELS", "server:error,crawler:debug")

	cfg, err = Load([]string{"-config", configPath})
	if err != nil {
		t.Fatalf("failed to load config with env: %v", err)
	}

	if cfg.LogLevel != "warn" {
		t.Errorf("expected LogLevel 'warn' from env, got %q", cfg.LogLevel)
	}
	if cfg.LogFile != "/env/rss2go.log" {
		t.Errorf("expected LogFile '/env/rss2go.log' from env, got %q", cfg.LogFile)
	}
	if len(cfg.LogLevels) != 2 || cfg.LogLevels["server"] != "error" || cfg.LogLevels["crawler"] != "debug" {
		t.Errorf("unexpected LogLevels from env: %v", cfg.LogLevels)
	}

	// 3. Check override via CLI flags
	cfg, err = Load([]string{
		"-config", configPath,
		"-log-level", "error",
		"-log-file", "/cli/rss2go.log",
		"-log-levels", "server:off,crawler:info",
	})
	if err != nil {
		t.Fatalf("failed to load config with flags: %v", err)
	}

	if cfg.LogLevel != "error" {
		t.Errorf("expected LogLevel 'error' from CLI, got %q", cfg.LogLevel)
	}
	if cfg.LogFile != "/cli/rss2go.log" {
		t.Errorf("expected LogFile '/cli/rss2go.log' from CLI, got %q", cfg.LogFile)
	}
	if len(cfg.LogLevels) != 2 || cfg.LogLevels["server"] != "off" || cfg.LogLevels["crawler"] != "info" {
		t.Errorf("unexpected LogLevels from CLI: %v", cfg.LogLevels)
	}
}

func TestConfig_InvalidLogSettings(t *testing.T) {
	_, err := Load([]string{"-log-level", "invalid"})
	if err == nil {
		t.Errorf("expected validation error for invalid log-level, got nil")
	}

	_, err = Load([]string{"-log-levels", "server:invalid"})
	if err == nil {
		t.Errorf("expected validation error for invalid component override log level, got nil")
	}

	_, err = Load([]string{"-log-levels", ":warn"})
	if err == nil {
		t.Errorf("expected validation error for empty component name in log-levels, got nil")
	}
}
