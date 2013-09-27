package mail

import (
	"testing"
	"github.com/jpoehls/gophermail"
)

func TestSendMailWithSendmail(t *testing.T) {
	d := NewMailDispatcher(
		"test@test.com",
		"recipient@test.com",
		true,
		"/bin/true",
		"",
		"",
		"",
	)

	msg := &gophermail.Message{
		From:     "from@example.com",
		To:       []string{"to@example.com"},
		Subject:  "Testing subject",
		Body:     "Test Body",
	}

	result := d.sendMailWithSendmail(msg)

	if result != nil {
		t.Error("Sending to /bin/true should always work.")
	}


	d = NewMailDispatcher(
		"test@test.com",
		"recipient@test.com",
		true,
		"/bin/false",
		"",
		"",
		"",
	)
	result = d.sendMailWithSendmail(msg)

	if result == nil {
		t.Error("Sending to /bin/false should always fail.")
	}
}

func TestObjectCreation(t *testing.T) {
	d := NewMailDispatcher(
		"test@test.com",
		"recipient@test.com",
		false,
		"",
		"localhost:25",
		"testuser",
		"testpass",
	)

	if d.MtaBinary != MTA_BINARY {
		t.Error("NewMailDispatcher should set the MTA to the default when given and empty string.")
	}

	defer func() {
		if err := recover(); err == nil {
			t.Error("NewMailDispatcher should fail on non existing MTA path.")
		}
	}()

	d = NewMailDispatcher(
		"test@test.com",
		"recipient@test.com",
		true,
		"/notexist",
		"",
		"",
		"",
	)
}
