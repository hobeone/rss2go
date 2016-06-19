package mail

import (
	"fmt"
	"io/ioutil"
	"net/mail"
	"os"
	"os/exec"
	"testing"

	"gopkg.in/gomail.v2"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
)

func NullLogger() logrus.FieldLogger {
	l := logrus.New()
	l.Out = ioutil.Discard
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
	dbh := db.NewMemoryDBHandle(false, NullLogger(), true)
	users, err := dbh.GetAllUsers()
	if err != nil {
		t.Fatalf("Error getting users: %v", err)
	}
	feeds, err := dbh.GetAllFeeds()
	if err != nil {
		t.Fatalf("Error getting feeds: %v", err)
	}

	mm := &MockedMailer{}
	md := NewDispatcher(
		"recipient@test.com",
		mm,
	)

	f := &feed.Feed{
		URL: feeds[0].URL,
	}
	s := &feed.Story{
		Feed: f,
	}
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
	cs = append(cs)
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
