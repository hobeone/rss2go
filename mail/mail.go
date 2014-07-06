// Wrap mail sending functionality.
//
// Can send either by calling a local sendmail binary or connecting to a SMTP
// server.
//
// Designed to run as a goroutine and centralize mail sending.
//
// TODO: add mail batching support

package mail

import (
	"bytes"
	"fmt"
	"net/mail"
	"net/smtp"
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/hobeone/gophermail" // Fix for subjects showing up quoted after go1.3
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed"
)

const MTA_BINARY = "sendmail"

type MailRequest struct {
	Item       *feed.Story
	ResultChan chan error
}

type MailSender interface {
	SendMail(*gophermail.Message) error
}

type CommandRunner interface {
	Run([]byte) ([]byte, error)
}

type SendmailRunner struct {
	SendmailPath string
}

func (r SendmailRunner) Run(input []byte) ([]byte, error) {
	cmd := exec.Command(r.SendmailPath, "-t", "-i")
	cmd.Stdin = bytes.NewReader(input)
	glog.Infof("Running command %#v", cmd)
	return cmd.CombinedOutput()
}

type LocalMTASender struct {
	Runner CommandRunner
}

func (self *LocalMTASender) SendMail(msg *gophermail.Message) error {
	msgBytes, err := msg.Bytes()
	if err != nil {
		return fmt.Errorf("error converting message to text: %s", err.Error())
	}
	_, err = self.Runner.Run(msgBytes)
	if err != nil {
		return fmt.Errorf("error running command %s", err.Error())
	}
	glog.Infof("Successfully sent mail: %s to %v", msg.Subject, msg.To)
	return nil
}

func NewLocalMTASender(mtaPath string) *LocalMTASender {
	if mtaPath == "" {
		mtaPath = MTA_BINARY
	}

	c, err := exec.LookPath(mtaPath)
	if err != nil {
		panic(fmt.Sprintf("Couldn't find specified MTA: %s", err.Error()))
	} else {
		glog.Infof("Found %s at %s.", mtaPath, c)
	}
	mtaPath = c

	return &LocalMTASender{
		Runner: SendmailRunner{mtaPath},
	}
}

type SMTPSender struct {
	SmtpServer   string
	SmtpUsername string
	SmtpPassword string
}

func (self *SMTPSender) SendMail(msg *gophermail.Message) error {
	server_parts := strings.SplitN(self.SmtpServer, ":", 2)
	a := smtp.PlainAuth("", self.SmtpUsername, self.SmtpPassword, server_parts[0])
	err := gophermail.SendMail(self.SmtpServer, a, msg)

	if err != nil {
		return fmt.Errorf("Error sending mail: %s\n", err.Error())
	}
	return nil
}

func NewSMTPSender(server string, username string, password string) MailSender {
	return &SMTPSender{}
}

type MailDispatcher struct {
	OutgoingMail chan *MailRequest
	Dbh          *db.DBHandle
	FromAddress  string
	MailSender   MailSender
}

func NewMailDispatcher(dbh *db.DBHandle, from_address string, sender MailSender) *MailDispatcher {
	return &MailDispatcher{
		OutgoingMail: make(chan *MailRequest),
		Dbh:          dbh,
		FromAddress:  from_address,
		MailSender:   sender,
	}
}

type NullMailSender struct{}

func (self *NullMailSender) SendMail(m *gophermail.Message) error {
	glog.Infof("NullMailer faked sending mail: %s to %v", m.Subject, m.To)
	return nil
}

func CreateAndStartMailer(dbh *db.DBHandle, config *config.Config) *MailDispatcher {
	// Create mail sender
	var sender MailSender

	if !config.Mail.SendMail {
		glog.Info("Using null mail sender as configured.")
		sender = &NullMailSender{}
	} else if config.Mail.MtaPath != "" {
		sender = NewLocalMTASender(config.Mail.MtaPath)
	} else if config.Mail.SmtpServer != "" {
		sender = NewSMTPSender(config.Mail.SmtpServer, config.Mail.SmtpUsername, config.Mail.SmtpPassword)
	} else {
		panic(fmt.Sprint("No mail sending capability defined in config."))
	}

	// Start Mailer
	mailer := NewMailDispatcher(
		dbh,
		config.Mail.FromAddress,
		sender,
	)
	glog.Infof("Created new mailer: %#v", mailer)
	go mailer.DispatchLoop()
	return mailer
}

func CreateAndStartStubMailer() *MailDispatcher {
	// Start Mailer
	mailer := NewMailDispatcher(
		db.NewMemoryDBHandle(false, false),
		"from@example.com",
		&NullMailSender{},
	)
	go mailer.DispatchLoop()
	return mailer
}

func (self *MailDispatcher) DispatchLoop() {
	for {
		request := <-self.OutgoingMail
		request.ResultChan <- self.sendToUsers(request.Item)
	}
}

func (self *MailDispatcher) getUsersForFeed(feed_url string) ([]db.User, error) {
	return self.Dbh.GetFeedUsers(feed_url)
}

func (self *MailDispatcher) sendToUsers(m *feed.Story) error {
	if self.MailSender == nil {
		return fmt.Errorf("No MailSender set, can not send mail.")
	}
	// email addresses subscribed to this feed -> []*db.User
	users, err := self.getUsersForFeed(m.Feed.Url)
	if err != nil {
		return err
	}
	for _, u := range users {
		msg := CreateMailFromItem(self.FromAddress, u.Email, m)
		err = self.MailSender.SendMail(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateMailFromItem(from string, to string, item *feed.Story) *gophermail.Message {
	content := FormatMessageBody(item)
	msg := &gophermail.Message{
		From: mail.Address{
			Address: from,
		},
		To:       []mail.Address{mail.Address{Address: to}},
		Subject:  item.Title,
		Body:     content, //TODO Convert to plain text
		HTMLBody: content,
		Headers:  mail.Header{},
	}
	if !item.Published.IsZero() {
		msg.Headers["Date"] = []string{item.Published.UTC().Format(time.RFC822)}
	}
	return msg
}

func FormatMessageBody(story *feed.Story) string {
	orig_link := fmt.Sprintf(`
<div class="original_link">
<a href="%s">%s</a>
</div><hr>`, story.Link, story.Title)

	return fmt.Sprintf("%s%s", orig_link, story.Content)
}
