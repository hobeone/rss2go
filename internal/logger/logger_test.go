package logger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
		err      bool
	}{
		{"debug", slog.LevelDebug, false},
		{"INFO", slog.LevelInfo, false},
		{"", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"off", LevelOff, false},
		{"invalid", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		lvl, err := ParseLevel(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("expected error for %q, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.input, err)
			}
			if lvl != tt.expected {
				t.Errorf("expected %v for %q, got %v", tt.expected, tt.input, lvl)
			}
		}
	}
}

func TestLoggerSetupAndFilter(t *testing.T) {
	var consoleBuf bytes.Buffer
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	opts := LoggingOptions{
		Level:     slog.LevelInfo,
		LogFile:   logFile,
		AddSource: false,
		ComponentLevels: map[string]slog.Level{
			"api":         slog.LevelWarn,
			"scheduler":   slog.LevelDebug,
			"silent-comp": LevelOff,
		},
		ConsoleWriter: &consoleBuf,
	}

	logger, closer, err := Setup(opts)
	if err != nil {
		t.Fatalf("failed to setup logger: %v", err)
	}
	if closer == nil {
		t.Fatalf("expected log file closer, got nil")
	}

	// Log messages
	ctx := context.Background()

	// 1. Global level: Info (should log to console and file)
	logger.InfoContext(ctx, "global info message")
	// 2. Global level: Debug (should NOT log)
	logger.DebugContext(ctx, "global debug message")

	// 3. Component override: api (level Warn) -> Info should NOT log, Warn should log
	apiLogger := logger.With("component", "api")
	apiLogger.InfoContext(ctx, "api info message")
	apiLogger.WarnContext(ctx, "api warn message")

	// 4. Component override: scheduler (level Debug) -> Debug should log
	schedLogger := logger.With("component", "scheduler")
	schedLogger.DebugContext(ctx, "scheduler debug message")

	// 5. Component override: silent-comp (LevelOff) -> Error should NOT log
	silentLogger := logger.With("component", "silent-comp")
	silentLogger.ErrorContext(ctx, "silent error message")

	// Close log file to flush
	if err := closer.Close(); err != nil {
		t.Fatalf("failed to close log file: %v", err)
	}

	// Read outputs
	consoleOutput := consoleBuf.String()
	fileBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	fileOutput := string(fileBytes)

	// Verify console outputs (which uses tint, custom format)
	if !strings.Contains(consoleOutput, "global info message") {
		t.Errorf("expected global info message in console, got:\n%s", consoleOutput)
	}
	if strings.Contains(consoleOutput, "global debug message") {
		t.Errorf("unexpected global debug message in console, got:\n%s", consoleOutput)
	}
	if strings.Contains(consoleOutput, "api info message") {
		t.Errorf("unexpected api info message in console, got:\n%s", consoleOutput)
	}
	if !strings.Contains(consoleOutput, "api warn message") {
		t.Errorf("expected api warn message in console, got:\n%s", consoleOutput)
	}
	if !strings.Contains(consoleOutput, "scheduler debug message") {
		t.Errorf("expected scheduler debug message in console, got:\n%s", consoleOutput)
	}
	if strings.Contains(consoleOutput, "silent error message") {
		t.Errorf("unexpected silent error message in console, got:\n%s", consoleOutput)
	}

	// Verify file outputs (which uses standard slog.TextHandler)
	if !strings.Contains(fileOutput, "level=INFO msg=\"global info message\"") &&
		!strings.Contains(fileOutput, "level=INFO") {
		t.Errorf("expected global info message in file, got:\n%s", fileOutput)
	}
	if !strings.Contains(fileOutput, "level=DEBUG msg=\"scheduler debug message\"") &&
		!strings.Contains(fileOutput, "level=DEBUG") {
		t.Errorf("expected scheduler debug message in file, got:\n%s", fileOutput)
	}
}

func TestDynamicLogLevelSet(t *testing.T) {
	var consoleBuf bytes.Buffer
	opts := LoggingOptions{
		Level:         slog.LevelInfo,
		ConsoleWriter: &consoleBuf,
	}

	logger, _, err := Setup(opts)
	if err != nil {
		t.Fatalf("failed to setup logger: %v", err)
	}

	ctx := context.Background()
	logger.DebugContext(ctx, "debug msg before")
	if consoleBuf.Len() > 0 {
		t.Errorf("unexpected debug log printed: %q", consoleBuf.String())
	}

	// Dynamically change log level to debug
	SetLogLevels(slog.LevelDebug, nil)

	logger.DebugContext(ctx, "debug msg after")
	if !strings.Contains(consoleBuf.String(), "debug msg after") {
		t.Errorf("expected debug log printed after level change, got:\n%s", consoleBuf.String())
	}
}

func TestComponentHierarchyAndOverrides(t *testing.T) {
	var consoleBuf bytes.Buffer
	opts := LoggingOptions{
		Level: slog.LevelInfo,
		ComponentLevels: map[string]slog.Level{
			"api":     slog.LevelWarn,
			"crawler": slog.LevelDebug,
			"g":       slog.LevelDebug,
		},
		ConsoleWriter: &consoleBuf,
	}

	logger, _, err := Setup(opts)
	if err != nil {
		t.Fatalf("failed to setup logger: %v", err)
	}

	ctx := context.Background()

	// 1. Slash hierarchy: "api/auth" should inherit "api" level (Warn)
	// Info should NOT log. Warn SHOULD log.
	authLogger := logger.With("component", "api/auth")
	authLogger.InfoContext(ctx, "api/auth info msg")
	authLogger.WarnContext(ctx, "api/auth warn msg")

	// 1b. Multi-level slash nested component: "api/v1/auth/session" should fall back to "api" (Warn)
	// Info should NOT log. Warn SHOULD log.
	sessionLogger := logger.With("component", "api/v1/auth/session")
	sessionLogger.InfoContext(ctx, "api/v1/auth/session info msg")
	sessionLogger.WarnContext(ctx, "api/v1/auth/session warn msg")

	// 1c. Slash at index 1 hierarchy: "g/crawler" should inherit "g" level (Debug)
	// Debug SHOULD log.
	crawlerGLogger := logger.With("component", "g/crawler")
	crawlerGLogger.DebugContext(ctx, "g/crawler debug msg")

	// 2. Direct log call component override (using record attributes)
	// Record attribute crawler (Debug) -> Debug should log.
	logger.DebugContext(ctx, "record crawler debug msg", "component", "crawler")
	// Record attribute api (Warn) -> Info should NOT log.
	logger.InfoContext(ctx, "record api info msg", "component", "api")

	output := consoleBuf.String()
	if strings.Contains(output, "api/auth info msg") {
		t.Errorf("unexpected api/auth info message logged: %s", output)
	}
	if !strings.Contains(output, "api/auth warn msg") {
		t.Errorf("expected api/auth warn message logged: %s", output)
	}
	if strings.Contains(output, "api/v1/auth/session info msg") {
		t.Errorf("unexpected api/v1/auth/session info message logged: %s", output)
	}
	if !strings.Contains(output, "api/v1/auth/session warn msg") {
		t.Errorf("expected api/v1/auth/session warn message logged: %s", output)
	}
	if !strings.Contains(output, "g/crawler debug msg") {
		t.Errorf("expected g/crawler debug message logged: %s", output)
	}
	if !strings.Contains(output, "record crawler debug msg") {
		t.Errorf("expected record crawler debug message logged: %s", output)
	}
	if strings.Contains(output, "record api info msg") {
		t.Errorf("unexpected record api info message logged: %s", output)
	}
}

type errorHandler struct {
	err error
}

func (e *errorHandler) Enabled(ctx context.Context, level slog.Level) bool { return true }
func (e *errorHandler) Handle(ctx context.Context, r slog.Record) error    { return e.err }
func (e *errorHandler) WithAttrs(attrs []slog.Attr) slog.Handler           { return e }
func (e *errorHandler) WithGroup(name string) slog.Handler                 { return e }

func TestMultiHandlerError(t *testing.T) {
	importErr := fmt.Errorf("error from handler 1")
	h1 := &errorHandler{err: importErr}
	h2 := &errorHandler{err: nil}
	mh := &multiHandler{handlers: []slog.Handler{h1, h2}}

	err := mh.Handle(context.Background(), slog.Record{Level: slog.LevelInfo})
	if err == nil {
		t.Fatalf("expected error from multiHandler, got nil")
	}
	if !errors.Is(err, importErr) {
		t.Errorf("expected wrapped error %v, got %v", importErr, err)
	}
}
