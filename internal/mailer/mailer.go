package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hobeone/rss2go/internal/config"
	"github.com/hobeone/rss2go/internal/db"
	"github.com/hobeone/rss2go/internal/metrics"
	"github.com/hobeone/rss2go/internal/sanitize"
	"gopkg.in/gomail.v2"
)

// MailRequest represents a request to send an email.
type MailRequest struct {
	To      []string
	Subject string
	Body    string
}

// sendFunc is the injectable sender used for testing.
type sendFunc func(req MailRequest) error

const (
	maxRetries    = 5
	initialDelay  = 10 * time.Second
	maxRetryDelay = 30 * time.Minute
)

// Sender delivers email directly with SMTP connection pooling and retry.
// Suitable for CLI commands that send one-off emails without pool overhead.
type Sender struct {
	config *config.Config
	logger *slog.Logger
	send   sendFunc

	mu         sync.Mutex
	smtpDialer *gomail.Dialer
	smtpConn   gomail.SendCloser
	idleTimer  *time.Timer
}

// NewSender creates a Sender for direct, synchronous email delivery.
func NewSender(cfg *config.Config, logger *slog.Logger) *Sender {
	s := &Sender{
		config: cfg,
		logger: logger.With("component", "mailer"),
	}
	if cfg.SMTPServer != "" {
		s.smtpDialer = gomail.NewDialer(cfg.SMTPServer, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass)
	}
	s.send = s.defaultSender
	return s
}

// Send delivers an email immediately, retrying with exponential backoff on failure.
func (s *Sender) Send(req MailRequest) error {
	var err error
	for i := 0; i <= maxRetries; i++ {
		err = s.send(req)
		if err == nil {
			atomic.AddUint64(&metrics.EmailsSentTotal, 1)
			return nil
		}
		if i < maxRetries {
			delay := min(time.Duration(math.Pow(2, float64(i)))*initialDelay, maxRetryDelay)
			s.logger.Warn("failed to send email, retrying", "attempt", i+1, "delay", delay, "error", err)
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("failed to send email after %d retries: %w", maxRetries, err)
}

// Close closes any open SMTP connection held by the Sender.
func (s *Sender) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeSMTP()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
}

func (s *Sender) defaultSender(req MailRequest) error {
	if s.config.SMTPServer != "" {
		return s.persistentSMTPSender(req)
	}
	if s.config.Sendmail != "" {
		return s.sendSendmail(req)
	}
	return fmt.Errorf("no mailer configured (SMTP or Sendmail)")
}

func (s *Sender) persistentSMTPSender(req MailRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.smtpConn == nil {
		s.logger.Debug("opening new SMTP connection", "server", s.config.SMTPServer)
		conn, err := s.smtpDialer.Dial()
		if err != nil {
			return fmt.Errorf("failed to dial SMTP: %w", err)
		}
		s.smtpConn = conn
	}

	m := gomail.NewMessage()
	m.SetHeader("From", s.config.SMTPSender)
	m.SetHeader("To", req.To...)
	m.SetHeader("Subject", req.Subject)
	m.SetBody("text/html", req.Body)

	if err := gomail.Send(s.smtpConn, m); err != nil {
		s.closeSMTP()
		return fmt.Errorf("failed to send via SMTP (server: %s, to: %v, subject: %q): %w", s.config.SMTPServer, req.To, req.Subject, err)
	}

	s.logger.Info("email sent successfully", "to", req.To, "subject", req.Subject)

	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.idleTimer = time.AfterFunc(3*time.Minute, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.smtpConn != nil {
			s.logger.Debug("closing idle SMTP connection", "server", s.config.SMTPServer)
			s.closeSMTP()
		}
	})

	return nil
}

func (s *Sender) closeSMTP() {
	if s.smtpConn != nil {
		s.logger.Debug("closing SMTP connection")
		if err := s.smtpConn.Close(); err != nil {
			s.logger.Debug("SMTP close error", "error", err)
		}
		s.smtpConn = nil
	}
}

func (s *Sender) sendSendmail(req MailRequest) error {
	// #nosec G204 - sendmail path is part of application configuration
	cmd := exec.Command(s.config.Sendmail, "-t")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	sanitizedTo := make([]string, len(req.To))
	for i, t := range req.To {
		sanitizedTo[i] = sanitize.Header(t)
	}

	msg := fmt.Sprintf("To: %s\nSubject: %s\nContent-Type: text/html; charset=UTF-8\n\n%s",
		strings.Join(sanitizedTo, ", "), sanitize.Header(req.Subject), req.Body)

	errChan := make(chan error, 1)
	go func() {
		defer func() { _ = stdin.Close() }()
		_, werr := stdin.Write([]byte(msg))
		errChan <- werr
	}()

	// Always read from errChan: when the process exits (cmd.Run returns),
	// its stdin pipe breaks, causing the Write goroutine to unblock and send.
	runErr := cmd.Run()
	writeErr := <-errChan

	if runErr != nil {
		return fmt.Errorf("sendmail command failed: %w", runErr)
	}
	if writeErr != nil {
		return fmt.Errorf("failed to write to sendmail stdin: %w", writeErr)
	}
	return nil
}



// Pool is a DB-backed outbox worker pool for at-least-once email delivery.
//
// Submit persists the request to the DB before signalling workers, so a crash
// between Submit and delivery will not lose the email.
type Pool struct {
	sender *Sender
	store  db.Store
	logger *slog.Logger

	notifyC      chan struct{}
	outboxCtx    context.Context
	outboxCancel context.CancelFunc
	wg           sync.WaitGroup
}

// NewPool creates an outbox-backed mailer pool. The store must be non-nil.
func NewPool(size int, cfg *config.Config, store db.Store, logger *slog.Logger) *Pool {
	p := &Pool{
		sender:  NewSender(cfg, logger),
		store:   store,
		logger:  logger.With("component", "mailer"),
		notifyC: make(chan struct{}, 1),
	}

	// Recover any rows stuck in 'delivering' from a prior crash.
	if err := store.ResetDeliveringToPending(context.Background()); err != nil {
		p.logger.Error("failed to reset delivering→pending on startup", "error", err)
	}

	p.outboxCtx, p.outboxCancel = context.WithCancel(context.Background())

	p.wg.Add(size)
	for i := range size {
		go p.outboxWorker(i)
	}

	return p
}

// Submit enqueues a mail request for at-least-once delivery via the DB outbox.
func (p *Pool) Submit(ctx context.Context, req MailRequest) {
	if err := p.store.EnqueueEmail(ctx, req.To, req.Subject, req.Body); err != nil {
		p.logger.Error("failed to enqueue email to outbox", "error", err)
		return
	}
	select {
	case p.notifyC <- struct{}{}:
	default:
	}
}

// Send delivers an email directly, bypassing the outbox queue.
func (p *Pool) Send(req MailRequest) error {
	return p.sender.Send(req)
}

// Close shuts down the outbox workers and waits for in-flight work to finish.
func (p *Pool) Close() {
	p.outboxCancel()
	p.wg.Wait()
	p.sender.Close()
}

func (p *Pool) outboxWorker(id int) {
	defer p.wg.Done()
	p.logger.Debug("outbox worker started", "worker_id", id)

	// Start with a stopped timer so the first Reset at the loop top is safe.
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		timer.Reset(30 * time.Second)
		select {
		case <-p.outboxCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			p.logger.Debug("outbox worker stopping", "worker_id", id)
			return
		case <-p.notifyC:
			if !timer.Stop() {
				<-timer.C
			}
			p.drainOutbox()
		case <-timer.C:
			// Periodic drain to pick up rows that were missed (e.g. after a
			// restart that recovered stuck rows without sending a notify).
			p.drainOutbox()
		}
	}
}

func (p *Pool) drainOutbox() {
	for {
		entry, err := p.store.ClaimPendingEmail(p.outboxCtx)
		if err != nil {
			if p.outboxCtx.Err() == nil {
				p.logger.Error("failed to claim pending email from outbox", "error", err)
			}
			return
		}
		if entry == nil {
			return // queue empty
		}

		err = p.sender.Send(MailRequest{To: entry.Recipients, Subject: entry.Subject, Body: entry.Body})
		if err != nil {
			p.logger.Error("failed to deliver outbox email", "id", entry.ID, "error", err)
			if rerr := p.store.ResetEmailToPending(p.outboxCtx, entry.ID); rerr != nil {
				p.logger.Error("failed to reset outbox email to pending", "id", entry.ID, "error", rerr)
			}
			return
		}

		if merr := p.store.MarkEmailDelivered(p.outboxCtx, entry.ID); merr != nil {
			p.logger.Error("failed to mark outbox email delivered", "id", entry.ID, "error", merr)
		}
	}
}
