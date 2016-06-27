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
	"io"
	"net/mail"
	"os/exec"
	"time"

	"gopkg.in/gomail.v2"

	"github.com/Sirupsen/logrus"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/feed"
)

// MTABINARY sets the default MTA binary to exec when sending mail locally.
// sendmail is supported by sendmail and postfix
const MTABINARY = "sendmail"

// Request defines a request for a Feed Story to be mailed to a list of email addresses
type Request struct {
	Item       *feed.Story
	Addresses  []mail.Address
	ResultChan chan error
}

// CommandRunner is the interface for running an external command (to send mail).
type CommandRunner interface {
	//	Run([]byte) ([]byte, error)
	Run(from string, to []string, msg []byte) error
}

// SendmailRunner sends email through the sendmail binary
type SendmailRunner struct {
	SendmailPath string
}

// Run sends the given byte array to sendmail over the command line.
func (r SendmailRunner) Run(from string, to []string, msg []byte) error {
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

// Sender is the interface for something that can send mail. The argument is a
// two dimensional array of messages grouped by Feed Item.  This is to allow
// reporting back of which items were successfully sent.
type Sender interface {
	SendMail(*gomail.Message) error
}

// LocalMTASender can send mail using a local binary (rather than over SMTP)
type LocalMTASender struct {
	Runner CommandRunner
}

// SendMail sends mail using a local binary
func (sender *LocalMTASender) SendMail(msg *gomail.Message) error {
	s := gomail.SendFunc(func(from string, to []string, msg io.WriterTo) error {
		b := bytes.NewBuffer([]byte{})
		msg.WriteTo(b)
		return sender.Runner.Run(from, to, b.Bytes())
	})

	err := gomail.Send(s, msg)
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
		mtaPath = MTABINARY
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
	reqChan  chan smtpRequest
	dialer   dialer
}

type smtpRequest struct {
	Message  *gomail.Message
	Response chan error
}

// SendMail sends the given message to the configured server.
//
// It does no batching or delay and just sends immediately.
func (s *SMTPSender) SendMail(msg *gomail.Message) error {
	r := make(chan error)
	s.reqChan <- smtpRequest{
		Message:  msg,
		Response: r,
	}
	return <-r
}

// NewSMTPSender returns a pointer to a new SMTPSender instance with
// defaults set.
func NewSMTPSender(server string, port int, username string, password string) Sender {
	s := &SMTPSender{
		Hostname: server,
		Port:     port,
		Username: username,
		Password: password,
		reqChan:  make(chan smtpRequest),
		dialer:   gomail.NewDialer(server, port, username, password),
	}
	go s.smtpDaemon()
	return s
}

type dialer interface {
	Dial() (gomail.SendCloser, error)
}

func (s *SMTPSender) smtpDaemon() {
	open := false
	var sender gomail.SendCloser
	var err error

	for {
		select {
		case m, ok := <-s.reqChan:
			if !ok {
				// Closed channel, stop and return
				return
			}
			if !open {
				if sender, err = s.dialer.Dial(); err != nil {
					logrus.Errorf("Error connecting to mail server: %v", err)
					m.Response <- err
					continue
				}
				open = true
			}
			if err := gomail.Send(sender, m.Message); err != nil {
				m.Response <- err
				logrus.Errorf("Error sending mail: %v", err)
				continue
			}
			m.Response <- nil
		// Close the connection to the SMTP server if no email was sent in
		// the last 30 seconds.
		case <-time.After(30 * time.Second):
			if open {
				logrus.Infof("Closing SMTP connection after 30 seconds of inactivity.")
				if err := sender.Close(); err != nil {
					logrus.Errorf("Error closing connection: %v", err)
				}
				open = false
			}
		}
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

// Dispatcher listens for mail Requests and sends them to the configured Sender
type Dispatcher struct {
	OutgoingMail chan *Request
	FromAddress  string
	MailSender   Sender
}

// NewDispatcher returns a newly created Dispatcher with defaults set.
func NewDispatcher(fromAddress string, sender Sender) *Dispatcher {
	return &Dispatcher{
		OutgoingMail: make(chan *Request),
		FromAddress:  fromAddress,
		MailSender:   sender,
	}
}

// CreateAndStartMailer returns a New Dispatcher with a sender cofigured from
// the config file.
func CreateAndStartMailer(config *config.Config) *Dispatcher {
	// Create mail sender
	var sender Sender

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
	mailer := NewDispatcher(
		config.Mail.FromAddress,
		sender,
	)
	logrus.Infof("Created new mailer: %#v", mailer)
	go mailer.DispatchLoop()
	return mailer
}

// CreateAndStartStubMailer returns a Dispatcher that will send all mail to
// null.  For testing.
func CreateAndStartStubMailer() *Dispatcher {
	// Start Mailer
	mailer := NewDispatcher(
		"from@example.com",
		&NullMailSender{},
	)
	go mailer.DispatchLoop()
	return mailer
}

// DispatchLoop start an infinite loop listening for MailRequests and handling
// them.
func (d *Dispatcher) DispatchLoop() {
	for {
		request := <-d.OutgoingMail
		request.ResultChan <- d.handleMailRequest(request)
	}
}

func (d *Dispatcher) handleMailRequest(m *Request) error {
	if d.MailSender == nil {
		return fmt.Errorf("no MailSender set, can not send mail")
	}
	if len(m.Addresses) == 0 {
		return fmt.Errorf("no recipients for mail given")
	}
	for _, addr := range m.Addresses {
		msg := CreateMailFromItem(d.FromAddress, addr, m.Item)
		err := d.MailSender.SendMail(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateMailFromItem returns a Message containing the given story.
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

// FormatMessageBody formats the Story for better reading in a mail client.
func FormatMessageBody(story *feed.Story) string {
	origLink := fmt.Sprintf(`
<div class="original_link">
<a href="%s">%s</a>
</div><hr>`, story.Link, story.Title)

	return fmt.Sprintf("%s%s", origLink, story.Content)
}
