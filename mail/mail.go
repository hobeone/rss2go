package mail

import (
	"bytes"
	"fmt"
	"github.com/jpoehls/gophermail"
	"log"
	"net/smtp"
	"os/exec"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/feed"
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

	stubOutMail  bool
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
			log.Printf("Found %s at %s.", mta_binary_name, c)
		}
		mta_binary_name = c
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

func CreateAndStartMailer(config config.Config) *MailDispatcher {
	// Start Mailer
	mailer := NewMailDispatcher(
		config.Mail.ToAddress,
		config.Mail.FromAddress,
		config.Mail.UseSendmail,
//		config.Mail.MtaPath,
		"/bin/true",
		config.Mail.SmtpServer,
		config.Mail.SmtpUsername,
		config.Mail.SmtpPassword,
	)
	if config.Mail.SendNoMail {
		mailer.SetDryRun(true)
	}
	go mailer.DispatchLoop()
	return mailer
}

func CreateAndStartStubMailer() *MailDispatcher {
	// Start Mailer
	mailer := NewMailDispatcher("to@exmaple.com", "from@example.com", false, "", "", "", "")
	mailer.SetDryRun(true)
	go mailer.DispatchLoop()
	return mailer
}

func (self *MailDispatcher) SetDryRun(v bool) {
	self.stubOutMail = v
}

func (self *MailDispatcher) sendMailWithSendmail(msg *gophermail.Message) error {
	cmd := exec.Command(self.MtaBinary, "-t")

	msgBytes, err := msg.Bytes()
	if err != nil {
		return fmt.Errorf("Error converting message to text: %s", err.Error())
	}
	cmd.Stdin = bytes.NewReader(msgBytes)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error running command \"%s %+v\": %s",
			cmd.Path, cmd.Args, output)
	}
	log.Printf("Successfully sent mail: %s", msg.Subject)
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
	msg := &gophermail.Message{
		From:     self.FromAddress,
		To:       []string{self.ToAddress},
		Subject:  m.Title,
		Body:     m.Content, // Convert to plain text
		HTMLBody: m.Content,
	}

	if self.stubOutMail {
		return nil
	}

	if self.UseLocalMTA {
		return self.sendMailWithSendmail(msg)
	} else {
		return self.sendMailWithSmtp(msg)
	}
}
