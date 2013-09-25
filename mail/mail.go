package mail

import (
	rss "github.com/jteeuwen/go-pkg-rss"
	//"net/smtp"
	"log"
)

type MailDispatcher struct {
	OutgoingMail chan *rss.Item
	UseLocalMTA bool
	SmtpServer string
	SmtpUsername string
	SmtpPassword string
	ToAddress string
	FromAddress string
}

func NewMailDispatcher(to_address string, from_address string ) MailDispatcher {
	return MailDispatcher {
		OutgoingMail: make(chan *rss.Item),
		ToAddress: to_address,
		FromAddress: from_address,
	}
}

func (self *MailDispatcher) DispatchLoop() {
	for {
		m := <- self.OutgoingMail

		log.Print("Stub Mailer...")
		log.Printf("To: %s\n", self.ToAddress)
		log.Printf("From: %s\n", self.FromAddress)
		log.Print("---------------------------")
		log.Printf(m.Content.Text)
	}
}
