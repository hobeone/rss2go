package sanitize

import "strings"

// Header strips CR and LF characters from s, replacing them with spaces.
// This prevents header injection attacks in email headers.
func Header(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
}
