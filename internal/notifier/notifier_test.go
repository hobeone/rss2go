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
	"sync/atomic"
	"testing"
	"time"
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

// mockConn pairs an accepted server-side connection with a per-connection
// silence flag, so a test can make exactly one connection stop responding
// (simulating a stale/hung cached connection) without affecting any other
// connection accepted before or after it.
type mockConn struct {
	conn     net.Conn
	silenced atomic.Bool
}

// mockSMTPServer simulates basic SMTP exchanges for tests. It tracks AUTH
// invocations and each accepted server-side connection so tests can force-
// close or silence a specific connection to exercise SMTPSender's reconnect
// and deadline handling.
type mockSMTPServer struct {
	addr      string
	authCount atomic.Int64

	// rejectAuth, when set, makes every AUTH PLAIN fail with a permanent
	// error instead of succeeding.
	rejectAuth atomic.Bool

	mu    sync.Mutex
	conns []*mockConn
	// rejectRcpt, when non-empty, makes RCPT TO for exactly this address
	// fail with a permanent error instead of succeeding. Guarded by mu
	// (read/written far less often than conns, sharing the lock is fine).
	rejectRcpt string

	wg sync.WaitGroup
}

func (srv *mockSMTPServer) setRejectRcpt(addr string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.rejectRcpt = addr
}

func (srv *mockSMTPServer) getRejectRcpt() string {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.rejectRcpt
}

// startMockSMTPServer starts a local TCP server that simulates basic SMTP
// exchanges.
func startMockSMTPServer(t *testing.T) *mockSMTPServer {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := &mockSMTPServer{addr: l.Addr().String()}

	srv.wg.Go(func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}

			mc := &mockConn{conn: conn}
			srv.mu.Lock()
			srv.conns = append(srv.conns, mc)
			srv.mu.Unlock()

			srv.wg.Go(func() {
				srv.handleConn(mc)
			})
		}
	})

	t.Cleanup(func() {
		_ = l.Close()
		srv.mu.Lock()
		for _, mc := range srv.conns {
			_ = mc.conn.Close()
		}
		srv.mu.Unlock()
		srv.wg.Wait()
	})

	return srv
}

// connAt returns the i'th server-side connection accepted so far, or nil.
func (srv *mockSMTPServer) connAt(i int) net.Conn {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if i >= len(srv.conns) {
		return nil
	}
	return srv.conns[i].conn
}

// silenceConnAt makes the i'th accepted connection stop responding to any
// further commands, without closing the socket — simulating a server that
// goes silent mid-session. Only that specific connection is affected;
// connections accepted before or after it behave normally.
func (srv *mockSMTPServer) silenceConnAt(i int) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if i < len(srv.conns) {
		srv.conns[i].silenced.Store(true)
	}
}

func (srv *mockSMTPServer) handleConn(mc *mockConn) {
	c := mc.conn
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

		// Once armed, stop responding to anything further on this
		// specific connection, simulating a server that goes silent
		// mid-session without closing the socket.
		if mc.silenced.Load() {
			continue
		}

		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO"):
			_, _ = c.Write([]byte("250-smtp.mock.test Hello\r\n250 AUTH PLAIN\r\n"))
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			if srv.rejectAuth.Load() {
				_, _ = c.Write([]byte("535 5.7.8 Authentication credentials invalid\r\n"))
				break
			}
			authed = true
			srv.authCount.Add(1)
			_, _ = c.Write([]byte("235 2.7.0 Authentication successful\r\n"))
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			if !authed {
				_, _ = c.Write([]byte("530 5.7.0 Must issue a STARTTLS or AUTH command first\r\n"))
			} else {
				_, _ = c.Write([]byte("250 2.1.0 Ok\r\n"))
			}
		case strings.HasPrefix(cmd, "RCPT TO:"):
			if reject := srv.getRejectRcpt(); reject != "" && strings.Contains(cmd, strings.ToUpper(reject)) {
				_, _ = c.Write([]byte("550 5.1.1 User unknown\r\n"))
			} else {
				_, _ = c.Write([]byte("250 2.1.5 Ok\r\n"))
			}
		case cmd == "NOOP":
			_, _ = c.Write([]byte("250 2.0.0 Ok\r\n"))
		case cmd == "DATA":
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
		case cmd == "QUIT":
			_, _ = c.Write([]byte("221 2.0.0 Bye\r\n"))
			return
		default:
			_, _ = c.Write([]byte("500 5.5.1 Command unrecognized\r\n"))
		}
	}
}

// startSilentListener accepts TCP connections but never writes a byte,
// simulating a server that accepts the connection but never sends the SMTP
// greeting — used to test connect()'s own deadline bound.
func startSilentListener(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	var mu sync.Mutex
	var conns []net.Conn
	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	})

	t.Cleanup(func() {
		_ = l.Close()
		mu.Lock()
		for _, c := range conns {
			_ = c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})

	return l.Addr().String()
}

func newTestSMTPSender(t *testing.T, addr string) *SMTPSender {
	t.Helper()

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

	return NewSMTPSender(cfg)
}

func TestSMTPSenderHappyPath(t *testing.T) {
	srv := startMockSMTPServer(t)

	sender := newTestSMTPSender(t, srv.addr)
	err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"})
	if err != nil {
		t.Fatalf("SMTP send failed: %v", err)
	}
}

func TestNilLoggerGuards(t *testing.T) {
	srv := startMockSMTPServer(t)

	host, portStr, err := net.SplitHostPort(srv.addr)
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
		log:       nil,
		opTimeout: defaultSMTPOpTimeout,
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

// TestSMTPSenderConnectionReuse proves consecutive Send() calls reuse the
// cached connection instead of dialing and authenticating fresh each time.
func TestSMTPSenderConnectionReuse(t *testing.T) {
	srv := startMockSMTPServer(t)
	sender := newTestSMTPSender(t, srv.addr)

	for i := range 3 {
		if err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"}); err != nil {
			t.Fatalf("send %d failed: %v", i, err)
		}
	}

	if got := srv.authCount.Load(); got != 1 {
		t.Errorf("expected 1 AUTH across 3 sequential sends, got %d", got)
	}
}

// TestSMTPSenderConcurrentSendsSerialize proves concurrent Send() calls are
// serialized onto the single cached connection without protocol corruption,
// and still only authenticate once.
func TestSMTPSenderConcurrentSendsSerialize(t *testing.T) {
	srv := startMockSMTPServer(t)
	sender := newTestSMTPSender(t, srv.addr)

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := range n {
		wg.Go(func() {
			errs[i] = sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"})
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("send %d failed: %v", i, err)
		}
	}

	if got := srv.authCount.Load(); got != 1 {
		t.Errorf("expected 1 AUTH across %d concurrent sends, got %d", n, got)
	}
}

// TestSMTPSenderReconnectAfterStaleConnection proves that when the cached
// connection dies server-side, the next Send() detects it and redials.
func TestSMTPSenderReconnectAfterStaleConnection(t *testing.T) {
	srv := startMockSMTPServer(t)
	sender := newTestSMTPSender(t, srv.addr)

	if err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"}); err != nil {
		t.Fatalf("first send failed: %v", err)
	}

	conn := srv.connAt(0)
	if conn == nil {
		t.Fatal("expected a server-side connection to have been accepted")
	}
	_ = conn.Close()

	if err := sender.Send(context.Background(), "Hello again", "<p>Test</p>", []string{"recipient@test.com"}); err != nil {
		t.Fatalf("second send failed after forced disconnect: %v", err)
	}

	if got := srv.authCount.Load(); got != 2 {
		t.Errorf("expected 2 AUTH (one per physical connection), got %d", got)
	}
}

// TestSMTPSenderHungConnectBound proves connect() itself is bounded by
// opTimeout when the server accepts the TCP connection but never sends the
// greeting.
func TestSMTPSenderHungConnectBound(t *testing.T) {
	addr := startSilentListener(t)
	sender := newTestSMTPSender(t, addr)
	sender.opTimeout = 100 * time.Millisecond

	start := time.Now()
	err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from a silent server, got nil")
	}
	if elapsed > time.Second {
		t.Errorf("expected Send to bound by ~opTimeout, took %v", elapsed)
	}

	// A subsequent send against a healthy server must still succeed.
	srv := startMockSMTPServer(t)
	sender2 := newTestSMTPSender(t, srv.addr)
	if err := sender2.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"}); err != nil {
		t.Fatalf("send against healthy server failed: %v", err)
	}
}

// TestSMTPSenderHungReuseBound proves that if the cached connection goes
// silent mid-session (without closing the socket), the Noop liveness check
// in getClient catches it — bounded by opTimeout, not hanging — and Send()
// transparently discards the stale connection and redials within the same
// call, so the send still succeeds rather than surfacing an error to the
// caller. This is a better outcome than a bare bounded error: the discard-
// on-any-error path is still the deeper safety net for staleness the Noop
// check misses (e.g. a hang mid-transaction, after Noop already succeeded),
// but the common case of a liveness-checkable stale connection self-heals
// invisibly.
func TestSMTPSenderHungReuseBound(t *testing.T) {
	srv := startMockSMTPServer(t)
	sender := newTestSMTPSender(t, srv.addr)
	sender.opTimeout = 100 * time.Millisecond

	if err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"}); err != nil {
		t.Fatalf("first send failed: %v", err)
	}

	// Silence only the one connection the first Send() established and
	// cached — a new connection dialed later must NOT inherit this.
	srv.silenceConnAt(0)

	start := time.Now()
	err := sender.Send(context.Background(), "Hello again", "<p>Test</p>", []string{"recipient@test.com"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected transparent redial to succeed despite the stale cached connection, got: %v", err)
	}
	if elapsed > time.Second {
		t.Errorf("expected the stale-connection detection + redial to bound by ~opTimeout, took %v", elapsed)
	}
	if elapsed < sender.opTimeout {
		t.Errorf("expected elapsed time to include the Noop liveness-check timeout (~%v), got %v", sender.opTimeout, elapsed)
	}

	if got := srv.authCount.Load(); got != 2 {
		t.Errorf("expected 2 AUTH (initial connection + redial after detecting staleness), got %d", got)
	}
}

// TestSMTPSenderRcptRejected proves a mid-transaction SMTP rejection (as
// opposed to a connection-level failure) still discards the cached
// connection via discardAndFail, so the next Send() redials rather than
// reusing a connection left in an undefined transaction state.
func TestSMTPSenderRcptRejected(t *testing.T) {
	srv := startMockSMTPServer(t)
	srv.setRejectRcpt("bad@test.com")
	sender := newTestSMTPSender(t, srv.addr)

	err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"bad@test.com"})
	if err == nil {
		t.Fatal("expected an error for a rejected recipient, got nil")
	}

	// A later send to a valid recipient must redial cleanly rather than
	// reuse the connection the rejection left mid-transaction.
	srv.setRejectRcpt("")
	if err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"good@test.com"}); err != nil {
		t.Fatalf("send after rejection failed: %v", err)
	}

	if got := srv.authCount.Load(); got != 2 {
		t.Errorf("expected 2 AUTH (rejection discarded the first connection, forcing a redial), got %d", got)
	}
}

// TestSMTPSenderAuthRejected proves a connect()-phase Auth failure returns a
// clean error with nothing cached, and does not corrupt state for a later
// send against a healthy configuration.
func TestSMTPSenderAuthRejected(t *testing.T) {
	srv := startMockSMTPServer(t)
	srv.rejectAuth.Store(true)
	sender := newTestSMTPSender(t, srv.addr)

	err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"})
	if err == nil {
		t.Fatal("expected an error for rejected authentication, got nil")
	}
	if sender.client != nil {
		t.Error("expected no client to be cached after a failed Auth")
	}

	srv.rejectAuth.Store(false)
	if err := sender.Send(context.Background(), "Hello", "<p>Test</p>", []string{"recipient@test.com"}); err != nil {
		t.Fatalf("send after auth rejection cleared failed: %v", err)
	}
}
