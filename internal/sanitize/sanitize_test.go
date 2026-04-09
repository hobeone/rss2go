package sanitize

import "testing"

func TestHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no change", "hello world", "hello world"},
		{"strips newline", "hello\nworld", "hello world"},
		{"strips carriage return", "hello\rworld", "hello world"},
		{"strips crlf", "hello\r\nworld", "hello  world"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Header(tt.input); got != tt.want {
				t.Errorf("Header(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
