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
	"gopkg.in/gomail.v2"
)

// MailRequest represents a request to send an email.
type MailRequest struct {
	To      []string
	Subject string
	Body    string
}

// Sender is an interface for sending emails.
type Sender func(req MailRequest) error

// Pool is a pool of workers for sending emails.
//
// When created with a non-nil db.Store it operates in outbox mode: Submit
// persists the request to the DB and workers drain by claiming rows. This
// gives at-least-once delivery across process restarts.
//
// When store is nil (CLI / test use) it falls back to a buffered channel
// (best-effort, in-process delivery).
type Pool struct {
	config *config.Config
	logger *slog.Logger
	sender Sender

	// outbox mode (store != nil)
	store   db.Store
	notifyC chan struct{}
	done    chan struct{}
	wg      sync.WaitGroup

	// legacy mode (store == nil)
	requests chan MailRequest

	// SMTP persistence management
	mu         sync.Mutex
	smtpDialer *gomail.Dialer
	smtpConn   gomail.SendCloser
	idleTimer  *time.Timer
}

// NewPool creates a new mailer pool.
//
// Pass a non-nil store to enable DB-backed outbox mode. Pass nil for simple
// in-process delivery (suitable for CLI commands that call Send directly).
func NewPool(size int, cfg *config.Config, store db.Store, logger *slog.Logger) *Pool {
	p := &Pool{
		config: cfg,
		logger: logger.With("component", "mailer"),
		store:  store,
	}

	if cfg.SMTPServer != "" {
		p.smtpDialer = gomail.NewDialer(cfg.SMTPServer, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass)
	}
	p.sender = p.defaultSender

	if store != nil {
		// Outbox mode: recover any rows stuck in 'delivering' from a prior crash.
		if err := store.ResetDeliveringToPending(context.Background()); err != nil {
			p.logger.Error("failed to reset delivering→pending on startup", "error", err)
		}

		p.notifyC = make(chan struct{}, 1)
		p.done = make(chan struct{})

		p.wg.Add(size)
		for i := range size {
			go p.outboxWorker(i)
		}
	} else {
		// Legacy mode: buffered channel, one goroutine per worker.
		p.requests = make(chan MailRequest, size*100)
		for i := range size {
			go p.channelWorker(i)
		}
	}

	return p
}

// Submit enqueues a mail request for delivery.
//
// In outbox mode it persists to the DB before signalling workers, so a crash
// between Submit and delivery will not lose the email. In legacy mode it
// writes to a buffered in-process channel.
func (p *Pool) Submit(ctx context.Context, req MailRequest) {
	if p.store != nil {
		if err := p.store.EnqueueEmail(ctx, req.To, req.Subject, req.Body); err != nil {
			p.logger.Error("failed to enqueue email to outbox", "error", err)
			return
		}
		select {
		case p.notifyC <- struct{}{}:
		default:
		}
	} else {
		p.requests <- req
	}
}

func (p *Pool) outboxWorker(id int) {
	defer p.wg.Done()
	p.logger.Debug("outbox worker started", "worker_id", id)
	for {
		select {
		case <-p.done:
			p.logger.Debug("outbox worker stopping", "worker_id", id)
			return
		case <-p.notifyC:
			p.drainOutbox()
		case <-time.After(30 * time.Second):
			// Periodic drain to pick up rows that were missed (e.g. after a
			// restart that recovered stuck rows without sending a notify).
			p.drainOutbox()
		}
	}
}

func (p *Pool) drainOutbox() {
	for {
		entry, err := p.store.ClaimPendingEmail(context.Background())
		if err != nil {
			p.logger.Error("failed to claim pending email from outbox", "error", err)
			return
		}
		if entry == nil {
			return // queue empty
		}

		err = p.Send(MailRequest{To: entry.Recipients, Subject: entry.Subject, Body: entry.Body})
		if err != nil {
			p.logger.Error("failed to deliver outbox email", "id", entry.ID, "error", err)
			if rerr := p.store.ResetEmailToPending(context.Background(), entry.ID); rerr != nil {
				p.logger.Error("failed to reset outbox email to pending", "id", entry.ID, "error", rerr)
			}
			return
		}

		if merr := p.store.MarkEmailDelivered(context.Background(), entry.ID); merr != nil {
			p.logger.Error("failed to mark outbox email delivered", "id", entry.ID, "error", merr)
		}
	}
}

func (p *Pool) channelWorker(id int) {
	p.logger.Debug("channel worker started", "worker_id", id)
	for req := range p.requests {
		p.logger.Debug("sending email", "to", req.To, "subject", req.Subject)
		if err := p.Send(req); err != nil {
			p.logger.Error("failed to send email", "to", req.To, "subject", req.Subject, "error", err)
		}
	}
}

const (
	maxRetries    = 5
	initialDelay  = 10 * time.Second
	maxRetryDelay = 30 * time.Minute
)

// Send sends an email immediately and returns the error.
// It retries with exponential backoff on failure.
func (p *Pool) Send(req MailRequest) error {
	var err error
	for i := 0; i <= maxRetries; i++ {
		err = p.sender(req)
		if err == nil {
			atomic.AddUint64(&metrics.EmailsSentTotal, 1)
			return nil
		}

		if i < maxRetries {
			delay := min(time.Duration(math.Pow(2, float64(i)))*initialDelay, maxRetryDelay)
			p.logger.Warn("failed to send email, retrying", "attempt", i+1, "delay", delay, "error", err)
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("failed to send email after %d retries: %w", maxRetries, err)
}

func (p *Pool) defaultSender(req MailRequest) error {
	if p.config.SMTPServer != "" {
		return p.persistentSMTPSender(req)
	}
	if p.config.Sendmail != "" {
		return p.sendSendmail(req)
	}
	return fmt.Errorf("no mailer configured (SMTP or Sendmail)")
}

func (p *Pool) persistentSMTPSender(req MailRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.smtpConn == nil {
		p.logger.Debug("opening new SMTP connection", "server", p.config.SMTPServer)
		s, err := p.smtpDialer.Dial()
		if err != nil {
			p.logger.Error("failed to dial SMTP server", "error", err)
			return fmt.Errorf("failed to dial SMTP: %w", err)
		}
		p.smtpConn = s
	}

	m := gomail.NewMessage()
	m.SetHeader("From", p.config.SMTPSender)
	m.SetHeader("To", req.To...)
	m.SetHeader("Subject", req.Subject)
	m.SetBody("text/html", req.Body)

	if err := gomail.Send(p.smtpConn, m); err != nil {
		p.logger.Error("SMTP send failed",
			"server", p.config.SMTPServer,
			"to", req.To,
			"subject", req.Subject,
			"error", err,
		)
		p.closeSMTP()
		return fmt.Errorf("failed to send via SMTP: %w", err)
	}

	p.logger.Info("email sent successfully", "to", req.To, "subject", req.Subject)

	if p.idleTimer != nil {
		p.idleTimer.Stop()
	}
	p.idleTimer = time.AfterFunc(3*time.Minute, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.smtpConn != nil {
			p.logger.Debug("closing idle SMTP connection", "server", p.config.SMTPServer)
			p.closeSMTP()
		}
	})

	return nil
}

//nolint:errcheck
func (p *Pool) closeSMTP() {
	if p.smtpConn != nil {
		p.logger.Debug("closing SMTP connection")
		p.smtpConn.Close() // #nosec G104
		p.smtpConn = nil
	}
}

func (p *Pool) sendSendmail(req MailRequest) error {
	// #nosec G204 - sendmail path is part of application configuration
	cmd := exec.Command(p.config.Sendmail, "-t")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("To: %s\nSubject: %s\nContent-Type: text/html; charset=UTF-8\n\n%s",
		strings.Join(req.To, ", "), req.Subject, req.Body)

	errChan := make(chan error, 1)
	go func() {
		defer func() { _ = stdin.Close() }()
		_, werr := stdin.Write([]byte(msg))
		errChan <- werr
	}()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sendmail command failed: %w", err)
	}

	if err := <-errChan; err != nil {
		return fmt.Errorf("failed to write to sendmail stdin: %w", err)
	}

	return nil
}

// Close shuts down the pool and waits for in-flight work to finish.
func (p *Pool) Close() {
	if p.store != nil {
		close(p.done)
		p.wg.Wait()
	} else {
		close(p.requests)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeSMTP()
	if p.idleTimer != nil {
		p.idleTimer.Stop()
	}
}
