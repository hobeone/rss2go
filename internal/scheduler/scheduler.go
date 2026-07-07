package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"rss2go/internal/crawler"
	"rss2go/internal/database"
	"rss2go/internal/extractor"
	"rss2go/internal/sanitizer"
	"rss2go/internal/types"
)

// Config configures the feed polling scheduler.
type Config struct {
	PollInterval time.Duration
	MaxWorkers   int
}

// Scheduler handles periodic feed crawls and queues email notifications.
type Scheduler struct {
	repo         *database.Repository
	crawler      *crawler.Crawler
	extractor    *extractor.Extractor
	sanitizer    *sanitizer.Sanitizer
	cfg          Config
	inFlight     map[int64]bool
	inFlightMu   sync.Mutex
	wg           sync.WaitGroup
	shutdownCh   chan struct{}
	shutdownOnce sync.Once
	log          *slog.Logger
}

// New creates a new Scheduler instance.
func New(
	repo *database.Repository,
	cr *crawler.Crawler,
	ex *extractor.Extractor,
	sa *sanitizer.Sanitizer,
	cfg Config,
	log *slog.Logger,
) *Scheduler {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 10
	}
	if log == nil {
		log = slog.Default().With("component", "scheduler")
	}

	return &Scheduler{
		repo:       repo,
		crawler:    cr,
		extractor:  ex,
		sanitizer:  sa,
		cfg:        cfg,
		inFlight:   make(map[int64]bool),
		shutdownCh: make(chan struct{}),
		log:        log,
	}
}

// Start runs the scheduler poll loop. It blocks until context is cancelled or Stop is called.
func (s *Scheduler) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	// Initial poll on startup
	if err := s.pollFeeds(ctx); err != nil {
		s.log.Error("Initial poll error", "err", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := s.pollFeeds(ctx); err != nil {
				s.log.Error("Poll error", "err", err)
			}
		case <-ctx.Done():
			s.Stop()
			return ctx.Err()
		case <-s.shutdownCh:
			return nil
		}
	}
}

// Stop gracefully stops the scheduler, waiting for active crawl tasks to complete.
func (s *Scheduler) Stop() {
	s.shutdownOnce.Do(func() {
		close(s.shutdownCh)
		s.wg.Wait()
	})
}

// pollFeeds queries the database for feeds due and starts worker tasks.
func (s *Scheduler) pollFeeds(ctx context.Context) error {
	feeds, err := s.repo.ListFeedsDue(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("scheduler: list due feeds: %w", err)
	}

	// Semaphore to bound concurrency
	sem := make(chan struct{}, s.cfg.MaxWorkers)

	for _, f := range feeds {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.shutdownCh:
			return nil
		default:
		}

		s.inFlightMu.Lock()
		if s.inFlight[f.ID] {
			s.inFlightMu.Unlock()
			continue
		}
		s.inFlight[f.ID] = true
		s.inFlightMu.Unlock() // --- no lock held below this line ---

		// Attempt to acquire worker slot
		select {
		case sem <- struct{}{}:
			s.wg.Add(1)
			go func(feed *types.Feed) {
				defer s.wg.Done()
				defer func() {
					<-sem
					s.inFlightMu.Lock()
					delete(s.inFlight, feed.ID)
					s.inFlightMu.Unlock() // --- no lock held below this line ---
				}()
				s.processFeed(ctx, feed)
			}(f)
		default:
			// Worker pool is full; release in-flight state and skip for this poll interval
			s.inFlightMu.Lock()
			delete(s.inFlight, f.ID)
			s.inFlightMu.Unlock() // --- no lock held below this line ---
		}
	}

	return nil
}

// TriggerCrawl forces the scheduler to process a feed immediately in a separate goroutine.
func (s *Scheduler) TriggerCrawl(ctx context.Context, feed *types.Feed) bool {
	s.inFlightMu.Lock()
	if s.inFlight[feed.ID] {
		s.inFlightMu.Unlock()
		return false
	}
	s.inFlight[feed.ID] = true
	s.inFlightMu.Unlock()

	s.wg.Go(func() {
		defer func() {
			s.inFlightMu.Lock()
			delete(s.inFlight, feed.ID)
			s.inFlightMu.Unlock()
		}()
		s.processFeed(ctx, feed)
	})
	return true
}

// processFeed coordinates the lifecycle of crawling a single feed source.
func (s *Scheduler) processFeed(ctx context.Context, feed *types.Feed) {
	res, crawlErr := s.crawler.Crawl(ctx, feed)

	now := time.Now().Round(0)

	if crawlErr != nil {
		// Log crawl error
		s.log.Error("Crawl failed", "url", feed.URL, "err", crawlErr)

		// Implement exponential backoff
		feed.BackoffFactor = min(feed.BackoffFactor*1.5, 24.0)

		expBackoff := time.Duration(float64(feed.PollIntervalSecs)*feed.BackoffFactor) * time.Second
		var backoff time.Duration
		if res != nil && res.RetryAfter != nil {
			backoff = max(*res.RetryAfter, expBackoff)
		} else {
			backoff = expBackoff
		}

		feed.NextPollAt = now.Add(backoff)
		feed.LastErrorStr = crawlErr.Error()
		feed.LastErrorTime = &now
		feed.LastErrorSnippet = ""

		if err := s.repo.UpdateFeed(ctx, feed); err != nil {
			s.log.Error("Failed to update feed status on crawl failure", "url", feed.URL, "err", err)
		}
		return
	}

	// Handle standard success paths
	feed.BackoffFactor = 1.0
	feed.LastErrorStr = ""
	feed.LastErrorTime = nil
	feed.LastErrorSnippet = ""
	feed.LastPolledAt = &now
	feed.NextPollAt = now.Add(time.Duration(feed.PollIntervalSecs) * time.Second)

	if res.NotModified {
		if err := s.repo.UpdateFeed(ctx, feed); err != nil {
			s.log.Error("Failed to update feed status on NotModified", "url", feed.URL, "err", err)
		}
		return
	}

	// Crawl succeeded and has updates
	feed.ETag = res.ETag
	feed.LastModified = res.LastModified

	// Update feed metadata before processing items
	if err := s.repo.UpdateFeed(ctx, feed); err != nil {
		s.log.Error("Failed to update feed cache markers", "url", feed.URL, "err", err)
	}

	// Load subscribers
	subscribers, err := s.repo.ListSubscriptionsForFeed(ctx, feed.ID)
	if err != nil {
		s.log.Error("Failed to load subscriptions", "title", feed.Title, "err", err)
		return
	}

	// Parse items
	for _, item := range res.Feed.Items {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdownCh:
			return
		default:
		}

		link := crawler.ResolveItemLink(item)

		guid := item.GUID
		if guid == "" {
			guid = link
		}
		if guid == "" {
			guid = item.Title
		}
		if guid == "" {
			continue // Unidentifiable item
		}

		seen, err := s.repo.IsItemSeen(ctx, feed.ID, guid)
		if err != nil {
			s.log.Error("Failed to check seen state for item", "guid", guid, "feed", feed.Title, "err", err)
			continue
		}
		if seen {
			continue
		}

		// Process item content
		content := item.Content
		if content == "" {
			content = item.Description
		}

		// Extract full article if requested and we have a valid link
		if feed.ExtractFullArticle && link != "" {
			extracted, err := s.extractor.Extract(ctx, link, feed.ExtractionStrategy, feed.CSSSelector)
			if err != nil {
				// Log and fallback to standard feed content
				s.log.Warn("Extraction failed (falling back to summary)", "feed", feed.Title, "link", link, "err", err)
			} else if extracted != "" {
				content = extracted
			}
		}

		// Sanitize HTML
		sanitized, err := s.sanitizer.Sanitize(content, feed.URL)
		if err != nil {
			s.log.Error("Failed to sanitize content", "guid", guid, "err", err)
			continue
		}

		// Construct HTML email body containing the title, link, and sanitized content.
		emailBody := fmt.Sprintf("<h2><a href=\"%s\">%s</a></h2>%s", link, item.Title, sanitized)

		// Queue email notifications atomically per subscriber for privacy
		if len(subscribers) == 0 {
			if err := s.repo.MarkItemSeen(ctx, feed.ID, guid); err != nil {
				s.log.Error("Failed to mark item seen with 0 subscribers", "err", err)
			}
		} else {
			for _, sub := range subscribers {
				txErr := s.repo.WithTx(ctx, func(txRepo *database.Repository) error {
					// Double-check inside txn
					txSeen, err := txRepo.IsItemSeen(ctx, feed.ID, guid)
					if err != nil {
						return err
					}
					if txSeen {
						return nil
					}

					outboxItem := &types.OutboxItem{
						Subject:       fmt.Sprintf("[%s] %s", feed.Title, item.Title),
						Body:          emailBody,
						Status:        types.OutboxPending,
						NextAttemptAt: time.Now(),
						Recipients:    []string{sub.Email},
					}

					if err := txRepo.EnqueueOutboxItem(ctx, outboxItem); err != nil {
						return err
					}

					return txRepo.MarkItemSeen(ctx, feed.ID, guid)
				})
				if txErr != nil {
					s.log.Error("Failed to queue notification and mark seen", "err", txErr)
				}
			}
		}
	}
}
