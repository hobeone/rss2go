package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"runtime/metrics"
	"sync/atomic"
	"time"

	"github.com/hobeone/rss2go/internal/config"
)

// Set holds application-level counters. It is safe for concurrent use.
// Pass a *Set by dependency injection instead of using package-level globals.
type Set struct {
	FeedsCrawledTotal  atomic.Uint64
	EmailsSentTotal    atomic.Uint64
	FeedsCrawledErrors atomic.Uint64
}

// IncFeedsCrawled atomically increments the feeds-crawled counter.
func (s *Set) IncFeedsCrawled() { s.FeedsCrawledTotal.Add(1) }

// IncFeedsCrawledErrors atomically increments the crawl-errors counter.
func (s *Set) IncFeedsCrawledErrors() { s.FeedsCrawledErrors.Add(1) }

// IncEmailsSent atomically increments the emails-sent counter.
func (s *Set) IncEmailsSent() { s.EmailsSentTotal.Add(1) }

// Start starts the Prometheus-style metrics HTTP server.
// It is a no-op when cfg.MetricsAddr is empty.
func Start(ctx context.Context, cfg *config.Config, m *Set, logger *slog.Logger) {
	if cfg.MetricsAddr == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "# HELP feeds_crawled_total The total number of feeds crawled.\n")
		fmt.Fprintf(w, "# TYPE feeds_crawled_total counter\n")
		fmt.Fprintf(w, "feeds_crawled_total %d\n", m.FeedsCrawledTotal.Load())

		fmt.Fprintf(w, "# HELP feeds_crawled_errors The total number of feed crawl errors.\n")
		fmt.Fprintf(w, "# TYPE feeds_crawled_errors counter\n")
		fmt.Fprintf(w, "feeds_crawled_errors %d\n", m.FeedsCrawledErrors.Load())

		fmt.Fprintf(w, "# HELP emails_sent_total The total number of emails sent.\n")
		fmt.Fprintf(w, "# TYPE emails_sent_total counter\n")
		fmt.Fprintf(w, "emails_sent_total %d\n", m.EmailsSentTotal.Load())
	})

	server := &http.Server{
		Addr:         cfg.MetricsAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logger.Info("starting metrics server", "addr", cfg.MetricsAddr)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server failed", "error", err)
		}
	}()
	// #nosec G118 -- intentional: we need a fresh context because ctx is already canceled
	go func() {
		<-ctx.Done()
		// #nosec G118 -- intentional: we need a fresh context because ctx is already canceled
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("metrics server shutdown error", "error", err)
		}
	}()
}

// StartStatsLoop starts a goroutine that logs runtime and application stats
// once per minute. It runs for the lifetime of ctx.
func StartStatsLoop(ctx context.Context, m *Set, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		const totalCPUMetric = "/cpu/classes/total:cpu-seconds"
		samples := make([]metrics.Sample, 1)
		samples[0].Name = totalCPUMetric

		metrics.Read(samples)
		lastCPU := samples[0].Value.Float64()
		lastTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)

				metrics.Read(samples)
				currentCPU := samples[0].Value.Float64()
				currentTime := time.Now()

				cpuDelta := currentCPU - lastCPU
				timeDelta := currentTime.Sub(lastTime).Seconds()
				cpuPct := (cpuDelta / timeDelta) * 100

				logger.Info("internal stats",
					"goroutines", runtime.NumGoroutine(),
					"alloc_mb", ms.Alloc/1024/1024,
					"sys_mb", ms.Sys/1024/1024,
					"heap_inuse_mb", ms.HeapInuse/1024/1024,
					"cpu_usage_pct", fmt.Sprintf("%.2f%%", cpuPct),
					"feeds_crawled", m.FeedsCrawledTotal.Load(),
					"emails_sent", m.EmailsSentTotal.Load(),
					"crawl_errors", m.FeedsCrawledErrors.Load(),
				)

				lastCPU = currentCPU
				lastTime = currentTime
			}
		}
	}()
}
