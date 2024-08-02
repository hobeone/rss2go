package mail

import (
	"fmt"
	"io"
	"net/mail"
	"os"
	"os/exec"
	"testing"

	"gopkg.in/gomail.v2"

	"github.com/hobeone/rss2go/db"
	"github.com/mmcdole/gofeed"
	"github.com/sirupsen/logrus"
)

func NullLogger() logrus.FieldLogger {
	l := logrus.New()
	l.Out = io.Discard
	return l
}

type MockedMailer struct {
	Called int
}

func (m *MockedMailer) SendMail(msg *gomail.Message) error {
	m.Called++
	return nil
}

func TestSendToUsersWithNoMailSender(t *testing.T) {
	mr := &Request{}
	md := &Dispatcher{}
	err := md.handleMailRequest(mr)

	if err == nil {
		t.Errorf("Sending mail with no MailSender should be an error.")
	}
}

func TestSendToUsers(t *testing.T) {
	dbh := db.NewMemoryDBHandle(NullLogger(), true)
	users, err := dbh.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}

	mm := &MockedMailer{}
	md := NewDispatcher(
		"recipient@test.com",
		mm,
	)

	s := &gofeed.Item{}
	mr := Request{
		Item: s,
		Addresses: []mail.Address{
			{Address: users[0].Email},
			{Address: users[1].Email},
		},
	}
	err = md.handleMailRequest(&mr)
	if err != nil {
		t.Fatalf("Error sending to users: %s", err)
	}
	if mm.Called != 2 {
		t.Fatalf("Expected 2 calls to the mailer, got %d", mm.Called)
	}
}

type TestCommandRunner struct {
	TestToRun string
}

func (r TestCommandRunner) Run(from string, to []string, msg []byte) error {
	cs := []string{fmt.Sprintf("-test.run=%s", r.TestToRun), "--"}
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	_, err := cmd.CombinedOutput()
	return err
}

func TestHelperProcessSuccess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	fmt.Println("testing helper process")
}

func TestHelperProcessFail(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(1)
	fmt.Println("testing helper process")
}

func TestLocalMTASender(t *testing.T) {
	gmsg := gomail.NewMessage()
	gmsg.SetHeader("From", "from@example.com")
	gmsg.SetHeader("To", "to@example.com")
	gmsg.SetHeader("Subject", "Testing subject")
	gmsg.SetBody("text/html", "Test Body")

	mta := NewLocalMTASender("/bin/true")
	mta.Runner = TestCommandRunner{"TestHelperProcessSuccess"}

	err := mta.SendMail(gmsg)
	if err != nil {
		t.Fatalf("Unexpected error on SendMail: %s", err)
	}

	mta.Runner = TestCommandRunner{"TestHelperProcessFail"}

	err = mta.SendMail(gmsg)
	if err == nil {
		t.Fatalf("Unexpected success with SendMail.")
	}
}

type testDialer struct {
	Called int
	Opened bool
}

func (d *testDialer) Dial() (gomail.SendCloser, error) {
	d.Called++
	d.Opened = true
	return &testSender{}, nil
}

type testSender struct{}

func (s *testSender) Send(from string, to []string, msg io.WriterTo) error {
	return nil
}

func (s *testSender) Close() error {
	return nil
}

func TestSMTPSender(t *testing.T) {
	gmsg := gomail.NewMessage()
	gmsg.SetHeader("From", "from@example.com")
	gmsg.SetHeader("To", "to@example.com")
	gmsg.SetHeader("Subject", "Testing subject")
	gmsg.SetBody("text/html", "Test Body")

	s := &SMTPSender{
		Hostname: "foo",
		Port:     1234,
		Username: "user",
		Password: "pwd",
		reqChan:  make(chan smtpRequest),
		dialer:   &testDialer{},
	}
	go s.smtpDaemon()
	err := s.SendMail(gmsg)
	if err != nil {
		t.Fatalf("Unexpected error on sendmail: %v", err)
	}
}
