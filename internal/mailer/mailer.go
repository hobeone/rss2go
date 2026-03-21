package mailer

import (
	"fmt"
	"log/slog"
	"math"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hobe/rss2go/internal/config"
	"github.com/hobe/rss2go/internal/metrics"
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
}

// NewPool creates a new mailer pool.
func NewPool(size int, cfg *config.Config, logger *slog.Logger) *Pool {
	p := &Pool{
		requests: make(chan MailRequest, size*2),
		config:   cfg,
		logger:   logger.With("component", "mailer"),
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
	initialDelay  = 1 * time.Second
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
			delay := time.Duration(math.Pow(2, float64(i))) * initialDelay
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
			p.logger.Warn("failed to send email, retrying", "attempt", i+1, "delay", delay, "error", err)
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("failed to send email after %d retries: %w", maxRetries, err)
}

func (p *Pool) defaultSender(req MailRequest) error {
	if p.config.SMTPServer != "" {
		return p.sendSMTP(req)
	}
	if p.config.Sendmail != "" {
		return p.sendSendmail(req)
	}
	return fmt.Errorf("no mailer configured (SMTP or Sendmail)")
}

func (p *Pool) sendSMTP(req MailRequest) error {
	m := gomail.NewMessage()
	m.SetHeader("From", p.config.SMTPSender)
	m.SetHeader("To", req.To...)
	m.SetHeader("Subject", req.Subject)
	m.SetBody("text/html", req.Body)

	d := gomail.NewDialer(p.config.SMTPServer, p.config.SMTPPort, p.config.SMTPUser, p.config.SMTPPass)
	// Some servers use SSL on 465, others use StartTLS on 587.
	// gomail detects this by default, but we can override it if needed.
	// Actually p.config.UseTLS should map to gomail's SSL if port is 465.
	// For now let's just use the Dialer.

	return d.DialAndSend(m)
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
}
