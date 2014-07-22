package mail

import (
	"fmt"
	"net/mail"
	"os"
	"os/exec"
	"testing"

	"github.com/hobeone/gophermail"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
)

type MockedMailer struct {
	Called int
}

func (m *MockedMailer) SendMail(msg Message) error {
	m.Called++
	return nil
}

func TestSendToUsersWithNoMailSender(t *testing.T) {
	mr := &MailRequest{}
	md := &MailDispatcher{}
	err := md.handleMailRequest(mr)

	if err == nil {
		t.Errorf("Sending mail with no MailSender should be an error.")
	}
}

func TestSendToUsers(t *testing.T) {
	dbh := db.NewMemoryDBHandle(false, false)
	feeds, users := db.LoadFixtures(t, dbh, "http://localhost")

	mm := &MockedMailer{}
	md := NewMailDispatcher(
		"recipient@test.com",
		mm,
	)

	f := &feed.Feed{
		Url: feeds[0].Url,
	}
	s := &feed.Story{
		Feed: f,
	}
	mr := MailRequest{
		Item: s,
		Addresses: []mail.Address{
			mail.Address{Address: users[0].Email},
			mail.Address{Address: users[1].Email},
		},
	}
	err := md.handleMailRequest(&mr)
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

func (r TestCommandRunner) Run(input []byte) ([]byte, error) {
	cs := []string{fmt.Sprintf("-test.run=%s", r.TestToRun), "--"}
	cs = append(cs)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	out, err := cmd.CombinedOutput()
	return out, err
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
	msg := &MailMessage{
		gophermail.Message{
			From:    mail.Address{Address: "from@example.com"},
			To:      []mail.Address{mail.Address{Address: "to@example.com"}},
			Subject: "Testing subject",
			Body:    "Test Body",
		},
	}

	mta := NewLocalMTASender("/bin/true")
	mta.Runner = TestCommandRunner{"TestHelperProcessSuccess"}

	err := mta.SendMail(msg)
	if err != nil {
		t.Fatalf("Unexpected error on SendMail: %s", err)
	}

	mta.Runner = TestCommandRunner{"TestHelperProcessFail"}

	err = mta.SendMail(msg)
	if err == nil {
		t.Fatalf("Unexpected success with SendMail.")
	}
}
