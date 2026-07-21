package notifier

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCleanHeader(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Hello World", "Hello World"},
		{"Hello\nWorld\r\n", "HelloWorld"},
		{"Subject\rLine\nInjection", "SubjectLineInjection"},
	}

	for _, tc := range cases {
		if got := CleanHeader(tc.input); got != tc.expected {
			t.Errorf("CleanHeader(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestSendmailSender(t *testing.T) {
	tempDir := t.TempDir()
	mockSendmailPath := filepath.Join(tempDir, "sendmail")
	outputFile := mockSendmailPath + ".out"

	// Create a mock shell script that writes stdin to a file
	scriptContent := fmt.Sprintf("#!/bin/sh\ncat > %s\n", outputFile)
	if err := os.WriteFile(mockSendmailPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write mock sendmail: %v", err)
	}

	sender := NewSendmailSender(mockSendmailPath, "sender@test.com")

	ctx := context.Background()
	recipients := []string{"recipient1@test.com", "recipient2@test.com"}
	subject := "Test Subject\nWith Injection" // Injection should be stripped
	body := "<h1>HTML Body</h1>"

	if err := sender.Send(ctx, subject, body, recipients); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output file content
	outBytes, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read mock sendmail output: %v", err)
	}

	output := string(outBytes)

	if !strings.Contains(output, "From: sender@test.com") {
		t.Errorf("output missing From: %s", output)
	}
	if !strings.Contains(output, "To: recipient1@test.com, recipient2@test.com") {
		t.Errorf("output missing To: %s", output)
	}
	if !strings.Contains(output, "Subject: Test SubjectWith Injection") { // Stripped \n
		t.Errorf("output subject not sanitized or missing: %s", output)
	}
	if !strings.Contains(output, "Content-Type: text/html; charset=UTF-8") {
		t.Errorf("output missing Content-Type: %s", output)
	}
	if !strings.Contains(output, "<h1>HTML Body</h1>") {
		t.Errorf("output missing body: %s", output)
	}
}

// startMockSMTPServer starts a local TCP server that simulates basic SMTP exchanges.
func startMockSMTPServer(t *testing.T) (string, func()) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	shutdown := make(chan struct{})
	var serverWg sync.WaitGroup

	serverWg.Go(func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				select {
				case <-shutdown:
					return
				default:
					t.Errorf("SMTP accept error: %v", err)
					return
				}
			}

			go handleSMTPConn(conn)
		}
	})

	closeFn := func() {
		close(shutdown)
		_ = l.Close()
		serverWg.Wait()
	}

	return l.Addr().String(), closeFn
}

func handleSMTPConn(c net.Conn) {
	defer func() { _ = c.Close() }()

	reader := bufio.NewReader(c)
	tp := textproto.NewReader(reader)

	// Greeting
	_, _ = c.Write([]byte("220 smtp.mock.test\r\n"))

	authed := false

	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}

		cmd := strings.ToUpper(line)
		if strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO") {
			_, _ = c.Write([]byte("250-smtp.mock.test Hello\r\n250 AUTH PLAIN\r\n"))
		} else if strings.HasPrefix(cmd, "AUTH PLAIN") {
			authed = true
			_, _ = c.Write([]byte("235 2.7.0 Authentication successful\r\n"))
		} else if strings.HasPrefix(cmd, "MAIL FROM:") {
			if !authed {
				_, _ = c.Write([]byte("530 5.7.0 Must issue a STARTTLS or AUTH command first\r\n"))
			} else {
				_, _ = c.Write([]byte("250 2.1.0 Ok\r\n"))
			}
		} else if strings.HasPrefix(cmd, "RCPT TO:") {
			_, _ = c.Write([]byte("250 2.1.5 Ok\r\n"))
		} else if cmd == "DATA" {
			_, _ = c.Write([]byte("354 Start mail input; end with <CR><LF>.<CR><LF>\r\n"))
			// Read mail body until "."
			for {
				bodyLine, err := tp.ReadLine()
				if err != nil {
					return
				}
				if bodyLine == "." {
					break
				}
			}
			_, _ = c.Write([]byte("250 2.0.0 Ok: queued\r\n"))
		} else if cmd == "QUIT" {
			_, _ = c.Write([]byte("221 2.0.0 Bye\r\n"))
			return
		} else {
			_, _ = c.Write([]byte("500 5.5.1 Command unrecognized\r\n"))
		}
	}
}

func TestSMTPSenderHappyPath(t *testing.T) {
	addr, closeFn := startMockSMTPServer(t)
	defer closeFn()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split host/port: %v", err)
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}

	cfg := SMTPConfig{
		Host:     host,
		Port:     port,
		Username: "user",
		Password: "pass",
		From:     "sender@test.com",
		Security: SecurityNone,
	}

	sender := NewSMTPSender(cfg)
	err = sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"})
	if err != nil {
		t.Fatalf("SMTP send failed: %v", err)
	}
}

func TestNilLoggerGuards(t *testing.T) {
	addr, closeFn := startMockSMTPServer(t)
	defer closeFn()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split host/port: %v", err)
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}

	// Directly initialize SMTPSender with nil log field (struct literal without NewSMTPSender)
	smtpSender := &SMTPSender{
		cfg: SMTPConfig{
			Host:     host,
			Port:     port,
			Username: "user",
			Password: "pass",
			From:     "sender@test.com",
			Security: SecurityNone,
		},
		log: nil,
	}

	if err := smtpSender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"}); err != nil {
		t.Fatalf("SMTPSender.Send with nil logger failed: %v", err)
	}

	// Directly initialize SendmailSender with nil log field
	tempDir := t.TempDir()
	mockSendmailPath := filepath.Join(tempDir, "sendmail")
	scriptContent := fmt.Sprintf("#!/bin/sh\ncat > %s.out\n", mockSendmailPath)
	if err := os.WriteFile(mockSendmailPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write mock sendmail: %v", err)
	}

	sendmailSender := &SendmailSender{
		path: mockSendmailPath,
		from: "sender@test.com",
		log:  nil,
	}

	if err := sendmailSender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"}); err != nil {
		t.Fatalf("SendmailSender.Send with nil logger failed: %v", err)
	}
}

func TestSMTPSenderDataCloseError(t *testing.T) {
	// Start a mock server that returns 554 error after receiving "." in DATA
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = l.Close() }()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		reader := bufio.NewReader(conn)
		tp := textproto.NewReader(reader)

		_, _ = conn.Write([]byte("220 smtp.mock.test\r\n"))

		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			cmd := strings.ToUpper(line)
			if strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO") {
				_, _ = conn.Write([]byte("250-smtp.mock.test Hello\r\n250 AUTH PLAIN\r\n"))
			} else if strings.HasPrefix(cmd, "AUTH PLAIN") {
				_, _ = conn.Write([]byte("235 2.7.0 Authentication successful\r\n"))
			} else if strings.HasPrefix(cmd, "MAIL FROM:") {
				_, _ = conn.Write([]byte("250 2.1.0 Ok\r\n"))
			} else if strings.HasPrefix(cmd, "RCPT TO:") {
				_, _ = conn.Write([]byte("250 2.1.5 Ok\r\n"))
			} else if cmd == "DATA" {
				_, _ = conn.Write([]byte("354 Start mail input; end with <CR><LF>.<CR><LF>\r\n"))
				for {
					bodyLine, err := tp.ReadLine()
					if err != nil {
						return
					}
					if bodyLine == "." {
						break
					}
				}
				// Reject data termination with 554
				_, _ = conn.Write([]byte("554 5.7.1 Transaction failed / rejected by policy\r\n"))
				return
			}
		}
	}()

	host, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host/port: %v", err)
	}
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	cfg := SMTPConfig{
		Host:     host,
		Port:     port,
		Username: "user",
		Password: "pass",
		From:     "sender@test.com",
		Security: SecurityNone,
	}

	sender := NewSMTPSender(cfg)
	err = sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"})
	if err == nil {
		t.Fatalf("expected error on SMTP DATA termination failure, got nil")
	}
	if !strings.Contains(err.Error(), "close data writer") {
		t.Errorf("expected error message to contain 'close data writer', got: %v", err)
	}
}
