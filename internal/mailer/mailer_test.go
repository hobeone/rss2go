package mailer

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hobeone/rss2go/internal/config"
	"github.com/hobeone/rss2go/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestPool_WorkerAndMetrics(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	
	// Reset metrics
	atomic.StoreUint64(&metrics.EmailsSentTotal, 0)

	p := NewPool(1, cfg, logger)
	defer p.Close()

	done := make(chan struct{})
	var sentReq MailRequest
	mockSender := func(req MailRequest) error {
		sentReq = req
		close(done)
		return nil
	}
	p.sender = mockSender

	req := MailRequest{
		To:      []string{"test@example.com"},
		Subject: "Test",
		Body:    "Body",
	}

	p.Submit(req)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for worker to process request")
	}

	assert.Equal(t, req.To, sentReq.To)
	assert.Equal(t, req.Subject, sentReq.Subject)
	
	// Check metrics
	assert.Equal(t, uint64(1), atomic.LoadUint64(&metrics.EmailsSentTotal))
}

func TestPool_DefaultSenderRouting(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	
	t.Run("NoConfig", func(t *testing.T) {
		p := NewPool(1, &config.Config{}, logger)
		defer p.Close()
		err := p.defaultSender(MailRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no mailer configured")
	})

	t.Run("SMTPConfigured", func(t *testing.T) {
		p := NewPool(1, &config.Config{SMTPServer: "localhost"}, logger)
		defer p.Close()
		// It will fail because localhost:0 is likely refused or we don't have a mock server,
		// but we want to ensure it routes to sendSMTP and returns an SMTP-related error or connection error,
		// rather than "no mailer configured".
		err := p.defaultSender(MailRequest{To: []string{"a@b.com"}})
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "no mailer configured")
	})

	t.Run("SendmailConfigured", func(t *testing.T) {
		p := NewPool(1, &config.Config{Sendmail: "/bin/false"}, logger) // Use a command that exits with error
		defer p.Close()
		err := p.defaultSender(MailRequest{To: []string{"a@b.com"}})
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "no mailer configured")
	})
}

