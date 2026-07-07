package notifier

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os/exec"
	"strings"
	"time"
)

// SecurityType defines SMTP connection transport security configurations.
type SecurityType string

const (
	SecurityNone     SecurityType = "none"
	SecuritySTARTTLS SecurityType = "starttls"
	SecuritySSL      SecurityType = "ssl"
)

// SMTPConfig holds configuration for the SMTP dispatcher.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	Security SecurityType
}

// Sender defines the interface for dispatching emails.
type Sender interface {
	Send(ctx context.Context, subject string, body string, recipients []string) error
}

// SMTPSender implements email dispatching via SMTP.
type SMTPSender struct {
	cfg SMTPConfig
}

// NewSMTPSender creates a new SMTPSender.
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

// Send dispatches an HTML email to recipients via SMTP.
func (s *SMTPSender) Send(ctx context.Context, subject string, body string, recipients []string) error {
	if len(recipients) == 0 {
		return fmt.Errorf("notifier: smtp: no recipients specified")
	}

	cleanedSubject := CleanHeader(subject)
	msg := buildMessage(s.cfg.From, recipients, cleanedSubject, body)

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	// Dial connection
	var conn net.Conn
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var err error

	if s.cfg.Security == SecuritySSL {
		tlsConfig := &tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("notifier: smtp dial failed: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("notifier: create smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	// STARTTLS handshake
	if s.cfg.Security == SecuritySTARTTLS {
		tlsConfig := &tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("notifier: starttls failed: %w", err)
		}
	}

	// Authenticate
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("notifier: authentication failed: %w", err)
		}
	}

	// Set sender and recipients
	if err := client.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("notifier: set mail from: %w", err)
	}

	for _, to := range recipients {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("notifier: set rcpt to %s: %w", to, err)
		}
	}

	// Write body
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("notifier: open data writer: %w", err)
	}
	defer func() { _ = w.Close() }()

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("notifier: write message data: %w", err)
	}

	return nil
}

// SendmailSender implements email dispatching via a local system sendmail command.
type SendmailSender struct {
	path string
	from string
}

// NewSendmailSender creates a new SendmailSender.
// If path is empty, /usr/sbin/sendmail is used.
func NewSendmailSender(path string, from string) *SendmailSender {
	if path == "" {
		path = "/usr/sbin/sendmail"
	}
	return &SendmailSender{
		path: path,
		from: from,
	}
}

// Send dispatches an HTML email via the local sendmail command.
func (s *SendmailSender) Send(ctx context.Context, subject string, body string, recipients []string) error {
	if len(recipients) == 0 {
		return fmt.Errorf("notifier: sendmail: no recipients specified")
	}

	cleanedSubject := CleanHeader(subject)
	msg := buildMessage(s.from, recipients, cleanedSubject, body)

	// Invoke local sendmail binary: sendmail -t
	cmd := exec.CommandContext(ctx, s.path, "-t")
	cmd.Stdin = bytes.NewReader(msg)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("notifier: sendmail failed (stderr: %q): %w", stderr.String(), err)
	}

	return nil
}

// CleanHeader strips line breaks to prevent SMTP header injection.
func CleanHeader(val string) string {
	val = strings.ReplaceAll(val, "\n", "")
	val = strings.ReplaceAll(val, "\r", "")
	return val
}

// buildMessage formats the MIME HTML email raw bytes.
func buildMessage(from string, to []string, subject string, body string) []byte {
	var buf bytes.Buffer
	_, _ = fmt.Fprintf(&buf, "From: %s\r\n", from)
	_, _ = fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(to, ", "))
	_, _ = fmt.Fprintf(&buf, "Subject: %s\r\n", subject)
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(body)
	return buf.Bytes()
}
