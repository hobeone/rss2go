package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lmittmann/tint"
)

// LevelOff is a log level that suppresses all output for a component.
const LevelOff = slog.Level(99)

// LoggingOptions configures structured logging behavior.
type LoggingOptions struct {
	Level           slog.Level
	LogFile         string
	AddSource       bool
	ComponentLevels map[string]slog.Level
	ConsoleWriter   io.Writer
}

var (
	globalLevelVar    = &slog.LevelVar{}
	componentLevelsMu sync.RWMutex
	componentLevels   = make(map[string]slog.Level)
)

type dynamicMinLevel struct{}

func (dynamicMinLevel) Level() slog.Level {
	minLvl := globalLevelVar.Level()
	componentLevelsMu.RLock()
	defer componentLevelsMu.RUnlock()
	for _, lvl := range componentLevels {
		if lvl < minLvl {
			minLvl = lvl
		}
	}
	return minLvl
}

// SetLogLevels updates the global log level and per-component level overrides at runtime.
func SetLogLevels(global slog.Level, compLevels map[string]slog.Level) {
	globalLevelVar.Set(global)
	componentLevelsMu.Lock()
	componentLevels = make(map[string]slog.Level, len(compLevels))
	maps.Copy(componentLevels, compLevels)
	componentLevelsMu.Unlock()
}

// GetGlobalLevel returns the current global log level as a lowercase string.
func GetGlobalLevel() string {
	return strings.ToLower(globalLevelVar.Level().String())
}

// Setup returns a configured *slog.Logger that writes to console (tint),
// and optionally to a log file, and/or a custom console writer.
func Setup(opts LoggingOptions) (*slog.Logger, io.Closer, error) {
	SetLogLevels(opts.Level, opts.ComponentLevels)

	var closer io.Closer
	var handlers []slog.Handler
	minLevel := dynamicMinLevel{}

	// 1. Console handler (Colorized via tint)
	consoleOut := opts.ConsoleWriter
	if consoleOut == nil {
		consoleOut = os.Stderr
	}
	handlers = append(handlers, tint.NewHandler(consoleOut, &tint.Options{
		Level:      minLevel,
		AddSource:  opts.AddSource,
		TimeFormat: time.TimeOnly,
	}))

	// 2. File handler (Plain text via standard TextHandler)
	if opts.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(opts.LogFile), 0o750); err != nil {
			return nil, nil, fmt.Errorf("create log directory: %w", err)
		}

		f, err := os.OpenFile(opts.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file %s: %w", opts.LogFile, err)
		}
		closer = f

		handlers = append(handlers, slog.NewTextHandler(f, &slog.HandlerOptions{
			Level:     minLevel,
			AddSource: opts.AddSource,
		}))
	}

	// 3. Combine and wrap with per-component filtering
	var h slog.Handler
	if len(handlers) == 1 {
		h = handlers[0]
	} else {
		h = &multiHandler{handlers: handlers}
	}

	h = &filterHandler{
		next: h,
	}

	logger := slog.New(h)
	slog.SetDefault(logger)

	return logger, closer, nil
}

// ParseLevel decodes a level string. Accepts case-insensitive "debug",
// "info", "warn", "error", "off". Empty returns LevelInfo.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	case "off":
		return LevelOff, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level: %q", s)
	}
}

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: next}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: next}
}

type filterHandler struct {
	next         slog.Handler
	currentAttrs []slog.Attr
}

func (f *filterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return f.next.Enabled(ctx, level)
}

func (f *filterHandler) Handle(ctx context.Context, r slog.Record) error {
	components := f.extractComponents(r)

	globalLvl := globalLevelVar.Level()
	effectiveLevel := globalLvl

	componentLevelsMu.RLock()
	for _, p := range slices.Backward(components) {
		for p != "" {
			if lvl, ok := componentLevels[p]; ok {
				effectiveLevel = lvl
				goto resolved
			}
			idx := strings.LastIndex(p, "/")
			if idx == -1 {
				break
			}
			p = p[:idx]
		}
	}
resolved:
	componentLevelsMu.RUnlock()

	if r.Level < effectiveLevel {
		return nil
	}

	return f.next.Handle(ctx, r)
}

func (f *filterHandler) extractComponents(r slog.Record) []string {
	var components []string
	for _, a := range f.currentAttrs {
		if a.Key == "component" {
			components = append(components, a.Value.String())
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			components = append(components, a.Value.String())
		}
		return true
	})
	return components
}

func (f *filterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(f.currentAttrs)+len(attrs))
	copy(newAttrs, f.currentAttrs)
	copy(newAttrs[len(f.currentAttrs):], attrs)
	return &filterHandler{
		next:         f.next.WithAttrs(attrs),
		currentAttrs: newAttrs,
	}
}

func (f *filterHandler) WithGroup(name string) slog.Handler {
	return &filterHandler{
		next:         f.next.WithGroup(name),
		currentAttrs: f.currentAttrs,
	}
}
