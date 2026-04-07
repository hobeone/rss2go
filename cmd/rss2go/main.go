package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/hobeone/rss2go"
	"github.com/hobeone/rss2go/internal/config"
	"github.com/hobeone/rss2go/internal/db/sqlite"
	"github.com/hobeone/rss2go/internal/version"
	"github.com/spf13/cobra"
)

var (
	cfgFile  string
	logLevel string
	rootCmd  = &cobra.Command{
		Use:   "rss2go",
		Short: "rss2go is an RSS-to-Email daemon",
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./rss2go.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "override log level (debug, info, warn, error)")
}

func main() {
	fmt.Println(version.Info())
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getLogger(cfg *config.Config) *slog.Logger {
	levelStr := cfg.LogLevel
	if logLevel != "" {
		levelStr = logLevel
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

func getStore(cfg *config.Config, logger *slog.Logger) (*sqlite.Store, error) {
	store, err := sqlite.New(cfg.DBPath, logger)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(rss2go.MigrationsFS); err != nil {
		return nil, err
	}
	return store, nil
}

// setup loads config, initialises the logger, and opens the SQLite store.
// Callers must defer store.Close() on success.
func setup() (*config.Config, *slog.Logger, *sqlite.Store, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, nil, nil, err
	}
	logger := getLogger(cfg)
	store, err := getStore(cfg, logger)
	if err != nil {
		return nil, nil, nil, err
	}
	return cfg, logger, store, nil
}

