package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"rss2go/internal/crawler"
	"rss2go/internal/database"
	"rss2go/internal/extractor"
	"rss2go/internal/sanitizer"
	"rss2go/internal/scheduler"
	"rss2go/internal/server/ui"
)

// Config holds the HTTP server configurations.
type Config struct {
	Addr              string
	MagicSecret       string
	HeartbeatInterval time.Duration
	ShutdownTimeout   time.Duration
	Broadcaster       *LogBroadcaster
	MailerMode        string
}

// Server wraps the API routes, embedded SPA, and daemon references.
type Server struct {
	repo        *database.Repository
	scheduler   *scheduler.Scheduler
	crawler     *crawler.Crawler
	extractor   *extractor.Extractor
	sanitizer   *sanitizer.Sanitizer
	broadcaster *LogBroadcaster
	cfg         Config
	httpServer  *http.Server
	log         *slog.Logger
}

// New creates a new HTTP Server instance.
func New(
	repo *database.Repository,
	sched *scheduler.Scheduler,
	cr *crawler.Crawler,
	ex *extractor.Extractor,
	sa *sanitizer.Sanitizer,
	cfg Config,
	log *slog.Logger,
) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.MagicSecret == "" {
		// Use a random default secret if none provided to keep magic links secure
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		cfg.MagicSecret = hex.EncodeToString(b)
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 15 * time.Second
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 5 * time.Second
	}
	if log == nil {
		log = slog.Default().With("component", "api")
	}

	b := cfg.Broadcaster
	if b == nil {
		b = NewLogBroadcaster()
	}

	return &Server{
		repo:        repo,
		scheduler:   sched,
		crawler:     cr,
		extractor:   ex,
		sanitizer:   sa,
		broadcaster: b,
		cfg:         cfg,
		log:         log,
	}
}

// Handler returns the registered http.Handler containing all routes and middleware.
func (s *Server) Handler() (http.Handler, error) {
	mux := http.NewServeMux()

	// Register endpoints directly (no auth required)
	mux.HandleFunc("GET /api/v1/subscriber/manage", s.handleSubscriberManage)
	mux.HandleFunc("POST /api/v1/subscriber/unsubscribe", s.handleSubscriberUnsubscribe)

	mux.HandleFunc("GET /api/v1/feeds", s.handleGetFeeds)
	mux.HandleFunc("POST /api/v1/feeds", s.handleCreateFeed)
	mux.HandleFunc("GET /api/v1/feeds/{id}", s.handleGetFeedDetails)
	mux.HandleFunc("GET /api/v1/feeds/{id}/items", s.handleGetFeedItems)
	mux.HandleFunc("PUT /api/v1/feeds/{id}", s.handleUpdateFeed)
	mux.HandleFunc("DELETE /api/v1/feeds/{id}", s.handleDeleteFeed)

	mux.HandleFunc("GET /api/v1/users", s.handleGetUsers)
	mux.HandleFunc("POST /api/v1/users", s.handleCreateUser)
	mux.HandleFunc("DELETE /api/v1/users/{id}", s.handleDeleteUser)

	mux.HandleFunc("POST /api/v1/subscriptions", s.handleSubscribe)
	mux.HandleFunc("DELETE /api/v1/subscriptions", s.handleUnsubscribe)

	mux.HandleFunc("GET /api/v1/stats", s.handleGetStats)
	mux.HandleFunc("GET /api/v1/logs", s.handleGetLogs)
	mux.HandleFunc("GET /api/v1/outbox", s.handleGetOutbox)

	mux.HandleFunc("POST /api/v1/feeds/{id}/test", s.handleTestFeed)
	mux.HandleFunc("POST /api/v1/feeds/{id}/scan", s.handleScanFeed)
	mux.HandleFunc("POST /api/v1/feeds/{id}/catchup", s.handleCatchupFeed)
	mux.HandleFunc("POST /api/v1/feeds/{id}/rewind", s.handleRewindFeed)

	// Mount Svelte SPA static files (with SPA fallback routing)
	subFS, err := fs.Sub(ui.Files, "dist")
	if err != nil {
		return nil, fmt.Errorf("server: sub frontend build directory: %w", err)
	}
	fileServer := http.FileServer(&spaFileSystem{fs: http.FS(subFS)})
	mux.Handle("/", fileServer)

	return mux, nil
}

// Start launches the HTTP server. It blocks until context is cancelled or Stop is called.
func (s *Server) Start(ctx context.Context) error {
	handler, err := s.Handler()
	if err != nil {
		return err
	}

	s.httpServer = &http.Server{
		Addr:    s.cfg.Addr,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	s.log.Info("Starting HTTP API Server", "addr", s.cfg.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: listen and serve failed: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
}



// LogBroadcaster manages concurrent SSE client channels and maintains a circular
// ring buffer of the last 100 log lines for replay on new connections.
type LogBroadcaster struct {
	mu       sync.Mutex
	clients  map[chan string]bool
	ring     [100]string
	ringHead int // index where the next write goes
	ringLen  int // number of valid entries (capped at 100)
}

// RegisterWithReplay atomically registers a new client channel and returns a
// snapshot of buffered history (oldest→newest) in one lock acquisition, so
// there is no race between replaying history and receiving new live lines.
func (b *LogBroadcaster) RegisterWithReplay(ch chan string) []string {
	b.mu.Lock()
	defer b.mu.Unlock() // --- no lock held below this line ---
	b.clients[ch] = true
	return b.historyLocked()
}

// Unregister removes a client channel.
func (b *LogBroadcaster) Unregister(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock() // --- no lock held below this line ---
	delete(b.clients, ch)
}

// Broadcast stores msg in the ring buffer and dispatches it to all active clients.
func (b *LogBroadcaster) Broadcast(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock() // --- no lock held below this line ---

	// Write into ring, overwriting oldest when full.
	b.ring[b.ringHead] = msg
	b.ringHead = (b.ringHead + 1) % len(b.ring)
	if b.ringLen < len(b.ring) {
		b.ringLen++
	}

	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// History returns buffered lines in chronological order (oldest→newest) under the lock.
func (b *LogBroadcaster) History() []string {
	b.mu.Lock()
	defer b.mu.Unlock() // --- no lock held below this line ---
	return b.historyLocked()
}

// historyLocked returns the ring buffer contents in chronological order.
// Caller must hold b.mu.
func (b *LogBroadcaster) historyLocked() []string {
	if b.ringLen == 0 {
		return nil
	}
	out := make([]string, b.ringLen)
	// oldest entry starts at (ringHead - ringLen + len(ring)) % len(ring)
	start := (b.ringHead - b.ringLen + len(b.ring)) % len(b.ring)
	for i := range b.ringLen {
		out[i] = b.ring[(start+i)%len(b.ring)]
	}
	return out
}

var ansiStrip = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// NewLogBroadcaster creates a new LogBroadcaster instance.
func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{
		clients: make(map[chan string]bool),
	}
}

// NewSSEWriter wraps an io.Writer and broadcasts written bytes (stripped of ANSI codes) to the broadcaster.
func NewSSEWriter(broadcaster *LogBroadcaster, out io.Writer) io.Writer {
	return &sseWriter{
		broadcaster: broadcaster,
		out:         out,
	}
}

type sseWriter struct {
	broadcaster *LogBroadcaster
	out         io.Writer
}

func (w *sseWriter) Write(p []byte) (n int, err error) {
	n, err = w.out.Write(p)
	if err == nil {
		clean := ansiStrip.ReplaceAll(p, nil)
		w.broadcaster.Broadcast(string(clean))
	}
	return n, err
}

type spaFileSystem struct {
	fs http.FileSystem
}

func (s *spaFileSystem) Open(name string) (http.File, error) {
	f, err := s.fs.Open(name)
	if err == nil {
		return f, nil
	}
	// Fallback to Svelte index.html client route driver
	return s.fs.Open("/index.html")
}
