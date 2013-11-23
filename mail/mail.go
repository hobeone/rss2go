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
	"github.com/golang/glog"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/feed"
	"github.com/jpoehls/gophermail"
	"net/mail"
	"net/smtp"
	"os/exec"
	"time"
)

const MTA_BINARY = "sendmail"

type MailRequest struct {
	Item       *feed.Story
	ResultChan chan error
}

type MailDispatcher struct {
	OutgoingMail chan *MailRequest
	UseLocalMTA  bool
	MtaBinary    string
	SmtpServer   string
	SmtpUsername string
	SmtpPassword string
	ToAddress    string
	FromAddress  string

	stubOutMail bool
}

func NewMailDispatcher(
	to_address string,
	from_address string,
	use_local_mta bool,
	mta_binary_name string,
	smtp_server string,
	smtp_username string,
	smtp_password string) *MailDispatcher {

	if mta_binary_name == "" {
		mta_binary_name = MTA_BINARY
	}

	if use_local_mta {
		c, err := exec.LookPath(mta_binary_name)
		if err != nil {
			panic(fmt.Sprintf("Couldn't find specified MTA: %s", err.Error()))
		} else {
			glog.Infof("Found %s at %s.", mta_binary_name, c)
		}
		mta_binary_name = c
	}

	if to_address == "" {
		panic("to_address can't be blank.")
	}

	return &MailDispatcher{
		OutgoingMail: make(chan *MailRequest),
		ToAddress:    to_address,
		FromAddress:  from_address,
		UseLocalMTA:  use_local_mta,
		MtaBinary:    mta_binary_name,
		SmtpServer:   smtp_server,
		SmtpUsername: smtp_username,
		SmtpPassword: smtp_password,
		stubOutMail:  false,
	}
}

func CreateAndStartMailer(config *config.Config) *MailDispatcher {
	// Start Mailer
	mailer := NewMailDispatcher(
		config.Mail.ToAddress,
		config.Mail.FromAddress,
		config.Mail.UseSendmail,
		config.Mail.MtaPath,
		config.Mail.SmtpServer,
		config.Mail.SmtpUsername,
		config.Mail.SmtpPassword,
	)
	if !config.Mail.SendMail {
		glog.Info("Setting dry run as configured.")
		mailer.SetDryRun(true)
	}
	glog.Infof("Created new mailer: %#v", mailer)
	go mailer.DispatchLoop()
	return mailer
}

func CreateAndStartStubMailer() *MailDispatcher {
	// Start Mailer
	mailer := NewMailDispatcher(
		"to@exmaple.com", "from@example.com", false, "", "", "", "")
	mailer.SetDryRun(true)
	go mailer.DispatchLoop()
	return mailer
}

func (self *MailDispatcher) SetDryRun(v bool) {
	self.stubOutMail = v
}

func (self *MailDispatcher) sendMailWithSendmail(
	msg *gophermail.Message) error {
	cmd := exec.Command(self.MtaBinary, "-t")

	msgBytes, err := msg.Bytes()
	if err != nil {
		return fmt.Errorf(
			"Error converting message to text: %s", err.Error())
	}
	cmd.Stdin = bytes.NewReader(msgBytes)

	_, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error running command %#v: %s",
			cmd.Args, err)
	}
	glog.Infof("Successfully sent mail: %s", msg.Subject)
	return nil
}

func (self *MailDispatcher) sendMailWithSmtp(msg *gophermail.Message) error {
	a := smtp.PlainAuth("", self.SmtpUsername, self.SmtpPassword, self.SmtpServer)
	err := gophermail.SendMail(self.SmtpServer, a, msg)

	if err != nil {
		return fmt.Errorf("Error sending mail: %s\n", err.Error())
	}
	return nil
}

func (self *MailDispatcher) DispatchLoop() {
	for {
		request := <-self.OutgoingMail
		request.ResultChan <- self.handleMail(request.Item)
	}
}

func (self *MailDispatcher) handleMail(m *feed.Story) error {
	if self.stubOutMail {
		return nil
	}

	msg := CreateMailFromItem(self.FromAddress, self.ToAddress, m)

	if self.UseLocalMTA {
		return self.sendMailWithSendmail(msg)
	} else {
		return self.sendMailWithSmtp(msg)
	}
}

func CreateMailFromItem(from string, to string, item *feed.Story) *gophermail.Message {
	content := FormatMessageBody(item)
	msg := &gophermail.Message{
		From:     from,
		To:       []string{to},
		Subject:  fmt.Sprintf("%s: %s", item.ParentFeed.Title, item.Title),
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
