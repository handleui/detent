package errors

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func TestExtractSnippet_NormalCase(t *testing.T) {
	content := `package main

import "fmt"

func main() {
	fmt.Println("Hello")
	fmt.Println("World")
	fmt.Println("!")
}
`
	tmpFile := createTempFile(t, "test.go", content)

	snippet := ExtractSnippet(tmpFile, 6)
	if snippet == nil {
		t.Fatal("ExtractSnippet returned nil for valid file and line")
	}

	if snippet.StartLine != 3 {
		t.Errorf("StartLine = %d, want 3", snippet.StartLine)
	}

	if snippet.ErrorLine != 4 {
		t.Errorf("ErrorLine = %d, want 4", snippet.ErrorLine)
	}

	if snippet.Language != "go" {
		t.Errorf("Language = %q, want %q", snippet.Language, "go")
	}

	if len(snippet.Lines) == 0 {
		t.Error("Expected non-empty Lines slice")
	}
}

func TestExtractSnippet_FileNotFound(t *testing.T) {
	snippet := ExtractSnippet("/nonexistent/path/to/file.go", 10)
	if snippet != nil {
		t.Error("Expected nil snippet for non-existent file")
	}
}

func TestExtractSnippet_LineZero(t *testing.T) {
	tmpFile := createTempFile(t, "test.go", "package main\n")

	snippet := ExtractSnippet(tmpFile, 0)
	if snippet != nil {
		t.Error("Expected nil snippet for line 0")
	}
}

func TestExtractSnippet_NegativeLine(t *testing.T) {
	tmpFile := createTempFile(t, "test.go", "package main\n")

	snippet := ExtractSnippet(tmpFile, -1)
	if snippet != nil {
		t.Error("Expected nil snippet for negative line")
	}

	snippet = ExtractSnippet(tmpFile, -100)
	if snippet != nil {
		t.Error("Expected nil snippet for large negative line")
	}
}

func TestExtractSnippet_LineBeyondFileLength(t *testing.T) {
	content := `line1
line2
line3
`
	tmpFile := createTempFile(t, "test.go", content)

	snippet := ExtractSnippet(tmpFile, 100)

	if snippet != nil {
		if len(snippet.Lines) > 3 {
			t.Errorf("Expected at most 3 lines, got %d", len(snippet.Lines))
		}
	}
}

func TestExtractSnippet_LinePastEndButNearby(t *testing.T) {
	content := `line1
line2
line3
line4
line5
`
	tmpFile := createTempFile(t, "test.go", content)

	snippet := ExtractSnippet(tmpFile, 7)

	if snippet != nil {
		if snippet.StartLine < 1 {
			t.Errorf("StartLine should be >= 1, got %d", snippet.StartLine)
		}
	}
}

func TestExtractSnippet_BinaryFileDetection(t *testing.T) {
	content := "package main\x00\x00\x00binary content"
	tmpFile := createTempFile(t, "test.go", content)

	snippet := ExtractSnippet(tmpFile, 1)
	if snippet != nil {
		t.Error("Expected nil snippet for binary file with null bytes")
	}
}

func TestExtractSnippet_BinaryFileHighNonPrintable(t *testing.T) {
	content := "\x01\x02\x03\x04\x05\x06\x07\x08abcd"
	tmpFile := createTempFile(t, "test.go", content)

	snippet := ExtractSnippet(tmpFile, 1)
	if snippet != nil {
		t.Error("Expected nil snippet for file with high non-printable ratio")
	}
}

func TestExtractSnippet_EmptyFile(t *testing.T) {
	tmpFile := createTempFile(t, "test.go", "")

	snippet := ExtractSnippet(tmpFile, 1)
	if snippet != nil {
		t.Error("Expected nil snippet for empty file")
	}
}

func TestExtractSnippet_VeryLongLines(t *testing.T) {
	longLine := make([]byte, MaxLineLength+100)
	for i := range longLine {
		longLine[i] = 'a'
	}
	content := "short line\n" + string(longLine) + "\nshort again\n"
	tmpFile := createTempFile(t, "test.go", content)

	snippet := ExtractSnippet(tmpFile, 2)
	if snippet == nil {
		t.Fatal("Expected snippet even with long line (should truncate)")
	}

	for _, line := range snippet.Lines {
		if len(line) > MaxLineLength+10 {
			t.Errorf("Line should be truncated but has length %d", len(line))
		}
	}
}

func TestExtractSnippet_UTF8Characters(t *testing.T) {
	content := `package main

// Japanese: 日本語
// Chinese: 中文
// Emoji: 🎉🎊🎈
// Russian: Привет
func main() {
	fmt.Println("こんにちは")
}
`
	tmpFile := createTempFile(t, "test.go", content)

	snippet := ExtractSnippet(tmpFile, 5)
	if snippet == nil {
		t.Fatal("Expected snippet for file with UTF-8 characters")
	}

	found := false
	for _, line := range snippet.Lines {
		if containsRune(line, '日') || containsRune(line, '中') || containsRune(line, '🎉') {
			found = true
			break
		}
	}
	if !found {
		t.Error("UTF-8 characters should be preserved in snippet")
	}
}

func TestExtractSnippet_UTF8Truncation(t *testing.T) {
	emoji := "🎉"
	content := ""
	for i := 0; i < MaxLineLength/4+50; i++ {
		content += emoji
	}
	content += "\n"
	tmpFile := createTempFile(t, "test.go", content)

	snippet := ExtractSnippet(tmpFile, 1)
	if snippet == nil {
		t.Fatal("Expected snippet even with long UTF-8 line")
	}

	for _, line := range snippet.Lines {
		if !utf8.ValidString(line) {
			t.Error("Invalid UTF-8 sequence after truncation")
		}
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		filePath string
		expected string
	}{
		{"main.go", "go"},
		{"path/to/file.go", "go"},

		{"app.ts", "typescript"},
		{"component.tsx", "typescript"},
		{"module.mts", "typescript"},
		{"module.cts", "typescript"},

		{"index.js", "javascript"},
		{"component.jsx", "javascript"},
		{"module.mjs", "javascript"},
		{"module.cjs", "javascript"},

		{"script.py", "python"},
		{"types.pyi", "python"},
		{"app.pyw", "python"},

		{"main.rs", "rust"},

		{"app.rb", "ruby"},

		{"Main.java", "java"},

		{"App.kt", "kotlin"},
		{"build.kts", "kotlin"},

		{"app.swift", "swift"},

		{"main.c", "c"},
		{"header.h", "c"},
		{"main.cpp", "cpp"},
		{"header.hpp", "cpp"},
		{"main.cc", "cpp"},
		{"main.cxx", "cpp"},

		{"Program.cs", "csharp"},

		{"index.php", "php"},

		{"App.vue", "vue"},
		{"Component.svelte", "svelte"},
		{"Page.astro", "astro"},

		{"config.json", "json"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"config.toml", "toml"},

		{"README.md", "markdown"},
		{"query.sql", "sql"},
		{"script.sh", "shell"},
		{"script.bash", "shell"},
		{"script.zsh", "shell"},

		{"file.xyz", "text"},
		{"file.unknown", "text"},
		{"noextension", "text"},

		{"FILE.GO", "go"},
		{"APP.TS", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			result := detectLanguage(tt.filePath)
			if result != tt.expected {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.filePath, result, tt.expected)
			}
		})
	}
}

func TestIsBinaryLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "normal text",
			line:     "package main",
			expected: false,
		},
		{
			name:     "text with tabs",
			line:     "\t\tindented code",
			expected: false,
		},
		{
			name:     "empty line",
			line:     "",
			expected: false,
		},
		{
			name:     "null byte",
			line:     "text\x00with null",
			expected: true,
		},
		{
			name:     "high non-printable ratio",
			line:     "\x01\x02\x03\x04\x05abc",
			expected: true,
		},
		{
			name:     "UTF-8 characters",
			line:     "日本語テスト",
			expected: false,
		},
		{
			name:     "emoji",
			line:     "Test 🎉 emoji",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBinaryLine(tt.line)
			if result != tt.expected {
				t.Errorf("isBinaryLine(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		expected string
	}{
		{
			name:     "no truncation needed",
			input:    "hello",
			maxBytes: 10,
			expected: "hello",
		},
		{
			name:     "truncate ASCII",
			input:    "hello world",
			maxBytes: 5,
			expected: "hello",
		},
		{
			name:     "truncate at UTF-8 boundary",
			input:    "日本語",
			maxBytes: 6,
			expected: "日本",
		},
		{
			name:     "truncate mid UTF-8 sequence",
			input:    "日本語",
			maxBytes: 4,
			expected: "日",
		},
		{
			name:     "empty string",
			input:    "",
			maxBytes: 10,
			expected: "",
		},
		{
			name:     "emoji truncation",
			input:    "🎉🎊🎈",
			maxBytes: 8,
			expected: "🎉🎊",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateUTF8(tt.input, tt.maxBytes)
			if result != tt.expected {
				t.Errorf("truncateUTF8(%q, %d) = %q, want %q",
					tt.input, tt.maxBytes, result, tt.expected)
			}
		})
	}
}

func TestExtractSnippetWithContext(t *testing.T) {
	content := `line1
line2
line3
line4
line5
line6
line7
line8
line9
line10
`
	tmpFile := createTempFile(t, "test.txt", content)

	tests := []struct {
		name         string
		line         int
		contextLines int
		wantStart    int
		wantLines    int
	}{
		{
			name:         "middle of file with context 2",
			line:         5,
			contextLines: 2,
			wantStart:    3,
			wantLines:    5,
		},
		{
			name:         "beginning of file",
			line:         1,
			contextLines: 2,
			wantStart:    1,
			wantLines:    3,
		},
		{
			name:         "end of file",
			line:         10,
			contextLines: 2,
			wantStart:    8,
			wantLines:    3,
		},
		{
			name:         "zero context",
			line:         5,
			contextLines: 0,
			wantStart:    5,
			wantLines:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snippet := ExtractSnippetWithContext(tmpFile, tt.line, tt.contextLines)
			if snippet == nil {
				t.Fatal("Expected non-nil snippet")
			}

			if snippet.StartLine != tt.wantStart {
				t.Errorf("StartLine = %d, want %d", snippet.StartLine, tt.wantStart)
			}

			if len(snippet.Lines) != tt.wantLines {
				t.Errorf("len(Lines) = %d, want %d", len(snippet.Lines), tt.wantLines)
			}
		})
	}
}

func TestExtractSnippetWithContext_InvalidInputs(t *testing.T) {
	tmpFile := createTempFile(t, "test.go", "line1\nline2\n")

	tests := []struct {
		name         string
		filePath     string
		line         int
		contextLines int
	}{
		{"empty path", "", 1, 3},
		{"negative context", tmpFile, 1, -1},
		{"directory not file", os.TempDir(), 1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snippet := ExtractSnippetWithContext(tt.filePath, tt.line, tt.contextLines)
			if snippet != nil {
				t.Error("Expected nil snippet for invalid input")
			}
		})
	}
}

func TestExtractSnippetsForErrors(t *testing.T) {
	content := `package main

func main() {
	x := 1
	y := 2
	fmt.Println(x + y)
}
`
	tmpFile := createTempFile(t, "main.go", content)

	basePath := filepath.Dir(tmpFile)
	fileName := filepath.Base(tmpFile)

	errors := []*ExtractedError{
		{
			Message:   "undefined: fmt",
			File:      fileName,
			Line:      6,
			LineKnown: true,
		},
		{
			Message:   "error without line",
			File:      fileName,
			Line:      0,
			LineKnown: false,
		},
		{
			Message:   "error in nonexistent file",
			File:      "nonexistent.go",
			Line:      1,
			LineKnown: true,
		},
	}

	succeeded, failed := ExtractSnippetsForErrors(errors, basePath)

	if succeeded != 1 {
		t.Errorf("succeeded = %d, want 1", succeeded)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}

	if errors[0].CodeSnippet == nil {
		t.Error("Expected CodeSnippet to be set for first error")
	}

	if errors[1].CodeSnippet != nil {
		t.Error("Expected no CodeSnippet for error without line")
	}

	if errors[2].CodeSnippet != nil {
		t.Error("Expected no CodeSnippet for nonexistent file")
	}
}

func TestExtractSnippetsForErrors_AbsolutePath(t *testing.T) {
	tmpFile := createTempFile(t, "test.go", "package main\n")

	errors := []*ExtractedError{
		{
			Message:   "test error",
			File:      tmpFile,
			Line:      1,
			LineKnown: true,
		},
	}

	succeeded, failed := ExtractSnippetsForErrors(errors, "")

	if succeeded != 1 {
		t.Errorf("succeeded = %d, want 1", succeeded)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}
}

func TestExtractSnippet_LargeFile(t *testing.T) {
	largeContent := make([]byte, MaxFileSize+1)
	for i := range largeContent {
		largeContent[i] = 'a'
		if i%80 == 79 {
			largeContent[i] = '\n'
		}
	}
	tmpFile := createTempFile(t, "large.go", string(largeContent))

	snippet := ExtractSnippet(tmpFile, 1)
	if snippet != nil {
		t.Error("Expected nil snippet for file larger than MaxFileSize")
	}
}

func TestExtractSnippet_MaxSnippetSize(t *testing.T) {
	// Create a file with many lines that would exceed MaxSnippetSize
	var lines []string
	lineContent := "// This is a moderately long line of code that will help us exceed the MaxSnippetSize limit"
	for i := 0; i < 100; i++ {
		lines = append(lines, lineContent)
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	tmpFile := createTempFile(t, "large_lines.go", content)

	// Request a snippet from the middle with large context
	snippet := ExtractSnippetWithContext(tmpFile, 50, 50)
	if snippet == nil {
		t.Fatal("Expected non-nil snippet")
	}

	// Calculate total size of snippet
	totalSize := 0
	for _, line := range snippet.Lines {
		totalSize += len(line) + 1 // +1 for newline
	}

	// Verify snippet doesn't exceed MaxSnippetSize
	if totalSize > MaxSnippetSize+100 { // Allow some margin for last line
		t.Errorf("Snippet size %d exceeds MaxSnippetSize %d", totalSize, MaxSnippetSize)
	}

	// Verify we got fewer lines than requested (due to size limit)
	expectedMaxLines := 101 // 50 before + 1 error line + 50 after
	if len(snippet.Lines) >= expectedMaxLines {
		t.Errorf("Expected fewer than %d lines due to size limit, got %d", expectedMaxLines, len(snippet.Lines))
	}
}

func TestExtractSnippetsForErrors_PathTraversal(t *testing.T) {
	// Create a temp directory structure
	tmpDir := t.TempDir()

	// Create a file inside the allowed directory
	allowedFile := filepath.Join(tmpDir, "allowed.go")
	if err := os.WriteFile(allowedFile, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("Failed to create allowed file: %v", err)
	}

	// Create a file outside the allowed directory (parent)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.go")
	if err := os.WriteFile(outsideFile, []byte("package secret\n"), 0o600); err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}

	tests := []struct {
		name          string
		filePath      string
		basePath      string
		wantSucceeded int
		wantFailed    int
	}{
		{
			name:          "path traversal with ../",
			filePath:      "../../../etc/passwd",
			basePath:      tmpDir,
			wantSucceeded: 0,
			wantFailed:    1,
		},
		{
			name:          "path traversal with multiple ../",
			filePath:      "subdir/../../../etc/passwd",
			basePath:      tmpDir,
			wantSucceeded: 0,
			wantFailed:    1,
		},
		{
			name:          "relative path to allowed file",
			filePath:      "allowed.go",
			basePath:      tmpDir,
			wantSucceeded: 1,
			wantFailed:    0,
		},
		{
			name:          "absolute path ignores basePath",
			filePath:      allowedFile,
			basePath:      tmpDir,
			wantSucceeded: 1,
			wantFailed:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := []*ExtractedError{
				{
					Message:   "test error",
					File:      tt.filePath,
					Line:      1,
					LineKnown: true,
				},
			}

			succeeded, failed := ExtractSnippetsForErrors(errors, tt.basePath)

			if succeeded != tt.wantSucceeded {
				t.Errorf("succeeded = %d, want %d", succeeded, tt.wantSucceeded)
			}
			if failed != tt.wantFailed {
				t.Errorf("failed = %d, want %d", failed, tt.wantFailed)
			}

			// Verify no snippet was extracted for traversal attempts
			if tt.wantFailed > 0 && errors[0].CodeSnippet != nil {
				t.Error("Expected no CodeSnippet for path traversal attempt")
			}
		})
	}
}

func TestExtractSnippetsForErrors_EmptyBasePath(t *testing.T) {
	tmpFile := createTempFile(t, "test.go", "package main\n")

	// With empty basePath, relative paths won't be resolved
	errors := []*ExtractedError{
		{
			Message:   "test error",
			File:      "relative/path/file.go",
			Line:      1,
			LineKnown: true,
		},
	}

	succeeded, failed := ExtractSnippetsForErrors(errors, "")

	// Should fail because relative path without basePath won't exist
	if succeeded != 0 {
		t.Errorf("succeeded = %d, want 0", succeeded)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}

	// Absolute path should work without basePath
	errors2 := []*ExtractedError{
		{
			Message:   "test error",
			File:      tmpFile,
			Line:      1,
			LineKnown: true,
		},
	}

	succeeded2, failed2 := ExtractSnippetsForErrors(errors2, "")
	if succeeded2 != 1 {
		t.Errorf("succeeded = %d, want 1", succeeded2)
	}
	if failed2 != 0 {
		t.Errorf("failed = %d, want 0", failed2)
	}
}

func TestExtractSnippetsForErrors_InvalidBasePath(t *testing.T) {
	errors := []*ExtractedError{
		{
			Message:   "test error",
			File:      "file.go",
			Line:      1,
			LineKnown: true,
		},
	}

	// Use a non-existent base path that can't be resolved
	// This tests the error handling when filepath.Abs fails
	succeeded, failed := ExtractSnippetsForErrors(errors, string([]byte{0})) // Invalid path with null byte

	// All errors should be counted as failed when basePath is invalid
	if succeeded != 0 {
		t.Errorf("succeeded = %d, want 0", succeeded)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
}

func TestExtractSnippet_SymlinkRejection(t *testing.T) {
	// Create a real file
	tmpDir := t.TempDir()
	realFile := filepath.Join(tmpDir, "real.go")
	if err := os.WriteFile(realFile, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("Failed to create real file: %v", err)
	}

	// Create a symlink to the file
	symlinkFile := filepath.Join(tmpDir, "symlink.go")
	if err := os.Symlink(realFile, symlinkFile); err != nil {
		t.Skipf("Cannot create symlinks on this system: %v", err)
	}

	// Real file should work
	snippet := ExtractSnippet(realFile, 1)
	if snippet == nil {
		t.Error("Expected snippet for real file")
	}

	// Symlink should be rejected
	snippet = ExtractSnippet(symlinkFile, 1)
	if snippet != nil {
		t.Error("Expected nil snippet for symlink (security: symlink rejection)")
	}
}

func TestExtractSnippet_ErrorLineCalculation(t *testing.T) {
	content := `line1
line2
line3
line4
line5
line6
line7
`
	tmpFile := createTempFile(t, "test.go", content)

	tests := []struct {
		name              string
		line              int
		contextLines      int
		wantStartLine     int
		wantErrorLine     int
		wantErrorLineText string
	}{
		{
			name:              "error at line 4 with context 2",
			line:              4,
			contextLines:      2,
			wantStartLine:     2,
			wantErrorLine:     3, // 4 - 2 + 1 = 3rd position in Lines slice (1-indexed)
			wantErrorLineText: "line4",
		},
		{
			name:              "error at line 1 with context 2",
			line:              1,
			contextLines:      2,
			wantStartLine:     1,
			wantErrorLine:     1, // First line in snippet
			wantErrorLineText: "line1",
		},
		{
			name:              "error at line 7 with context 2",
			line:              7,
			contextLines:      2,
			wantStartLine:     5,
			wantErrorLine:     3, // 7 - 5 + 1 = 3rd position
			wantErrorLineText: "line7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snippet := ExtractSnippetWithContext(tmpFile, tt.line, tt.contextLines)
			if snippet == nil {
				t.Fatal("Expected non-nil snippet")
			}

			if snippet.StartLine != tt.wantStartLine {
				t.Errorf("StartLine = %d, want %d", snippet.StartLine, tt.wantStartLine)
			}

			if snippet.ErrorLine != tt.wantErrorLine {
				t.Errorf("ErrorLine = %d, want %d", snippet.ErrorLine, tt.wantErrorLine)
			}

			// Verify the actual error line content (ErrorLine is 1-indexed into Lines slice)
			if tt.wantErrorLine > 0 && tt.wantErrorLine <= len(snippet.Lines) {
				actualLine := snippet.Lines[tt.wantErrorLine-1]
				if actualLine != tt.wantErrorLineText {
					t.Errorf("Error line content = %q, want %q", actualLine, tt.wantErrorLineText)
				}
			}
		})
	}
}

func TestExtractSnippetsForErrors_NoFileOrLine(t *testing.T) {
	tmpFile := createTempFile(t, "test.go", "package main\n")

	errors := []*ExtractedError{
		{
			Message: "no file",
			File:    "",
			Line:    1,
		},
		{
			Message: "no line",
			File:    tmpFile,
			Line:    0,
		},
		{
			Message: "no file or line",
			File:    "",
			Line:    0,
		},
	}

	succeeded, failed := ExtractSnippetsForErrors(errors, "")

	// None should succeed (all skipped due to missing file or line)
	if succeeded != 0 {
		t.Errorf("succeeded = %d, want 0", succeeded)
	}
	// None should be counted as failed either (they're skipped, not failed)
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}
}

func TestIsSensitiveFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		// Exact matches
		{"env file", "/path/to/.env", true},
		{"env.local", "/path/to/.env.local", true},
		{"env.development", "/path/to/.env.development", true},
		{"env.production", "/path/to/.env.production", true},
		{"env.test", "/path/to/.env.test", true},
		{"credentials.json", "/path/to/credentials.json", true},
		{"secrets.json", "/path/to/secrets.json", true},
		{"secrets.yaml", "/path/to/secrets.yaml", true},
		{"secrets.yml", "/path/to/secrets.yml", true},
		{"netrc", "/home/user/.netrc", true},
		{"npmrc", "/home/user/.npmrc", true},
		{"pypirc", "/home/user/.pypirc", true},
		{"id_rsa", "/home/user/.ssh/id_rsa", true},
		{"id_ed25519", "/home/user/.ssh/id_ed25519", true},
		{"id_ecdsa", "/home/user/.ssh/id_ecdsa", true},
		{"id_dsa", "/home/user/.ssh/id_dsa", true},
		{"htpasswd", "/etc/htpasswd", true},
		{"shadow", "/etc/shadow", true},
		{"passwd", "/etc/passwd", true},

		// Extension matches
		{"pem file", "/path/to/cert.pem", true},
		{"key file", "/path/to/private.key", true},
		{"p12 file", "/path/to/cert.p12", true},
		{"pfx file", "/path/to/cert.pfx", true},

		// .env prefix variants
		{"env.staging", "/path/to/.env.staging", true},
		{"env.custom", "/path/to/.env.custom", true},
		{"env.foo.bar", "/path/to/.env.foo.bar", true},

		// Non-sensitive files
		{"regular go file", "/path/to/main.go", false},
		{"regular ts file", "/path/to/app.ts", false},
		{"regular json", "/path/to/config.json", false},
		{"regular yaml", "/path/to/config.yaml", false},
		{"env in path but not filename", "/path/.env/config.json", false},
		{"credentials in path", "/path/credentials/config.json", false},
		{"similar name", "/path/to/environment.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSensitiveFile(tt.filePath)
			if got != tt.want {
				t.Errorf("isSensitiveFile(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestExtractSnippet_SensitiveFileRejection(t *testing.T) {
	// Create a temp .env file
	content := "SECRET_KEY=mysecretvalue\nAPI_TOKEN=abc123\n"
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Should return nil for sensitive files
	snippet := ExtractSnippet(envFile, 1)
	if snippet != nil {
		t.Error("Expected nil snippet for .env file (sensitive)")
	}

	// Test other sensitive extensions
	keyFile := filepath.Join(tmpDir, "private.key")
	if err := os.WriteFile(keyFile, []byte("-----BEGIN RSA PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("Failed to create key file: %v", err)
	}

	snippet = ExtractSnippet(keyFile, 1)
	if snippet != nil {
		t.Error("Expected nil snippet for .key file (sensitive)")
	}
}

func createTempFile(t *testing.T, name, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, name)
	if err := os.WriteFile(tmpFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	return tmpFile
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// TestExtractSnippetsForErrors_BatchedSameFile tests that multiple errors in the same file
// are processed efficiently with a single file read (batched extraction).
func TestExtractSnippetsForErrors_BatchedSameFile(t *testing.T) {
	content := `package main

import "fmt"

func main() {
	x := 1
	y := 2
	z := 3
	w := 4
	fmt.Println(x, y, z, w)
}
`
	tmpFile := createTempFile(t, "batch.go", content)
	basePath := filepath.Dir(tmpFile)
	fileName := filepath.Base(tmpFile)

	// Create multiple errors in the same file at different lines
	errors := []*ExtractedError{
		{
			Message:   "error at line 6",
			File:      fileName,
			Line:      6,
			LineKnown: true,
		},
		{
			Message:   "error at line 8",
			File:      fileName,
			Line:      8,
			LineKnown: true,
		},
		{
			Message:   "error at line 10",
			File:      fileName,
			Line:      10,
			LineKnown: true,
		},
	}

	succeeded, failed := ExtractSnippetsForErrors(errors, basePath)

	// All three should succeed
	if succeeded != 3 {
		t.Errorf("succeeded = %d, want 3", succeeded)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}

	// Verify each error got its snippet with correct language detection
	for i, err := range errors {
		if err.CodeSnippet == nil {
			t.Errorf("error %d: CodeSnippet is nil", i)
			continue
		}
		if err.CodeSnippet.Language != "go" {
			t.Errorf("error %d: Language = %q, want %q", i, err.CodeSnippet.Language, "go")
		}
		if len(err.CodeSnippet.Lines) == 0 {
			t.Errorf("error %d: Lines is empty", i)
		}
	}

	// Verify the snippets have correct error line positions
	if errors[0].CodeSnippet.ErrorLine != 4 { // Line 6 with ±3 context starts at line 3, so error is at position 4
		t.Errorf("error 0: ErrorLine = %d, want 4", errors[0].CodeSnippet.ErrorLine)
	}
}
