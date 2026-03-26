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

var (
	FeedsCrawledTotal  uint64
	EmailsSentTotal    uint64
	FeedsCrawledErrors uint64
)

// Start starts a simple metrics HTTP server if configured and initiates periodic internal stats logging.
func Start(ctx context.Context, cfg *config.Config, logger *slog.Logger) {
	if cfg.MetricsAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "# HELP feeds_crawled_total The total number of feeds crawled.\n")
			fmt.Fprintf(w, "# TYPE feeds_crawled_total counter\n")
			fmt.Fprintf(w, "feeds_crawled_total %d\n", atomic.LoadUint64(&FeedsCrawledTotal))

			fmt.Fprintf(w, "# HELP feeds_crawled_errors The total number of feed crawl errors.\n")
			fmt.Fprintf(w, "# TYPE feeds_crawled_errors counter\n")
			fmt.Fprintf(w, "feeds_crawled_errors %d\n", atomic.LoadUint64(&FeedsCrawledErrors))

			fmt.Fprintf(w, "# HELP emails_sent_total The total number of emails sent.\n")
			fmt.Fprintf(w, "# TYPE emails_sent_total counter\n")
			fmt.Fprintf(w, "emails_sent_total %d\n", atomic.LoadUint64(&EmailsSentTotal))
		})

		logger.Info("starting metrics server", "addr", cfg.MetricsAddr)
		go func() {
			server := &http.Server{
				Addr:         cfg.MetricsAddr,
				Handler:      mux,
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  120 * time.Second,
			}
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics server failed", "error", err)
			}
		}()
	}

	// Internal stats logging loop
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
				var m runtime.MemStats
				runtime.ReadMemStats(&m)

				metrics.Read(samples)
				currentCPU := samples[0].Value.Float64()
				currentTime := time.Now()

				cpuDelta := currentCPU - lastCPU
				timeDelta := currentTime.Sub(lastTime).Seconds()
				cpuPct := (cpuDelta / timeDelta) * 100

				logger.Info("internal stats",
					"goroutines", runtime.NumGoroutine(),
					"alloc_mb", m.Alloc/1024/1024,
					"sys_mb", m.Sys/1024/1024,
					"heap_inuse_mb", m.HeapInuse/1024/1024,
					"cpu_usage_pct", fmt.Sprintf("%.2f%%", cpuPct),
					"feeds_crawled", atomic.LoadUint64(&FeedsCrawledTotal),
					"emails_sent", atomic.LoadUint64(&EmailsSentTotal),
					"crawl_errors", atomic.LoadUint64(&FeedsCrawledErrors),
				)

				lastCPU = currentCPU
				lastTime = currentTime
			}
		}
	}()
}
