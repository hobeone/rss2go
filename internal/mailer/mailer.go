package mailer

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"os/exec"
	"strings"
	"sync/atomic"

	"github.com/hobe/rss2go/internal/config"
	"github.com/hobe/rss2go/internal/metrics"
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

	for i := 0; i < size; i++ {
		go p.worker(i)
	}

	return p
}

func (p *Pool) worker(id int) {
	p.logger.Debug("starting worker", "worker_id", id)
	for req := range p.requests {
		p.logger.Debug("sending email", "to", req.To, "subject", req.Subject)
		err := p.sender(req)
		if err != nil {
			p.logger.Error("failed to send email", "to", req.To, "subject", req.Subject, "error", err)
		} else {
			atomic.AddUint64(&metrics.EmailsSentTotal, 1)
		}
	}
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
	auth := smtp.PlainAuth("", p.config.SMTPUser, p.config.SMTPPass, p.config.SMTPServer)
	addr := fmt.Sprintf("%s:%d", p.config.SMTPServer, p.config.SMTPPort)

	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		strings.Join(req.To, ", "), req.Subject, req.Body))

	return smtp.SendMail(addr, auth, p.config.SMTPSender, req.To, msg)
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
