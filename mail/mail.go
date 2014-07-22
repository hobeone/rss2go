// Wrap mail sending functionality.
//
// Can send either by calling a local sendmail binary or connecting to a SMTP
// server.
//
// Designed to run as a goroutine and centralize mail sending.
//
// TODO: add mail batching support
// Change use of gophermail.Message to an interface

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

const MTA_BINARY = "sendmail"

type MailRequest struct {
	Item       *feed.Story
	Addresses  []mail.Address
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
	glog.Infof("Running command %#v", cmd.Args)
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

type NullMailSender struct {
	Count int
}

func (self *NullMailSender) SendMail(m *gophermail.Message) error {
	self.Count++
	glog.Infof("NullMailer faked sending mail: %s to %v", m.Subject, m.To)
	return nil
}

func CreateAndStartMailer(config *config.Config) *MailDispatcher {
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
		request.ResultChan <- self.sendRequest(request)
	}
}

func (self *MailDispatcher) sendRequest(m *MailRequest) error {
	if self.MailSender == nil {
		return fmt.Errorf("no MailSender set, can not send mail")
	}
	if len(m.Addresses) == 0 {
		return fmt.Errorf("no recipients for mail given")
	}
	for _, a := range m.Addresses {
		msg := CreateMailFromItem(self.FromAddress, a, m.Item)
		err := self.MailSender.SendMail(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateMailFromItem(from string, to mail.Address, item *feed.Story) *gophermail.Message {
	content := FormatMessageBody(item)
	msg := &gophermail.Message{
		From: mail.Address{
			Address: from,
		},
		To:       []mail.Address{to},
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
