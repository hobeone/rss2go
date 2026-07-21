package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"rss2go/internal/config"
	"rss2go/internal/crawler"
	"rss2go/internal/database"
	"rss2go/internal/extractor"
	"rss2go/internal/logger"
	"rss2go/internal/notifier"
	"rss2go/internal/outbox"
	"rss2go/internal/sanitizer"
	"rss2go/internal/scheduler"
	"rss2go/internal/server"
	"rss2go/internal/sidecar"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	isSidecar, cmdAddr, globalArgs := parseSidecarArgs(os.Args[1:])

	// Load resolved configuration via YAML config file, env variables, and flags
	cfg, err := config.Load(globalArgs)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(2)
	}

	if isSidecar {
		if cmdAddr != "" {
			cfg.SidecarAddr = cmdAddr
		}

		logLvl, err := logger.ParseLevel(cfg.LogLevel)
		if err != nil {
			logLvl = slog.LevelInfo
		}
		compLvls := make(map[string]slog.Level)
		for comp, lvlStr := range cfg.LogLevels {
			if lvl, err := logger.ParseLevel(lvlStr); err == nil {
				compLvls[comp] = lvl
			}
		}

		_, logCloser, err := logger.Setup(logger.LoggingOptions{
			Level:           logLvl,
			LogFile:         cfg.LogFile,
			ComponentLevels: compLvls,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error setting up sidecar logger: %v\n", err)
			os.Exit(1)
		}
		if logCloser != nil {
			defer func() {
				_ = logCloser.Close()
			}()
		}

		slog.Info("Starting rss2go scraper sidecar", "addr", cfg.SidecarAddr, "version", Version, "commit", Commit, "date", Date)

		srv := sidecar.NewServer(cfg.SidecarAddr, nil, slog.Default().With("component", "sidecar"))

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		if err := srv.Start(ctx); err != nil {
			slog.Error("Scraper Sidecar crashed", "err", err)
			os.Exit(1)
		}
		slog.Info("Scraper sidecar shutdown complete")
		return
	}

	// 1. Initialize log broadcaster and unified logger
	broadcaster := server.NewLogBroadcaster()
	consoleWriter := server.NewSSEWriter(broadcaster, os.Stderr)

	logLvl, err := logger.ParseLevel(cfg.LogLevel)
	if err != nil {
		logLvl = slog.LevelInfo
	}
	compLvls := make(map[string]slog.Level)
	for comp, lvlStr := range cfg.LogLevels {
		if lvl, err := logger.ParseLevel(lvlStr); err == nil {
			compLvls[comp] = lvl
		}
	}

	_, logCloser, err := logger.Setup(logger.LoggingOptions{
		Level:           logLvl,
		LogFile:         cfg.LogFile,
		ComponentLevels: compLvls,
		ConsoleWriter:   consoleWriter,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	if logCloser != nil {
		defer func() {
			_ = logCloser.Close()
		}()
	}

	slog.Info("Starting rss2go aggregator daemon", "version", Version, "commit", Commit, "date", Date)

	// 1. Initialize SQLite database
	slog.Info("Opening SQLite database connection", "path", cfg.DBPath)
	db, err := database.Open(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to open SQLite database", "path", cfg.DBPath, "err", err)
		os.Exit(1)
	}
	defer func() {
		slog.Info("Closing database connection")
		_ = db.Close()
	}()

	repo := database.NewRepository(db)

	// 2. Initialize crawler, extractor, and sanitizer
	cr := crawler.NewCrawler(nil, slog.Default().With("component", "crawler"))
	ex := extractor.NewExtractor(nil, slog.Default().With("component", "extractor"))
	sa := sanitizer.NewSanitizer(800) // Default 800px width limit for emails

	// 3. Initialize mail delivery notifier
	var delivery notifier.Sender
	switch cfg.MailerMode {
	case "smtp":
		var sec notifier.SecurityType
		switch cfg.SMTPSecurity {
		case "ssl":
			sec = notifier.SecuritySSL
		case "none":
			sec = notifier.SecurityNone
		default:
			sec = notifier.SecuritySTARTTLS
		}
		slog.Info("Configuring SMTP mailer notifier", "host", cfg.SMTPHost, "port", cfg.SMTPPort, "from", cfg.SMTPFrom, "security", sec)
		delivery = notifier.NewSMTPSender(notifier.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUser,
			Password: cfg.SMTPPass,
			From:     cfg.SMTPFrom,
			Security: sec,
		}, slog.Default().With("component", "notifier"))
	case "sendmail":
		slog.Info("Configuring sendmail binary notifier", "from", cfg.SMTPFrom)
		delivery = notifier.NewSendmailSender("", cfg.SMTPFrom, slog.Default().With("component", "notifier"))
	case "mock":
		slog.Info("Configuring dry-run mock mailer (logs only)")
		delivery = &mockNotifier{}
	}

	// 4. Initialize Outbox Worker queue
	slog.Info("Starting background outbox worker queue")
	worker := outbox.NewQueue(repo, delivery, outbox.Config{
		MaxRetries:     5,
		InitialBackoff: 5 * time.Minute,
	}, slog.Default().With("component", "outbox"))

	// 5. Initialize Scheduler
	slog.Info("Starting polling scheduler", "max_workers", cfg.Crawlers, "interval", cfg.PollInterval)
	sched := scheduler.New(repo, cr, ex, sa, scheduler.Config{
		MaxWorkers:   cfg.Crawlers,
		PollInterval: cfg.PollInterval,
	}, slog.Default().With("component", "scheduler"))

	// 6. Initialize HTTP Server
	slog.Info("Configuring API server", "addr", cfg.Addr)
	srv := server.New(repo, sched, cr, ex, sa, server.Config{
		Addr:        cfg.Addr,
		Broadcaster: broadcaster,
		MailerMode:  cfg.MailerMode,
	}, slog.Default().With("component", "api"))

	// Graceful signal listener context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Launch Outbox worker
	go func() {
		_ = worker.Start(ctx)
		slog.Info("Outbox worker queue stopped")
	}()

	// Launch Aggregator scheduler
	go func() {
		_ = sched.Start(ctx)
		slog.Info("Polling scheduler stopped")
	}()

	// Launch HTTP Server (blocks until context is done)
	if err := srv.Start(ctx); err != nil {
		slog.Error("API Server crashed", "err", err)
		os.Exit(1)
	}

	slog.Info("rss2go daemon shutdown complete")
}

type mockNotifier struct{}

func (m *mockNotifier) Send(ctx context.Context, subject string, body string, recipients []string) error {
	slog.Info("[MOCK MAIL] Sending notification", "recipients", recipients, "subject", subject, "body_len", len(body))
	return nil
}

func parseSidecarArgs(args []string) (bool, string, []string) {
	if len(args) == 0 {
		return false, "", args
	}
	if args[0] != "sidecar" {
		return false, "", args
	}

	var sidecarAddr string
	var globalArgs []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-addr" || arg == "--addr" {
			if i+1 < len(args) {
				sidecarAddr = args[i+1]
				i++
			}
		} else if after, ok := strings.CutPrefix(arg, "-addr="); ok {
			sidecarAddr = after
		} else if after, ok := strings.CutPrefix(arg, "--addr="); ok {
			sidecarAddr = after
		} else {
			globalArgs = append(globalArgs, arg)
		}
	}
	return true, sidecarAddr, globalArgs
}
