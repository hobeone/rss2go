package notifier

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"os/exec"
	"strings"
	"sync"
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

// defaultSMTPOpTimeout bounds every SMTP command (including the initial
// connect/greeting/STARTTLS/Auth sequence) so a black-holed or silent server
// cannot hang the shared connection indefinitely.
const defaultSMTPOpTimeout = 30 * time.Second

// SMTPSender implements email dispatching via SMTP, reusing a single
// authenticated connection across calls to avoid tripping provider login-rate
// limits (e.g. Gmail's "too many logins").
type SMTPSender struct {
	cfg SMTPConfig
	log *slog.Logger

	// opTimeout bounds every SMTP operation. Unexported and directly
	// settable by same-package tests so hung-connection tests don't need
	// to wait out the real default.
	opTimeout time.Duration

	mu sync.Mutex
	// conn is the raw dial-time connection, kept separately from client
	// because *smtp.Client does not expose it and STARTTLS wraps it in a
	// *tls.Conn — SetDeadline on this reference still bounds the wrapped
	// connection's I/O.
	conn   net.Conn
	client *smtp.Client
}

// NewSMTPSender creates a new SMTPSender with an optional logger.
func NewSMTPSender(cfg SMTPConfig, log ...*slog.Logger) *SMTPSender {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default().With("component", "notifier")
	}
	return &SMTPSender{cfg: cfg, log: l, opTimeout: defaultSMTPOpTimeout}
}

// logger returns s.log, falling back to a default so struct literals built
// without NewSMTPSender (as some tests do) stay nil-safe.
func (s *SMTPSender) logger() *slog.Logger {
	if s.log != nil {
		return s.log
	}
	return slog.Default().With("component", "notifier")
}

// connect dials, greets, STARTTLSes, and authenticates a fresh connection.
// Callers must hold s.mu.
func (s *SMTPSender) connect(ctx context.Context) (*smtp.Client, error) {
	log := s.logger()
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	var conn net.Conn
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var err error

	if s.cfg.Security == SecuritySSL {
		tlsConfig := &tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: tlsConfig}
		conn, err = tlsDialer.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		log.Debug("SMTP connection failed", "addr", addr, "security", s.cfg.Security, "err", err)
		return nil, fmt.Errorf("notifier: smtp dial failed: %w", err)
	}

	// Bound the greeting read + STARTTLS + Auth sequence: none of these
	// have a deadline beyond the TCP handshake otherwise, so a server that
	// accepts the connection but stays silent would hang forever.
	if err := conn.SetDeadline(time.Now().Add(s.opTimeout)); err != nil {
		_ = conn.Close()
		log.Debug("Failed to set SMTP connect deadline", "addr", addr, "err", err)
		return nil, fmt.Errorf("notifier: set connect deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		log.Debug("Failed creating SMTP client", "host", s.cfg.Host, "err", err)
		return nil, fmt.Errorf("notifier: create smtp client: %w", err)
	}

	if s.cfg.Security == SecuritySTARTTLS {
		tlsConfig := &tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			log.Debug("SMTP STARTTLS failed", "host", s.cfg.Host, "err", err)
			return nil, fmt.Errorf("notifier: starttls failed: %w", err)
		}
	}

	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			_ = client.Close()
			log.Debug("SMTP authentication failed", "user", s.cfg.Username, "err", err)
			return nil, fmt.Errorf("notifier: authentication failed: %w", err)
		}
	}

	log.Debug("SMTP connection established", "addr", addr, "security", s.cfg.Security)
	s.conn = conn
	s.client = client
	return client, nil
}

// closeCached discards the cached connection. Once any operation on it times
// out or errors, the underlying tls.Conn (if STARTTLS was used) is
// permanently unusable per crypto/tls's own semantics, so it must never be
// reused after an error — always redial instead of retrying in place.
// Callers must hold s.mu.
func (s *SMTPSender) closeCached() {
	if s.client != nil {
		_ = s.client.Close()
	}
	s.client = nil
	s.conn = nil
}

// getClient returns a live, authenticated client, reusing the cached one
// after a liveness check or dialing fresh otherwise. Callers must hold s.mu.
func (s *SMTPSender) getClient(ctx context.Context) (*smtp.Client, error) {
	if s.client != nil {
		if err := s.conn.SetDeadline(time.Now().Add(s.opTimeout)); err == nil {
			if err := s.client.Noop(); err == nil {
				s.logger().Debug("Reusing cached SMTP connection")
				return s.client, nil
			}
		}
		s.logger().Debug("Cached SMTP connection stale, reconnecting")
		s.closeCached()
	}
	return s.connect(ctx)
}

// Send dispatches an HTML email to recipients via SMTP, reusing a cached
// connection across calls when possible.
func (s *SMTPSender) Send(ctx context.Context, subject string, body string, recipients []string) error {
	if len(recipients) == 0 {
		return fmt.Errorf("notifier: smtp: no recipients specified")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	log := s.logger()
	cleanedSubject := CleanHeader(subject)
	log.Debug("Starting SMTP email delivery", "host", s.cfg.Host, "port", s.cfg.Port, "recipients_count", len(recipients), "subject", cleanedSubject)

	msg := buildMessage(s.cfg.From, recipients, cleanedSubject, body)

	// Holding s.mu across blocking network I/O below is a deliberate
	// exception to this project's "never hold a mutex during I/O" rule
	// (GEMINI.md): the whole point of this type is that all sends share
	// one physical SMTP connection/session, and SMTP is a single-stream
	// protocol — interleaving Mail/Rcpt/Data from two callers on the same
	// connection would corrupt the session, not just contend on a lock.
	// TestSMTPSenderConcurrentSendsSerialize proves this is load-bearing,
	// not incidental.
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}

	if err := s.conn.SetDeadline(time.Now().Add(s.opTimeout)); err != nil {
		return s.discardAndFail("set send deadline", err)
	}

	if err := client.Mail(s.cfg.From); err != nil {
		return s.discardAndFail("set mail from", err)
	}

	for _, to := range recipients {
		if err := client.Rcpt(to); err != nil {
			return s.discardAndFail(fmt.Sprintf("set rcpt to %s", to), err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return s.discardAndFail("open data writer", err)
	}

	if _, err := w.Write(msg); err != nil {
		return s.discardAndFail("write message data", err)
	}

	if err := w.Close(); err != nil {
		return s.discardAndFail("close data writer", err)
	}

	log.Debug("SMTP email delivered successfully", "recipients_count", len(recipients), "bytes", len(msg))
	return nil
}

// discardAndFail discards the cached connection (any error at this point
// means it must not be reused) and wraps err with op for the caller. Callers
// must hold s.mu.
func (s *SMTPSender) discardAndFail(op string, err error) error {
	s.logger().Debug("SMTP operation failed, discarding connection", "op", op, "err", err)
	s.closeCached()
	return fmt.Errorf("notifier: %s: %w", op, err)
}

// SendmailSender implements email dispatching via a local system sendmail command.
type SendmailSender struct {
	path string
	from string
	log  *slog.Logger
}

// NewSendmailSender creates a new SendmailSender with optional logger.
// If path is empty, /usr/sbin/sendmail is used.
func NewSendmailSender(path string, from string, log ...*slog.Logger) *SendmailSender {
	if path == "" {
		path = "/usr/sbin/sendmail"
	}
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default().With("component", "notifier")
	}
	return &SendmailSender{
		path: path,
		from: from,
		log:  l,
	}
}

// Send dispatches an HTML email via the local sendmail command.
func (s *SendmailSender) Send(ctx context.Context, subject string, body string, recipients []string) error {
	if len(recipients) == 0 {
		return fmt.Errorf("notifier: sendmail: no recipients specified")
	}

	log := s.log
	if log == nil {
		log = slog.Default().With("component", "notifier")
	}

	cleanedSubject := CleanHeader(subject)
	log.Debug("Starting sendmail binary delivery", "path", s.path, "recipients_count", len(recipients), "subject", cleanedSubject)

	msg := buildMessage(s.from, recipients, cleanedSubject, body)

	// Invoke local sendmail binary: sendmail -t
	cmd := exec.CommandContext(ctx, s.path, "-t")
	cmd.Stdin = bytes.NewReader(msg)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Debug("Sendmail execution failed", "path", s.path, "stderr", stderr.String(), "err", err)
		return fmt.Errorf("notifier: sendmail failed (stderr: %q): %w", stderr.String(), err)
	}

	log.Debug("Sendmail binary delivery succeeded", "recipients_count", len(recipients), "bytes", len(msg))
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
