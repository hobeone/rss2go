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
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/feed"
	"gopkg.in/gomail.v1"
)

// Default MTA binary to exec when sending mail locally.
// sendmail is supported by sendmail and postfix
const MTA_BINARY = "sendmail"

// MailRequest defines a request for a Feed Story to be mailed to a list of email addresses
type MailRequest struct {
	Item       *feed.Story
	Addresses  []mail.Address
	ResultChan chan error
}

// CommandRunner is the interface for running an external command (to send mail).
type CommandRunner interface {
	//	Run([]byte) ([]byte, error)
	Run(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// SendmailRunner sends email through the sendmail binary
type SendmailRunner struct {
	SendmailPath string
}

// Run sends the given byte array to sendmail over the command line.
func (r SendmailRunner) Run(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	//func (r SendmailRunner) Run(input []byte) ([]byte, error) {
	cmd := exec.Command(r.SendmailPath, "-t", "-i")
	cmd.Stdin = bytes.NewReader(msg)
	logrus.Infof("Running command %#v", cmd.Args)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Info("Error running command %#v, output %s", cmd.Args, output)
	}
	return err
}

// MailSender is the interface for something that can send mail.
type MailSender interface {
	SendMail(*gomail.Message) error
}

// LocalMTASender can send mail using a local binary (rather than over SMTP)
type LocalMTASender struct {
	Runner CommandRunner
}

// SendMail sends mail using a local binary
func (sender *LocalMTASender) SendMail(msg *gomail.Message) error {
	mailer := gomail.NewMailer("host", "user", "pwd", 465, gomail.SetSendMail(sender.Runner.Run))
	err := mailer.Send(msg)
	if err != nil {
		return fmt.Errorf("error running command %s", err.Error())
	}
	logrus.Infof("Successfully sent mail: %v to %v", msg.GetHeader("From"), msg.GetHeader("To"))
	return nil
}

// NewLocalMTASender returns a pointer to a new LocalMTASender instance with
// defaults set.
func NewLocalMTASender(mtaPath string) *LocalMTASender {
	if mtaPath == "" {
		mtaPath = MTA_BINARY
	}

	c, err := exec.LookPath(mtaPath)
	if err != nil {
		panic(fmt.Sprintf("Couldn't find specified MTA: %s", err.Error()))
	} else {
		logrus.Infof("Found %s at %s.", mtaPath, c)
	}
	mtaPath = c

	return &LocalMTASender{
		Runner: SendmailRunner{mtaPath},
	}
}

// SMTPSender encapsualtes functionality to send mail to a SMTP server
type SMTPSender struct {
	Hostname string
	Port     int
	Username string
	Password string
}

// SendMail sends the given message to the configured server.
//
// It does no batching or delay and just sends immediately.
func (s *SMTPSender) SendMail(msg *gomail.Message) error {
	mailer := gomail.NewMailer(s.Hostname, s.Username, s.Password, s.Port)
	return mailer.Send(msg)
}

// NewSMTPSender returns a pointer to a new SMTPSender instance with
// defaults set.
func NewSMTPSender(server string, port int, username string, password string) MailSender {
	return &SMTPSender{
		Hostname: server,
		Port:     port,
		Username: username,
		Password: password,
	}
}

// NullMailSender doesn't send mail.  It does record the number of times it has been called which is useful for testing.
type NullMailSender struct {
	Count int
}

// SendMail increments a counter for checking in tests.
func (sender *NullMailSender) SendMail(m *gomail.Message) error {
	sender.Count++
	logrus.Infof("NullMailer faked sending mail: %s to %v", m.GetHeader("From"), m.GetHeader("To"))
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
		logrus.Info("Using null mail sender as configured.")
		sender = &NullMailSender{}
	} else if config.Mail.MtaPath != "" {
		logrus.Infof("Using Local MTA: %s", config.Mail.MtaPath)
		sender = NewLocalMTASender(config.Mail.MtaPath)
	} else if config.Mail.Hostname != "" {
		logrus.Infof("Using SMTP Server: %s:%d", config.Mail.Hostname, config.Mail.Port)
		sender = NewSMTPSender(config.Mail.Hostname, config.Mail.Port, config.Mail.Username, config.Mail.Password)
	} else {
		panic(fmt.Sprint("No mail sending capability defined in config."))
	}

	// Start Mailer
	mailer := NewMailDispatcher(
		config.Mail.FromAddress,
		sender,
	)
	logrus.Infof("Created new mailer: %#v", mailer)
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

func CreateMailFromItem(from string, to mail.Address, item *feed.Story) *gomail.Message {
	content := FormatMessageBody(item)
	gmsg := gomail.NewMessage()
	gmsg.SetHeader("From", from)
	gmsg.SetHeader("To", to.String())
	gmsg.SetHeader("Subject", item.Title)
	gmsg.SetBody("text/html", content)
	if !item.Published.IsZero() {
		gmsg.SetHeader("Date", item.Published.UTC().Format(time.RFC822))
	}
	return gmsg
}

func FormatMessageBody(story *feed.Story) string {
	origLink := fmt.Sprintf(`
<div class="original_link">
<a href="%s">%s</a>
</div><hr>`, story.Link, story.Title)

	return fmt.Sprintf("%s%s", origLink, story.Content)
}
