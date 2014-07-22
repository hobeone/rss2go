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
	"github.com/hobeone/rss2go/feed"
)

// Default MTA binary to exec when sending mail locally.
// sendmail is supported by sendmail and postfix
const MTA_BINARY = "sendmail"

// Message defines the interface that mail messages should implement
type Message interface {
	Bytes() ([]byte, error)
	MailSubject() string
	MailTo() []mail.Address
	MailFrom() *mail.Address
}

// MailMessage wraps gophermail.Message for the Message interface
type MailMessage struct {
	gophermail.Message
}

// MailSubject returns the Subject of a message
func (m *MailMessage) MailSubject() string {
	return m.Subject
}

// MailTo returns the To addresses of an email
func (m *MailMessage) MailTo() []mail.Address {
	return m.To
}

// MailFrom returns the From address of an email
func (m *MailMessage) MailFrom() *mail.Address {
	return &m.From
}

// MailRequest defines a request for a Feed Story to be mailed to a list of email addresses
type MailRequest struct {
	Item       *feed.Story
	Addresses  []mail.Address
	ResultChan chan error
}

// CommandRunner is the interface for running an external command (to send mail).
type CommandRunner interface {
	Run([]byte) ([]byte, error)
}

// SendmailRunner sends email through the sendmail binary
type SendmailRunner struct {
	SendmailPath string
}

// Run sends the given byte array to sendmail over the command line.
func (r SendmailRunner) Run(input []byte) ([]byte, error) {
	cmd := exec.Command(r.SendmailPath, "-t", "-i")
	cmd.Stdin = bytes.NewReader(input)
	glog.Infof("Running command %#v", cmd.Args)
	return cmd.CombinedOutput()
}

// MailSender is the interface for something that can send mail.
type MailSender interface {
	SendMail(Message) error
}

// LocalMTASender can send mail using a local binary (rather than over SMTP)
type LocalMTASender struct {
	Runner CommandRunner
}

// SendMail sends mail using a local binary
func (self *LocalMTASender) SendMail(msg Message) error {
	msgBytes, err := msg.Bytes()
	if err != nil {
		return fmt.Errorf("error converting message to text: %s", err.Error())
	}
	_, err = self.Runner.Run(msgBytes)
	if err != nil {
		return fmt.Errorf("error running command %s", err.Error())
	}
	glog.Infof("Successfully sent mail: %s to %v", msg.MailSubject(), msg.MailTo())
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

func (self *SMTPSender) SendMail(msg Message) error {
	server_parts := strings.SplitN(self.SmtpServer, ":", 2)
	a := smtp.PlainAuth("", self.SmtpUsername, self.SmtpPassword, server_parts[0])

	msgBytes, err := msg.Bytes()
	if err != nil {
		return err
	}
	msgTo := make([]string, len(msg.MailTo()))
	for i, to := range msg.MailTo() {
		msgTo[i] = to.String()
	}
	err = smtp.SendMail(self.SmtpServer, a, msg.MailFrom().String(), msgTo, msgBytes)
	if err != nil {
		return fmt.Errorf("Error sending mail: %s\n", err.Error())
	}
	return nil
}

func NewSMTPSender(server string, username string, password string) MailSender {
	return &SMTPSender{}
}

// NullMailSender doesn't send mail.  It does record the number of times it has been called which is useful for testing.
type NullMailSender struct {
	Count int
}

func (self *NullMailSender) SendMail(m Message) error {
	self.Count++
	glog.Infof("NullMailer faked sending mail: %s to %v", m.MailSubject(), m.MailTo())
	return nil
}

//
// Mail Dispatcher
//

type MailDispatcher struct {
	OutgoingMail chan *MailRequest
	FromAddress  string
	MailSender   MailSender
}

func NewMailDispatcher(from_address string, sender MailSender) *MailDispatcher {
	return &MailDispatcher{
		OutgoingMail: make(chan *MailRequest),
		FromAddress:  from_address,
		MailSender:   sender,
	}
}

func CreateAndStartMailer(config *config.Config) *MailDispatcher {
	// Create mail sender
	var sender MailSender

	if !config.Mail.SendMail {
		glog.Info("Using null mail sender as configured.")
		sender = &NullMailSender{}
	} else if config.Mail.MtaPath != "" {
		glog.Infof("Using Local MTA: %s", config.Mail.MtaPath)
		sender = NewLocalMTASender(config.Mail.MtaPath)
	} else if config.Mail.SmtpServer != "" {
		glog.Infof("Using SMTP Server: %s", config.Mail.SmtpServer)
		sender = NewSMTPSender(config.Mail.SmtpServer, config.Mail.SmtpUsername, config.Mail.SmtpPassword)
	} else {
		panic(fmt.Sprint("No mail sending capability defined in config."))
	}

	// Start Mailer
	mailer := NewMailDispatcher(
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
		"from@example.com",
		&NullMailSender{},
	)
	go mailer.DispatchLoop()
	return mailer
}

func (self *MailDispatcher) DispatchLoop() {
	for {
		request := <-self.OutgoingMail
		request.ResultChan <- self.handleMailRequest(request)
	}
}

func (self *MailDispatcher) handleMailRequest(m *MailRequest) error {
	if self.MailSender == nil {
		return fmt.Errorf("no MailSender set, can not send mail")
	}
	if len(m.Addresses) == 0 {
		return fmt.Errorf("no recipients for mail given")
	}
	for _, addr := range m.Addresses {
		msg := CreateMailFromItem(self.FromAddress, addr, m.Item)
		err := self.MailSender.SendMail(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateMailFromItem(from string, to mail.Address, item *feed.Story) *MailMessage {
	content := FormatMessageBody(item)
	msg := MailMessage{
		gophermail.Message{
			From: mail.Address{
				Address: from,
			},
			To:       []mail.Address{to},
			Subject:  item.Title,
			Body:     content, //TODO Convert to plain text
			HTMLBody: content,
			Headers:  mail.Header{},
		},
	}
	if !item.Published.IsZero() {
		msg.Headers["Date"] = []string{item.Published.UTC().Format(time.RFC822)}
	}
	return &msg
}

func FormatMessageBody(story *feed.Story) string {
	orig_link := fmt.Sprintf(`
<div class="original_link">
<a href="%s">%s</a>
</div><hr>`, story.Link, story.Title)

	return fmt.Sprintf("%s%s", orig_link, story.Content)
}
