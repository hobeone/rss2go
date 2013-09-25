package mail

import (
	"testing"
	rss "github.com/jteeuwen/go-pkg-rss"
)

func TestMail(t *testing.T) {
	m := NewMailDispatcher(
		"test@test.com","recipient@gmail.com")
	go m.DispatchLoop()

	f := &rss.Item {
		Title: "Testing the Mailer...",
		Source: &rss.Source {
			Url: "http://testing.test/item_url",
			Text: "Item Url Name",
		},
		Content: &rss.Content {
			Text: "Test Content",
		},
	}

	m.OutgoingMail <- f
}
