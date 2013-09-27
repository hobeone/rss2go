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
	if c.Mail.UseSmtp {
		t.Fatal("Expected UseSmtp to be false")
	}
	path := "../testdata/test_config.toml"
	err := c.ReadConfig(path)
	if err != nil {
		t.Fatal("Expected no errors when parsing: ", path)
	}
	if !c.Mail.UseSmtp {
		t.Fatal("Expected c.Mail.UseSmtp to be true")
	}
}
