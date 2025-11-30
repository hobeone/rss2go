package log

import (
	"io"
	"log/slog"
	"os"
)

// SetupLogger configures the default slog logger.
func SetupLogger(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// SetNullOutput sets the default logger to discard everything.
// useful when running unittests.
func SetNullOutput() {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	slog.SetDefault(logger)
}
