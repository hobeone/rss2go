package outbox

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rss2go/internal/database"
	"rss2go/internal/types"
)

type MockSender struct {
	mu     sync.Mutex
	sent   []sentEmail
	err    error
	called int
}

type sentEmail struct {
	Subject    string
	Body       string
	Recipients []string
}

func (m *MockSender) Send(ctx context.Context, subject string, body string, recipients []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called++

	if m.err != nil {
		return m.err
	}

	m.sent = append(m.sent, sentEmail{
		Subject:    subject,
		Body:       body,
		Recipients: recipients,
	})
	return nil
}

func (m *MockSender) getSent() []sentEmail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent
}

func (m *MockSender) getCalled() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

func setupTestDB(t *testing.T) *database.Repository {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return database.NewRepository(db)
}

func TestOutboxQueueHappyPath(t *testing.T) {
	repo := setupTestDB(t)
	sender := &MockSender{}
	ctx := context.Background()

	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		PollInterval:   5 * time.Millisecond,
	}

	queue := NewQueue(repo, sender, cfg, nil)

	// Enqueue email
	item := &types.OutboxItem{
		Subject:       "Test Hello",
		Body:          "<p>Hello World</p>",
		Recipients:    []string{"user@test.com"},
		Status:        types.OutboxPending,
		NextAttemptAt: time.Now().Add(-time.Second), // Ready to poll
	}

	if err := repo.EnqueueOutboxItem(ctx, item); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	// Run outbox processPending once; delivery is synchronous, so by the
	// time this returns, the item has already been sent.
	if err := queue.processPending(ctx); err != nil {
		t.Fatalf("processPending failed: %v", err)
	}

	// Verify sent email
	sent := sender.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 email to be sent, got %d", len(sent))
	}
	if sent[0].Subject != "Test Hello" || sent[0].Recipients[0] != "user@test.com" {
		t.Errorf("sent email details mismatch: %+v", sent[0])
	}

	// Check status in DB
	fetched, err := repo.GetOutboxItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("failed to fetch outbox item: %v", err)
	}
	if fetched.Status != types.OutboxDelivered {
		t.Errorf("expected status to be delivered, got %s", fetched.Status)
	}
}

func TestOutboxQueueRetryBackoff(t *testing.T) {
	repo := setupTestDB(t)
	sender := &MockSender{err: errors.New("temp SMTP failure")}
	ctx := context.Background()

	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		PollInterval:   5 * time.Millisecond,
	}

	queue := NewQueue(repo, sender, cfg, nil)

	item := &types.OutboxItem{
		Subject:       "Test Retry",
		Body:          "Body",
		Recipients:    []string{"user@test.com"},
		Status:        types.OutboxPending,
		NextAttemptAt: time.Now().Add(-time.Second),
	}

	if err := repo.EnqueueOutboxItem(ctx, item); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	// First attempt (fails)
	if err := queue.processPending(ctx); err != nil {
		t.Fatalf("processPending failed: %v", err)
	}

	fetched, err := repo.GetOutboxItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("failed to fetch item: %v", err)
	}

	if fetched.Status != types.OutboxPending {
		t.Errorf("expected status to revert to pending for retry, got %s", fetched.Status)
	}
	if fetched.RetryCount != 1 {
		t.Errorf("expected RetryCount to be 1, got %d", fetched.RetryCount)
	}
	if fetched.LastError != "temp SMTP failure" {
		t.Errorf("expected LastError to match, got %q", fetched.LastError)
	}

	// Verify next attempt was scheduled in the future with initial backoff
	now := time.Now()
	if fetched.NextAttemptAt.Before(now) {
		t.Errorf("expected NextAttemptAt to be in the future, got %v", fetched.NextAttemptAt)
	}
	diff := fetched.NextAttemptAt.Sub(now)
	if diff < 20*time.Millisecond || diff > 100*time.Millisecond {
		t.Errorf("expected NextAttemptAt to be set with ~50ms backoff, got diff %v", diff)
	}
}

func TestOutboxQueuePermanentFailure(t *testing.T) {
	repo := setupTestDB(t)
	sender := &MockSender{err: errors.New("permanent failure")}
	ctx := context.Background()

	cfg := Config{
		MaxRetries:     1, // Only 1 attempt allowed
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		PollInterval:   5 * time.Millisecond,
	}

	queue := NewQueue(repo, sender, cfg, nil)

	item := &types.OutboxItem{
		Subject:       "Test Perm Failure",
		Body:          "Body",
		Recipients:    []string{"user@test.com"},
		Status:        types.OutboxPending,
		NextAttemptAt: time.Now().Add(-time.Second),
	}

	if err := repo.EnqueueOutboxItem(ctx, item); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	// Run attempt (fails and hits limit)
	if err := queue.processPending(ctx); err != nil {
		t.Fatalf("processPending failed: %v", err)
	}

	fetched, err := repo.GetOutboxItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("failed to fetch item: %v", err)
	}

	if fetched.Status != types.OutboxFailed {
		t.Errorf("expected status to be failed, got %s", fetched.Status)
	}
	if fetched.RetryCount != 1 {
		t.Errorf("expected RetryCount to match max retry, got %d", fetched.RetryCount)
	}

	// Ensure it won't be processed again (retry limit reached)
	sender.mu.Lock()
	sender.called = 0 // Reset counter
	sender.mu.Unlock()

	// Try processing again
	if err := queue.processPending(ctx); err != nil {
		t.Fatalf("processPending failed: %v", err)
	}

	if sender.getCalled() != 0 {
		t.Errorf("expected 0 calls since item is failed, got %d", sender.getCalled())
	}

	// Test skipped item directly when RetryCount >= MaxRetries and next_attempt_at is in the past
	itemPast := &types.OutboxItem{
		Subject:       "Test Skipped In Past",
		Body:          "Body",
		Recipients:    []string{"user@test.com"},
		Status:        types.OutboxFailed,
		RetryCount:    3,
		NextAttemptAt: time.Now().Add(-time.Hour),
	}
	if err := repo.EnqueueOutboxItem(ctx, itemPast); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	sender.mu.Lock()
	sender.called = 0
	sender.mu.Unlock()

	if err := queue.processPending(ctx); err != nil {
		t.Fatalf("processPending failed: %v", err)
	}

	if sender.getCalled() != 0 {
		t.Errorf("expected 0 calls since RetryCount >= MaxRetries, got %d", sender.getCalled())
	}
}

func TestOutboxQueueStartStop(t *testing.T) {
	repo := setupTestDB(t)
	sender := &MockSender{}
	ctx, cancel := context.WithCancel(context.Background())

	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		PollInterval:   2 * time.Millisecond,
	}

	queue := NewQueue(repo, sender, cfg, nil)

	var runErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		runErr = queue.Start(ctx)
	})

	// Let the queue run for a bit
	time.Sleep(10 * time.Millisecond)

	// Cancel context to stop queue
	cancel()
	wg.Wait()

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		t.Errorf("expected context.Canceled error or nil, got: %v", runErr)
	}
}

func TestCalculateBackoff(t *testing.T) {
	initial := 5 * time.Second
	max := 60 * time.Second

	cases := []struct {
		retries  int
		expected time.Duration
	}{
		{0, 5 * time.Second},
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 20 * time.Second},
		{4, 40 * time.Second},
		{5, 60 * time.Second}, // Capped at max
		{10, 60 * time.Second},
		{-10, 5 * time.Second},  // negative retry check
		{62, 60 * time.Second},  // overflow check
		{100, 60 * time.Second}, // overflow check
	}

	for _, tc := range cases {
		if got := calculateBackoff(tc.retries, initial, max); got != tc.expected {
			t.Errorf("calculateBackoff(%d) = %v, expected %v", tc.retries, got, tc.expected)
		}
	}

	// Boundary test for shift >= 62 with math.MaxInt64
	largeMax := time.Duration(9223372036854775807) // math.MaxInt64
	if got := calculateBackoff(63, 1*time.Nanosecond, largeMax); got != largeMax {
		t.Errorf("calculateBackoff(63, 1ns, MaxInt64) = %v, expected %v", got, largeMax)
	}

	// Boundary test where max is not exactly divisible by initial
	// initial = 3, max = 7, retryCount = 2 (shift = 1, factor = 2, maxFactor = 2)
	// factor > maxFactor is false (2 > 2 is false), so it should return 3 * 2 = 6, not max (7)
	if got := calculateBackoff(2, 3*time.Nanosecond, 7*time.Nanosecond); got != 6*time.Nanosecond {
		t.Errorf("calculateBackoff(2, 3ns, 7ns) = %v, expected 6ns", got)
	}
}

func TestQueueDefaultConfigFallback(t *testing.T) {
	q := NewQueue(nil, nil, Config{}, nil)
	if q.cfg.MaxRetries != 5 {
		t.Errorf("expected default MaxRetries 5, got %d", q.cfg.MaxRetries)
	}
	if q.cfg.InitialBackoff != 5*time.Minute {
		t.Errorf("expected default InitialBackoff 5m, got %v", q.cfg.InitialBackoff)
	}
	if q.cfg.MaxBackoff != 24*time.Hour {
		t.Errorf("expected default MaxBackoff 24h, got %v", q.cfg.MaxBackoff)
	}
	if q.cfg.PollInterval != 10*time.Second {
		t.Errorf("expected default PollInterval 10s, got %v", q.cfg.PollInterval)
	}
}

type erroringDBTX struct {
	database.DBTX
	failList                  bool
	failUpdateAfterDelivering bool
	deliveringCalled          bool
}

func (m *erroringDBTX) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.failUpdateAfterDelivering && strings.Contains(query, "UPDATE outbox SET") {
		if m.deliveringCalled {
			return nil, errors.New("forced update error")
		}
		m.deliveringCalled = true
	}
	return m.DBTX.ExecContext(ctx, query, args...)
}

func (m *erroringDBTX) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.failList && strings.Contains(query, "FROM outbox") {
		return nil, errors.New("forced list error")
	}
	return m.DBTX.QueryContext(ctx, query, args...)
}

func TestOutboxQueueDBCallbacks(t *testing.T) {
	// 1. Test when ListPendingOutboxItems returns an error
	t.Run("list pending fails", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")
		db, err := database.Open(dbPath)
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer func() { _ = db.Close() }()

		mockTX := &erroringDBTX{
			DBTX:     db,
			failList: true,
		}
		repo := database.NewRepository(mockTX)
		sender := &MockSender{}
		cfg := Config{PollInterval: 2 * time.Millisecond}
		queue := NewQueue(repo, sender, cfg, nil)

		// This should return the database list error
		err = queue.processPending(context.Background())
		if err == nil {
			t.Error("expected processPending to return error, got nil")
		}
	})

	// 2. Test when update status fails after delivery success
	t.Run("update status fails on delivery success", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")
		db, err := database.Open(dbPath)
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer func() { _ = db.Close() }()

		mockTX := &erroringDBTX{
			DBTX:                      db,
			failUpdateAfterDelivering: true,
		}
		repo := database.NewRepository(mockTX)
		sender := &MockSender{}
		cfg := Config{
			MaxRetries:     3,
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     100 * time.Millisecond,
		}
		queue := NewQueue(repo, sender, cfg, nil)

		item := &types.OutboxItem{
			Subject:       "Test Success Status Update Fail",
			Body:          "Body",
			Recipients:    []string{"user@test.com"},
			Status:        types.OutboxPending,
			NextAttemptAt: time.Now().Add(-time.Second),
		}
		if err := repo.EnqueueOutboxItem(context.Background(), item); err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		if err := queue.processPending(context.Background()); err != nil {
			t.Fatalf("processPending failed: %v", err)
		}
		// Status should still be delivering because final update failed
		fetched, err := repo.GetOutboxItem(context.Background(), item.ID)
		if err != nil {
			t.Fatalf("failed to fetch: %v", err)
		}
		if fetched.Status != types.OutboxDelivering {
			t.Errorf("expected status to remain OutboxDelivering, got %s", fetched.Status)
		}
	})

	// 3. Test when update status fails after delivery error
	t.Run("update status fails on delivery error", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")
		db, err := database.Open(dbPath)
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer func() { _ = db.Close() }()

		mockTX := &erroringDBTX{
			DBTX:                      db,
			failUpdateAfterDelivering: true,
		}
		repo := database.NewRepository(mockTX)
		sender := &MockSender{err: errors.New("smtp fail")}
		cfg := Config{
			MaxRetries:     3,
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     100 * time.Millisecond,
		}
		queue := NewQueue(repo, sender, cfg, nil)

		item := &types.OutboxItem{
			Subject:       "Test Fail Status Update Fail",
			Body:          "Body",
			Recipients:    []string{"user@test.com"},
			Status:        types.OutboxPending,
			NextAttemptAt: time.Now().Add(-time.Second),
		}
		if err := repo.EnqueueOutboxItem(context.Background(), item); err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		if err := queue.processPending(context.Background()); err != nil {
			t.Fatalf("processPending failed: %v", err)
		}
		// Status should still be delivering
		fetched, err := repo.GetOutboxItem(context.Background(), item.ID)
		if err != nil {
			t.Fatalf("failed to fetch: %v", err)
		}
		if fetched.Status != types.OutboxDelivering {
			t.Errorf("expected status to remain OutboxDelivering, got %s", fetched.Status)
		}
	})
}

func TestOutboxQueueDistantFutureAndBoundary(t *testing.T) {
	repo := setupTestDB(t)
	sender := &MockSender{err: errors.New("smtp fail")}
	ctx := context.Background()

	cfg := Config{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}
	queue := NewQueue(repo, sender, cfg, nil)

	// Enqueue item with RetryCount = 1 (1 retry remaining)
	item := &types.OutboxItem{
		Subject:       "Distant Future Test",
		Body:          "Body",
		Recipients:    []string{"user@test.com"},
		Status:        types.OutboxPending,
		RetryCount:    1,
		NextAttemptAt: time.Now().Add(-time.Second),
	}
	if err := repo.EnqueueOutboxItem(ctx, item); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	if err := queue.processPending(ctx); err != nil {
		t.Fatalf("processPending failed: %v", err)
	}

	fetched, err := repo.GetOutboxItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("failed to fetch: %v", err)
	}

	if fetched.Status != types.OutboxFailed {
		t.Errorf("expected status to be failed, got %s", fetched.Status)
	}
	if fetched.RetryCount != 2 {
		t.Errorf("expected RetryCount to be 2, got %d", fetched.RetryCount)
	}

	// Verify next attempt was scheduled in the distant future
	futureThreshold := time.Now().Add(50 * 365 * 24 * time.Hour)
	if fetched.NextAttemptAt.Before(futureThreshold) {
		t.Errorf("expected NextAttemptAt to be in the distant future (>50 years), got %v", fetched.NextAttemptAt)
	}

	// Now try to run processing again with an item whose RetryCount is exactly equal to MaxRetries (2)
	sender.mu.Lock()
	sender.called = 0
	sender.mu.Unlock()

	itemExact := &types.OutboxItem{
		Subject:       "Exact Boundary Test",
		Body:          "Body",
		Recipients:    []string{"user@test.com"},
		Status:        types.OutboxPending,
		RetryCount:    2,
		NextAttemptAt: time.Now().Add(-time.Second),
	}
	if err := repo.EnqueueOutboxItem(ctx, itemExact); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	if err := queue.processPending(ctx); err != nil {
		t.Fatalf("processPending failed: %v", err)
	}

	if sender.getCalled() != 0 {
		t.Errorf("expected 0 calls since RetryCount matches MaxRetries, got %d", sender.getCalled())
	}
}

// TestOutboxQueueSequentialOrder proves delivery is now fully sequential and
// oldest-due-first, matching ListPendingOutboxItems' ORDER BY next_attempt_at.
func TestOutboxQueueSequentialOrder(t *testing.T) {
	repo := setupTestDB(t)
	sender := &MockSender{}
	ctx := context.Background()

	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		PollInterval:   5 * time.Millisecond,
	}
	queue := NewQueue(repo, sender, cfg, nil)

	base := time.Now().Add(-time.Hour)
	// Enqueue deliberately out of NextAttemptAt order.
	items := []*types.OutboxItem{
		{Subject: "third", Body: "b", Recipients: []string{"u@test.com"}, Status: types.OutboxPending, NextAttemptAt: base.Add(30 * time.Minute)},
		{Subject: "first", Body: "b", Recipients: []string{"u@test.com"}, Status: types.OutboxPending, NextAttemptAt: base.Add(10 * time.Minute)},
		{Subject: "second", Body: "b", Recipients: []string{"u@test.com"}, Status: types.OutboxPending, NextAttemptAt: base.Add(20 * time.Minute)},
	}
	for _, it := range items {
		if err := repo.EnqueueOutboxItem(ctx, it); err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}
	}

	if err := queue.processPending(ctx); err != nil {
		t.Fatalf("processPending failed: %v", err)
	}

	sent := sender.getSent()
	if len(sent) != 3 {
		t.Fatalf("expected 3 emails sent, got %d", len(sent))
	}

	wantOrder := []string{"first", "second", "third"}
	gotOrder := make([]string, len(sent))
	for i, s := range sent {
		gotOrder[i] = s.Subject
	}
	for i, want := range wantOrder {
		if gotOrder[i] != want {
			t.Errorf("send order mismatch: got %v, want %v", gotOrder, wantOrder)
			break
		}
	}
}

// blockingSender blocks in Send until release is closed, signaling once
// (non-blocking) on started when a send has actually begun.
type blockingSender struct {
	release <-chan struct{}
	started chan struct{}
}

func (b *blockingSender) Send(_ context.Context, _ string, _ string, _ []string) error {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-b.release
	return nil
}

// TestOutboxQueueStopWaitsForInFlightDelivery proves Stop()'s documented
// "waits for in-flight deliveries to complete" guarantee still holds after
// re-scoping wg from per-item to per-processPending-call: Stop() called from
// a goroutine other than Start's own must block until the currently
// in-flight delivery actually finishes.
func TestOutboxQueueStopWaitsForInFlightDelivery(t *testing.T) {
	repo := setupTestDB(t)

	release := make(chan struct{})
	sender := &blockingSender{release: release, started: make(chan struct{}, 1)}
	ctx := context.Background()

	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		PollInterval:   2 * time.Millisecond,
	}
	queue := NewQueue(repo, sender, cfg, nil)

	item := &types.OutboxItem{
		Subject:       "Slow Send",
		Body:          "Body",
		Recipients:    []string{"user@test.com"},
		Status:        types.OutboxPending,
		NextAttemptAt: time.Now().Add(-time.Second),
	}
	if err := repo.EnqueueOutboxItem(ctx, item); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	var startWg sync.WaitGroup
	startWg.Go(func() {
		_ = queue.Start(ctx)
	})

	// Wait until delivery has actually started, so Stop() below is
	// guaranteed to race against real in-flight work, not an empty poll.
	select {
	case <-sender.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delivery to start")
	}

	stopped := make(chan struct{})
	go func() {
		queue.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("Stop() returned before in-flight delivery completed")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return after delivery completed")
	}

	startWg.Wait()
}

// cancelAfterFirstSendSender wraps a MockSender and cancels the given
// context after its first Send() call, simulating ctx being canceled
// partway through a poll cycle's batch.
type cancelAfterFirstSendSender struct {
	*MockSender
	cancel func()
}

func (s *cancelAfterFirstSendSender) Send(ctx context.Context, subject, body string, recipients []string) error {
	err := s.MockSender.Send(ctx, subject, body, recipients)
	s.cancel()
	return err
}

// TestOutboxQueueProcessPendingStopsOnCanceledContext proves processPending
// bails out of its sequential delivery loop as soon as ctx is canceled,
// rather than working through every remaining item in the batch first.
func TestOutboxQueueProcessPendingStopsOnCanceledContext(t *testing.T) {
	repo := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	mock := &MockSender{}
	sender := &cancelAfterFirstSendSender{MockSender: mock, cancel: cancel}

	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}
	queue := NewQueue(repo, sender, cfg, nil)

	for i := range 3 {
		item := &types.OutboxItem{
			Subject:       "Item",
			Body:          "Body",
			Recipients:    []string{"user@test.com"},
			Status:        types.OutboxPending,
			NextAttemptAt: time.Now().Add(-time.Duration(3-i) * time.Second),
		}
		if err := repo.EnqueueOutboxItem(context.Background(), item); err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}
	}

	err := queue.processPending(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	if got := mock.getCalled(); got != 1 {
		t.Errorf("expected exactly 1 delivery attempt before bailing on cancellation, got %d", got)
	}
}

// recordingHandler is a minimal slog.Handler that captures every record it
// receives, so tests can assert on log level/content without parsing text
// output.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *recordingHandler) hasMessageAtLevel(msg string, level slog.Level) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == level && r.Message == msg {
			return true
		}
	}
	return false
}

// TestOutboxQueueNoErrorLogOnContextCancellation proves a context-cancellation
// mid-batch (processPending returning context.Canceled) is not logged at
// Error level — that's routine, cancel-driven shutdown, not a genuine
// processing failure.
func TestOutboxQueueNoErrorLogOnContextCancellation(t *testing.T) {
	repo := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	mock := &MockSender{}
	sender := &cancelAfterFirstSendSender{MockSender: mock, cancel: cancel}
	handler := &recordingHandler{}

	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		PollInterval:   2 * time.Millisecond,
	}
	queue := NewQueue(repo, sender, cfg, slog.New(handler))

	for i := range 3 {
		item := &types.OutboxItem{
			Subject:       "Item",
			Body:          "Body",
			Recipients:    []string{"user@test.com"},
			Status:        types.OutboxPending,
			NextAttemptAt: time.Now().Add(-time.Duration(3-i) * time.Second),
		}
		if err := repo.EnqueueOutboxItem(context.Background(), item); err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}
	}

	// Drive through the real Start loop: item 1's Send() cancels ctx, so
	// processPending's ctx.Err() check bails on item 2, returning
	// context.Canceled up into Start's own log-suppression logic.
	var wg sync.WaitGroup
	wg.Go(func() {
		_ = queue.Start(ctx)
	})
	wg.Wait()

	if got := mock.getCalled(); got != 1 {
		t.Fatalf("expected exactly 1 delivery attempt before Start's loop observed cancellation, got %d", got)
	}
	// Scoped to Start's own "Processing error" log call (the one this fix
	// touches) rather than asserting no Error-level record at all: item 1's
	// own delivery can independently log an unrelated DB-write error here
	// (its status-update races the same cancellation), which is pre-existing
	// deliverItem behavior this PR does not change and is out of scope for
	// this test's claim.
	if handler.hasMessageAtLevel("Processing error", slog.LevelError) {
		t.Error("expected no \"Processing error\" log record for a context-cancellation shutdown")
	}
}
