package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/hobeone/rss2go/internal/config"
	"log/slog"
)

var (
	FeedsCrawledTotal  uint64
	EmailsSentTotal    uint64
	FeedsCrawledErrors uint64
)

// Start starts a simple metrics HTTP server if configured.
func Start(cfg *config.Config, logger *slog.Logger) {
	if cfg.MetricsAddr == "" {
		return
	}

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
		if err := http.ListenAndServe(cfg.MetricsAddr, mux); err != nil {
			logger.Error("metrics server failed", "error", err)
		}
	}()
}
