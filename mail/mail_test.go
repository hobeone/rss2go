package mail

import (
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
	"github.com/jpoehls/gophermail"
	"testing"
)

type MockedMailer struct {
	Called int
}

func (m *MockedMailer) SendMail(msg *gophermail.Message) error {
	m.Called++
	return nil
}

func TestSendToUsersWithNoMailSender(t *testing.T) {
	s := &feed.Story{}

	md := &MailDispatcher{}
	err := md.sendToUsers(s)

	if err == nil {
		t.Errorf("Sending mail with no MailSender should be an error.")
	}
}

func TestSendToUsers(t *testing.T) {
	dbh := db.NewMemoryDbDispatcher(false, false)
	feed1, err := dbh.AddFeed("test1", "http://foo.bar/")
	if err != nil {
		t.Fatalf("Error creating test feed: %s", err)
	}

	user1, err := dbh.AddUser("name", "email@example.com", "pass")
	if err != nil {
		t.Fatalf("Error creating test user: %s", err)
	}
	user2, err := dbh.AddUser("nam2", "email2@example.com", "pass")
	if err != nil {
		t.Fatalf("Error creating test user: %s", err)
	}

	err = dbh.AddFeedsToUser(user1, []*db.FeedInfo{feed1})
	if err != nil {
		t.Fatalf("Error adding feeds to user: %s", err)
	}
	err = dbh.AddFeedsToUser(user2, []*db.FeedInfo{feed1})
	if err != nil {
		t.Fatalf("Error adding feeds to user: %s", err)
	}

	mm := new(MockedMailer)
	md := NewMailDispatcher(
		dbh,
		"recipient@test.com",
		mm,
	)

	f := &feed.Feed{
		Url: feed1.Url,
	}
	s := &feed.Story{
		Feed: f,
	}
	err = md.sendToUsers(s)
	if err != nil {
		t.Fatalf("Error sending to users: %s", err)
	}
	if mm.Called != 2 {
		t.Fatalf("Expected 2 calls to the mailer, got %d", mm.Called)
	}
}

func TestLocalMTASender(t *testing.T) {
	msg := &gophermail.Message{
		From:    "from@example.com",
		To:      []string{"to@example.com"},
		Subject: "Testing subject",
		Body:    "Test Body",
	}

	mta := NewLocalMTASender("/bin/true")
	err := mta.SendMail(msg)
	if err != nil {
		t.Fatalf("Error sending mail with /bin/true which should always work. Err: %s", err)
	}

	mta = NewLocalMTASender("/bin/false")
	err = mta.SendMail(msg)
	if err == nil {
		t.Fatalf("Sending mail with /bin/false which should always work.")
	}
}
