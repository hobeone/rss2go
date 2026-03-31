package mailer

import (
	"context"
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

	// nil store → legacy channel mode
	p := NewPool(1, cfg, nil, logger)
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

	p.Submit(context.Background(), req)

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
		p := NewPool(1, &config.Config{}, nil, logger)
		defer p.Close()
		err := p.defaultSender(MailRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no mailer configured")
	})

	t.Run("SMTPConfigured", func(t *testing.T) {
		p := NewPool(1, &config.Config{SMTPServer: "localhost"}, nil, logger)
		defer p.Close()
		err := p.defaultSender(MailRequest{To: []string{"a@b.com"}})
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "no mailer configured")
	})

	t.Run("SendmailConfigured", func(t *testing.T) {
		p := NewPool(1, &config.Config{Sendmail: "/bin/false"}, nil, logger)
		defer p.Close()
		err := p.defaultSender(MailRequest{To: []string{"a@b.com"}})
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "no mailer configured")
	})
}
