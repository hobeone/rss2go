package config

import (
	"os"
	"testing"
)

func TestReadConfigFailsOnNonExistingPath(t *testing.T) {
	c := NewConfig()
	path := "/does/not/exist"
	err := c.ReadConfig(path)
	if _, ok := err.(*os.PathError); !ok {
		t.Error("Expected PathError on non existing path: ", path)
	}
}

func TestReadConfigFailsOnBadFormat(t *testing.T) {
	c := NewConfig()
	path := "../testdata/configs/bad_format.toml"
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
	path := "../testdata/configs/test_config.toml"
	err := c.ReadConfig(path)
	if err != nil {
		t.Fatal("Expected no errors when parsing: ", path)
	}
	if !c.Mail.UseSmtp {
		t.Fatal("Expected c.Mail.UseSmtp to be true")
	}
}
