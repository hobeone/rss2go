package mailer

import (
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/hobeone/rss2go/internal/config"
	"github.com/hobeone/rss2go/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestSender_SendIncrementsMetrics(t *testing.T) {
	atomic.StoreUint64(&metrics.EmailsSentTotal, 0)

	s := NewSender(&config.Config{}, slog.New(slog.DiscardHandler))
	defer s.Close()

	var got MailRequest
	s.send = func(req MailRequest) error {
		got = req
		return nil
	}

	req := MailRequest{To: []string{"test@example.com"}, Subject: "Test", Body: "Body"}
	err := s.Send(req)

	assert.NoError(t, err)
	assert.Equal(t, req.To, got.To)
	assert.Equal(t, req.Subject, got.Subject)
	assert.Equal(t, uint64(1), atomic.LoadUint64(&metrics.EmailsSentTotal))
}

func TestSender_DefaultSenderRouting(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("NoConfig", func(t *testing.T) {
		s := NewSender(&config.Config{}, logger)
		defer s.Close()
		err := s.defaultSender(MailRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no mailer configured")
	})

	t.Run("SMTPConfigured", func(t *testing.T) {
		s := NewSender(&config.Config{SMTPServer: "localhost"}, logger)
		defer s.Close()
		err := s.defaultSender(MailRequest{To: []string{"a@b.com"}})
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "no mailer configured")
	})

	t.Run("SendmailConfigured", func(t *testing.T) {
		s := NewSender(&config.Config{Sendmail: "/bin/false"}, logger)
		defer s.Close()
		err := s.defaultSender(MailRequest{To: []string{"a@b.com"}})
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "no mailer configured")
	})
}
