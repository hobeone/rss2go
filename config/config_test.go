package config

import (
	"testing"
)

func TestReadConfigFailsOnNonExistingPath(t *testing.T) {
	c := NewConfig()
	path := "/does/not/exist"
	err := c.ReadConfig(path)
	if err == nil {
		t.Error("Expected error on non existing path: ", path)
	}
}

func TestReadConfigFailsOnBadFormat(t *testing.T) {
	c := NewConfig()
	path := "/etc/passwd"
	err := c.ReadConfig(path)
	if err == nil {
		t.Error("Expected error on bad format config: ", path)
	}
}

func TestDefaultsGetOverridden(t *testing.T) {
	c := NewConfig()
	path := "../testdata/test_config.toml"
	err := c.ReadConfig(path)
	if err != nil {
		t.Fatal("Expected no errors when parsing: ", path)
	}
	if c.Mail.UseSmtp {
		t.Fatal("Expected c.Mail.Smtp to be false")
	}
}

func TestSendNoMailSetter(t *testing.T) {
	c := NewConfig()
	if c.Mail.SendNoMail == true {
		t.Error("SendNoMail should be false.")
	}
	c.Mail.SendNoMail = true
	if c.Mail.SendNoMail == false {
		t.Error("SendNoMail should be true.")
	}
}
