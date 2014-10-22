package feed

import (
	"testing"
)

func TestParseDate(t *testing.T) {
	_, err := parseDate("Goops")
	if err == nil {
		t.Fatal("Expected error when parsing bogus date")
	}

	_, err = parseDate("12-31-1999")
	if err != nil {
		t.Fatalf("Expected to be able to parse simple date, got %s", err)
	}
}
