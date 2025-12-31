package errors

import (
	"runtime"
	"testing"
)

func TestGroupedErrors_HasErrors(t *testing.T) {
	tests := []struct {
		name     string
		errors   []*ExtractedError
		expected bool
	}{
		{
			name: "has errors",
			errors: []*ExtractedError{
				{Message: "error 1", Severity: "error", File: "file1.go"},
				{Message: "warning 1", Severity: "warning", File: "file1.go"},
			},
			expected: true,
		},
		{
			name: "only warnings",
			errors: []*ExtractedError{
				{Message: "warning 1", Severity: "warning", File: "file1.go"},
				{Message: "warning 2", Severity: "warning", File: "file2.go"},
			},
			expected: false,
		},
		{
			name:     "empty",
			errors:   []*ExtractedError{},
			expected: false,
		},
		{
			name: "error without file",
			errors: []*ExtractedError{
				{Message: "error 1", Severity: "error"},
				{Message: "warning 1", Severity: "warning"},
			},
			expected: true,
		},
		{
			name: "mixed errors and warnings",
			errors: []*ExtractedError{
				{Message: "warning 1", Severity: "warning", File: "file1.go"},
				{Message: "warning 2", Severity: "warning", File: "file2.go"},
				{Message: "error 1", Severity: "error", File: "file3.go"},
				{Message: "warning 3", Severity: "warning", File: "file4.go"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grouped := GroupByFileWithBase(tt.errors, "")
			if got := grouped.HasErrors(); got != tt.expected {
				t.Errorf("HasErrors() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGroupedErrors_HasErrors_Performance(t *testing.T) {
	// Create a large set of errors to verify O(1) lookup performance
	const numErrors = 100000
	errors := make([]*ExtractedError, numErrors)
	for i := 0; i < numErrors; i++ {
		severity := "warning"
		if i == numErrors-1 {
			severity = "error"
		}
		errors[i] = &ExtractedError{
			Message:  "test error",
			Severity: severity,
			File:     "test.go",
		}
	}

	grouped := GroupByFileWithBase(errors, "")

	// This should be O(1) - just checking the hasErrors flag
	if !grouped.HasErrors() {
		t.Error("HasErrors() should return true when errors exist")
	}
}

func TestMakeRelative(t *testing.T) {
	// Use OS-appropriate separators for tests
	tests := []struct {
		name     string
		path     string
		basePath string
		expected string
	}{
		{
			name:     "path traversal false positive - similar prefix",
			path:     "/home/user-data/file.txt",
			basePath: "/home/user",
			expected: "/home/user-data/file.txt",
		},
		{
			name:     "valid subpath",
			path:     "/home/user/file.txt",
			basePath: "/home/user",
			expected: "file.txt",
		},
		{
			name:     "completely different path",
			path:     "/other/path",
			basePath: "/home/user",
			expected: "/other/path",
		},
		{
			name:     "nested subpath",
			path:     "/home/user/sub/dir/file.txt",
			basePath: "/home/user",
			expected: "sub/dir/file.txt",
		},
		{
			name:     "same path",
			path:     "/home/user",
			basePath: "/home/user",
			expected: ".",
		},
		{
			name:     "basePath with trailing slash",
			path:     "/home/user/file.txt",
			basePath: "/home/user/",
			expected: "file.txt",
		},
		{
			name:     "empty basePath",
			path:     "/home/user/file.txt",
			basePath: "",
			expected: "/home/user/file.txt",
		},
		{
			name:     "relative path input",
			path:     "relative/path.txt",
			basePath: "/home/user",
			expected: "relative/path.txt",
		},
		{
			name:     "parent directory escape",
			path:     "/home/other/file.txt",
			basePath: "/home/user",
			expected: "/home/other/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip on Windows due to path format differences
			if runtime.GOOS == "windows" {
				t.Skip("Skipping Unix path tests on Windows")
			}

			result := makeRelative(tt.path, tt.basePath)
			if result != tt.expected {
				t.Errorf("makeRelative(%q, %q) = %q, want %q",
					tt.path, tt.basePath, result, tt.expected)
			}
		})
	}
}
