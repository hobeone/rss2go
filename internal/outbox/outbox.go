package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"rss2go/internal/database"
	"rss2go/internal/notifier"
	"rss2go/internal/types"
)

// Config configures the outbox queue processor.
type Config struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	PollInterval   time.Duration
}

// Queue manages background processing of the durable email outbox.
type Queue struct {
	repo         *database.Repository
	sender       notifier.Sender
	cfg          Config
	inFlight     map[int64]bool
	inFlightMu   sync.Mutex
	wg           sync.WaitGroup
	shutdownCh   chan struct{}
	shutdownOnce sync.Once
	log          *slog.Logger
}

// NewQueue creates a new outbox Queue processor.
func NewQueue(repo *database.Repository, sender notifier.Sender, cfg Config, log *slog.Logger) *Queue {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 5 * time.Minute
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 24 * time.Hour
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if log == nil {
		log = slog.Default().With("component", "outbox")
	}

	return &Queue{
		repo:       repo,
		sender:     sender,
		cfg:        cfg,
		inFlight:   make(map[int64]bool),
		shutdownCh: make(chan struct{}),
		log:        log,
	}
}

// Start launches the outbox processing loop. It blocks until context is cancelled or Stop is called.
func (q *Queue) Start(ctx context.Context) error {
	ticker := time.NewTicker(q.cfg.PollInterval)
	defer ticker.Stop()

	for {
		// Process pending items
		if err := q.processPending(ctx); err != nil {
			q.log.Error("Processing error", "err", err)
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			q.Stop()
			return ctx.Err()
		case <-q.shutdownCh:
			return nil
		}
	}
}

// Stop gracefully stops the queue processor, waiting for in-flight deliveries to complete.
func (q *Queue) Stop() {
	q.shutdownOnce.Do(func() {
		close(q.shutdownCh)
		q.wg.Wait()
	})
}

// processPending queries items ready for delivery and spawns worker goroutines.
func (q *Queue) processPending(ctx context.Context) error {
	// Only fetch items where retry count is below limits
	items, err := q.repo.ListPendingOutboxItems(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("outbox: list pending: %w", err)
	}

	for _, item := range items {
		// Filter out items already hit max retries
		if item.RetryCount >= q.cfg.MaxRetries {
			continue
		}

		q.inFlightMu.Lock()
		if q.inFlight[item.ID] {
			q.inFlightMu.Unlock()
			continue
		}
		q.inFlight[item.ID] = true
		q.inFlightMu.Unlock()

		q.wg.Add(1)
		go func(it *types.OutboxItem) {
			defer q.wg.Done()
			defer func() {
				q.inFlightMu.Lock()
				delete(q.inFlight, it.ID)
				q.inFlightMu.Unlock()
			}()

			q.deliverItem(ctx, it)
		}(item)
	}

	return nil
}

// deliverItem attempts sending an email and updates its database state based on results.
func (q *Queue) deliverItem(ctx context.Context, item *types.OutboxItem) {
	// Set status to delivering in DB first to protect against double delivery on restart
	item.Status = types.OutboxDelivering
	now := time.Now()
	item.LastAttemptAt = &now

	// We wrap status updates in WithTx if needed, but since it's a single update, standard repo write is fine.
	if err := q.repo.UpdateOutboxItemStatus(ctx, item); err != nil {
		q.log.Error("Failed to set status to delivering", "id", item.ID, "err", err)
		return
	}

	// Attempt delivery
	err := q.sender.Send(ctx, item.Subject, item.Body, item.Recipients)
	now = time.Now()
	item.LastAttemptAt = &now

	if err != nil {
		// Increment retry counters and schedule next attempt
		item.RetryCount++
		item.LastError = err.Error()

		if item.RetryCount >= q.cfg.MaxRetries {
			item.Status = types.OutboxFailed
			item.NextAttemptAt = time.Now().Add(100 * 365 * 24 * time.Hour) // Distant future (stop retrying)
		} else {
			item.Status = types.OutboxPending // Re-queue
			backoff := calculateBackoff(item.RetryCount, q.cfg.InitialBackoff, q.cfg.MaxBackoff)
			item.NextAttemptAt = time.Now().Add(backoff)
		}

		if dbErr := q.repo.UpdateOutboxItemStatus(ctx, item); dbErr != nil {
			q.log.Error("Failed to update failed status", "id", item.ID, "dbErr", dbErr, "sendErr", err)
		}
		return
	}

	// Success
	item.Status = types.OutboxDelivered
	item.LastError = ""
	if dbErr := q.repo.UpdateOutboxItemStatus(ctx, item); dbErr != nil {
		q.log.Error("Failed to set delivered status", "id", item.ID, "err", dbErr)
	}
}

func calculateBackoff(retryCount int, initial, max time.Duration) time.Duration {
	if retryCount <= 0 {
		return initial
	}

	shift := uint(retryCount - 1)
	if shift >= 62 {
		return max
	}

	factor := int64(1 << shift)
	maxFactor := int64(max / initial)
	if factor > maxFactor {
		return max
	}

	return initial * time.Duration(factor)
}

type dbTxRepo interface {
	UpdateOutboxItemStatus(ctx context.Context, item *types.OutboxItem) error
	ListPendingOutboxItems(ctx context.Context, now time.Time) ([]*types.OutboxItem, error)
}

var _ dbTxRepo = (*database.Repository)(nil)
