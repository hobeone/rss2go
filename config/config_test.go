package config

import (
	"fmt" // Added for fmt.Errorf
	"os"  // Added for os.CreateTemp
	"os/user"
	"strings" // Added for strings.NewReader and strings.ReplaceAll

	"testing"
)

func TestReadConfigFailsOnNonExistingPath(t *testing.T) {
	c := NewConfig()
	path := "/does/not/exist"
	err := c.ReadConfig(path)
	if err == nil {
		t.Errorf("Expected PathError on non existing path: %s", path)
	}
}

func TestReadConfigFailsOnBadFormat(t *testing.T) {
	c := NewConfig()
	path := "../testdata/configs/bad_config.json"
	err := c.ReadConfig(path)

	if err == nil {
		t.Error("Expected error on bad format config: ", path)
	}
}

func TestDefaultsGetOverridden(t *testing.T) {
	c := NewConfig()
	if c.Mail.UseSMTP {
		t.Fatal("Expected UseSMTP to be false")
	}
	path := "../testdata/configs/test_config.json"
	err := c.ReadConfig(path)
	if err != nil {
		t.Fatalf("Expected no errors when parsing: %s, got %s", path, err)
	}
	if !c.Mail.UseSMTP {
		t.Fatal("Expected c.Mail.UseSMTP to be true")
	}
}

func TestReplaceTildeInPath(t *testing.T) {
	originalCurrentUserFunc := currentUserFunc
	t.Cleanup(func() {
		currentUserFunc = originalCurrentUserFunc
	})

	mockUser := &user.User{
		HomeDir: "/mocked/home",
	}
	currentUserFunc = func() (*user.User, error) {
		return mockUser, nil
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Path with tilde", "~/testfile", "/mocked/home/testfile"},
		{"Path without tilde", "/etc/config.json", "/etc/config.json"},
		{"Empty path", "", ""},
		{"Path as just tilde", "~", "/mocked/home"},
		{"Path with multiple tildes", "~/test/~/again", "/mocked/home/test/~/again"},
		{"Path with tilde not at start", "/some/path/~/other", "/some/path/~/other"},
		{"Path with tilde in middle of component", "test~file", "test~file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := replaceTildeInPath(tt.input)
			if actual != tt.expected {
				t.Errorf("For input '%s', expected '%s', but got '%s'", tt.input, tt.expected, actual)
			}
		})
	}

	// Test error case for currentUserFunc
	currentUserFunc = func() (*user.User, error) {
		return nil, fmt.Errorf("mock user error")
	}
	t.Run("UserError", func(t *testing.T) {
		input := "~/testfile_error"
		expected := "~/testfile_error" // Should return path unmodified
		actual := replaceTildeInPath(input)
		if actual != expected {
			t.Errorf("For input '%s' with user error, expected '%s', but got '%s'", input, expected, actual)
		}
	})
}

func TestHighlightBytePosition(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		pos               int64
		expectedLine      int
		expectedCol       int
		expectedHighlight string
	}{
		{
			name:         "Error at line 1, col 1",
			input:        `{"key": "error here"}`,
			pos:          8, // 'e' in "error here"
			expectedLine: 1,
			expectedCol:  9,
			expectedHighlight: "    1: {\"key\": \"error here\"}\n" +
				strings.Repeat(" ", (9-1)+7) + "^\n",
		},
		{
			name:         "Error in middle of line 1",
			input:        `{"key": "some error here"}`,
			pos:          13, // 'e' in "error"
			expectedLine: 1,
			expectedCol:  14,
			expectedHighlight: "    1: {\"key\": \"some error here\"}\n" +
				strings.Repeat(" ", (14-1)+7) + "^\n",
		},
		{
			name:         "Error at end of line 1",
			input:        `{"key": "some error"}`,
			pos:          19, // Closing }
			expectedLine: 1,
			expectedCol:  20,
			expectedHighlight: "    1: {\"key\": \"some error\"}\n" +
				strings.Repeat(" ", (20-1)+7) + "^\n",
		},
		{
			name:         "Error on line 2",
			input:        "{\n\"key\": \"error on line2\"}", // pos points to 'e'
			pos:          10,                                // 'e' in "error on line2"
			expectedLine: 2,
			expectedCol:  9, // "key": "e -> 0-indexed 8 on line, so 1-based 9
			expectedHighlight: "    1: {\n" +
				"    2: \"key\": \"error on line2\"}\n" +
				strings.Repeat(" ", (9-1)+7) + "^\n",
		},
		{
			name: "Error with multi-line context",
			// Target 'e' in "error here" on line 3
			// Line 1: "{\n" (2 bytes)
			// Line 2: "  \"name\": \"test\",\n" (19 bytes)
			// Line 3: "  \"value\": \"" (11 bytes) + 'e' is target.
			// Pos = 2 + 19 + 11 = 32
			input:        "{\n  \"name\": \"test\",\n  \"value\": \"error here\"\n}",
			pos:          32, // 'e' in "error here"
			expectedLine: 3,
			expectedCol:  13, // "  \"value\": \"e is 0-indexed 12 on its line
			expectedHighlight: "    2:   \"name\": \"test\",\n" +
				"    3:   \"value\": \"error here\"\n" + // fullErrorLine should capture this
				strings.Repeat(" ", (13-1)+7) + "^\n",
			// Note: The function reads the rest of the line, so the final "}" from input won't be in this highlight part for line 3.
		},
		{
			name:         "Error position out of bounds (negative)",
			input:        `{"key": "value"}`,
			pos:          -1, // Effectively pos=0
			expectedLine: 1,
			expectedCol:  1,
			expectedHighlight: "    1: {\"key\": \"value\"}\n" +
				strings.Repeat(" ", (1-1)+7) + "^\n",
		},
		{
			name:         "Error position out of bounds (too large)",
			input:        `{"key": "value"}`, // length 16
			pos:          100,                // Effectively pos=16 (after last char)
			expectedLine: 1,
			expectedCol:  17, // Column after the last character
			expectedHighlight: "    1: {\"key\": \"value\"}\n" +
				strings.Repeat(" ", (17-1)+7) + "^\n",
		},
		{
			name:         "Error at first char of input",
			input:        `{`,
			pos:          0,
			expectedLine: 1,
			expectedCol:  1,
			expectedHighlight: "    1: {\n" +
				strings.Repeat(" ", (1-1)+7) + "^\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			line, col, highlight := highlightBytePosition(r, tt.pos)

			if line != tt.expectedLine {
				t.Errorf("Expected line %d, got %d", tt.expectedLine, line)
			}
			if col != tt.expectedCol {
				t.Errorf("Expected col %d, got %d", tt.expectedCol, col)
			}
			// Replace CRNL with NL for consistent comparison on Windows vs Unix
			normalizedHighlight := strings.ReplaceAll(highlight, "\r\n", "\n")
			normalizedExpectedHighlight := strings.ReplaceAll(tt.expectedHighlight, "\r\n", "\n")

			if normalizedHighlight != normalizedExpectedHighlight {
				t.Errorf("Expected highlight:\n%s\nGot:\n%s", normalizedExpectedHighlight, normalizedHighlight)
			}
		})
	}
}

func TestReadConfigInvalidValues(t *testing.T) {
	tests := []struct {
		name        string
		jsonContent string
		expectError bool // True if ReadConfig itself should fail (e.g., JSON parsing/type error)
	}{
		{
			name:        "Mail.Port as string",
			jsonContent: `{"Mail": {"Port": "not-a-number"}}`,
			expectError: true,
		},
		{
			name:        "Mail.Port as negative",
			jsonContent: `{"Mail": {"Port": -1}}`,
			expectError: false, // ReadConfig unmarshals, doesn't validate semantics unless type mismatch
		},
		{
			name:        "Crawl.MaxCrawlers as zero",
			jsonContent: `{"Crawl": {"MaxCrawlers": 0}}`,
			expectError: false,
		},
		{
			name:        "Crawl.MaxCrawlers as negative",
			jsonContent: `{"Crawl": {"MaxCrawlers": -5}}`,
			expectError: false,
		},
		{
			name:        "Crawl.MinInterval negative",
			jsonContent: `{"Crawl": {"MinInterval": -100}}`,
			expectError: false,
		},
		{
			name:        "Crawl.MaxInterval less than MinInterval",
			jsonContent: `{"Crawl": {"MinInterval": 500, "MaxInterval": 100}}`,
			expectError: false,
		},
		{
			name:        "DB.Path as empty string",
			jsonContent: `{"DB": {"Path": ""}}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp(t.TempDir(), "test_config_*.json")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			filePath := tmpFile.Name()
			// No need for t.Cleanup(os.Remove(filePath)) as t.TempDir() handles cleanup

			if _, err := tmpFile.WriteString(tt.jsonContent); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			tmpFile.Close()

			c := NewConfig()
			err = c.ReadConfig(filePath)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for scenario '%s', but got nil", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for scenario '%s', but got: %v", tt.name, err)
				}
			}
		})
	}
}
