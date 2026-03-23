package mailer

import (
	"fmt"
	"log/slog"
	"math"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hobeone/rss2go/internal/config"
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
type Pool struct {
	requests chan MailRequest
	config   *config.Config
	logger   *slog.Logger
	sender   Sender

	// SMTP persistence management
	mu         sync.Mutex
	smtpDialer *gomail.Dialer
	smtpConn   gomail.SendCloser
	idleTimer  *time.Timer
}

// NewPool creates a new mailer pool.
func NewPool(size int, cfg *config.Config, logger *slog.Logger) *Pool {
	p := &Pool{
		// Use a larger buffer to prevent blocking watcher goroutines
		// if SMTP delivery is slow.
		requests: make(chan MailRequest, size*100),
		config:   cfg,
		logger:   logger.With("component", "mailer"),
	}

	if cfg.SMTPServer != "" {
		p.smtpDialer = gomail.NewDialer(cfg.SMTPServer, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass)
		//p.smtpDialer.SSL = cfg.UseTLS
	}

	p.sender = p.defaultSender

	for i := range size {
		go p.worker(i)
	}

	return p
}

func (p *Pool) worker(id int) {
	p.logger.Debug("starting worker", "worker_id", id)
	for req := range p.requests {
		p.logger.Debug("sending email", "to", req.To, "subject", req.Subject)
		err := p.Send(req)
		if err != nil {
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
// It also increments the metrics on success.
// If an error occurs, it retries with exponential backoff.
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
		p.logger.Debug("opening new SMTP connection",
			"server", p.config.SMTPServer,
		)
		s, err := p.smtpDialer.Dial()
		if err != nil {
			p.logger.Error("failed to dial SMTP server",
				"error", err,
			)
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

	// Reset idle timer to close connection after 3 minutes of inactivity
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

func (p *Pool) closeSMTP() {
	if p.smtpConn != nil {
		p.logger.Debug("closing SMTP connection")
		p.smtpConn.Close()
		p.smtpConn = nil
	}
}

func (p *Pool) sendSMTP(req MailRequest) error {
	// This method is now replaced by persistentSMTPSender
	return p.persistentSMTPSender(req)
}

func (p *Pool) sendSendmail(req MailRequest) error {
	cmd := exec.Command(p.config.Sendmail, "-t")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("To: %s\nSubject: %s\nContent-Type: text/html; charset=UTF-8\n\n%s",
		strings.Join(req.To, ", "), req.Subject, req.Body)

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(msg))
	}()

	return cmd.Run()
}

// Submit sends a mail request to the pool.
func (p *Pool) Submit(req MailRequest) {
	p.requests <- req
}

// Close closes the pool.
func (p *Pool) Close() {
	close(p.requests)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeSMTP()
	if p.idleTimer != nil {
		p.idleTimer.Stop()
	}
}
