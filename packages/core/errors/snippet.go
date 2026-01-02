package errors

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Snippet extraction constants
const (
	DefaultContextLines = 3           // ±3 lines around error (7 total)
	MaxLineLength       = 500         // Truncate lines longer than this
	MaxSnippetSize      = 2048        // Max total snippet size in bytes
	MaxFileSize         = 1024 * 1024 // Skip files larger than 1MB
	ScannerBufferSize   = 256 * 1024  // 256KB buffer for scanner (handles long lines in large files)
)

// sensitiveFilePatterns are file patterns that should never be read for snippets
// to prevent information disclosure of secrets and credentials
var sensitiveFilePatterns = []string{
	".env",
	".env.local",
	".env.development",
	".env.production",
	".env.test",
	"credentials.json",
	"secrets.json",
	"secrets.yaml",
	"secrets.yml",
	".netrc",
	".npmrc",
	".pypirc",
	"id_rsa",
	"id_ed25519",
	"id_ecdsa",
	"id_dsa",
	".pem",
	".key",
	".p12",
	".pfx",
	"htpasswd",
	"shadow",
	"passwd",
}

// Language detection map (extension -> language name)
var extensionToLanguage = map[string]string{
	".go":     "go",
	".ts":     "typescript",
	".tsx":    "typescript",
	".js":     "javascript",
	".jsx":    "javascript",
	".mjs":    "javascript",
	".cjs":    "javascript",
	".mts":    "typescript",
	".cts":    "typescript",
	".py":     "python",
	".pyi":    "python",
	".pyw":    "python",
	".rs":     "rust",
	".rb":     "ruby",
	".java":   "java",
	".kt":     "kotlin",
	".kts":    "kotlin",
	".swift":  "swift",
	".c":      "c",
	".h":      "c",
	".cpp":    "cpp",
	".hpp":    "cpp",
	".cc":     "cpp",
	".cxx":    "cpp",
	".cs":     "csharp",
	".php":    "php",
	".vue":    "vue",
	".svelte": "svelte",
	".astro":  "astro",
	".json":   "json",
	".yaml":   "yaml",
	".yml":    "yaml",
	".toml":   "toml",
	".md":     "markdown",
	".sql":    "sql",
	".sh":     "shell",
	".bash":   "shell",
	".zsh":    "shell",
}

// ExtractSnippet extracts source code context around an error location.
// Uses DefaultContextLines (±3 lines) for context.
// Returns nil if the file cannot be read or line is invalid.
func ExtractSnippet(filePath string, line int) *CodeSnippet {
	return ExtractSnippetWithContext(filePath, line, DefaultContextLines)
}

// ExtractSnippetWithContext extracts source code with configurable context lines.
// Returns nil if:
//   - filePath is empty
//   - line <= 0 (unknown location)
//   - file doesn't exist or can't be read
//   - file is larger than MaxFileSize
//   - file appears to be binary
//   - file is a symlink (security: prevents symlink attacks)
//   - file matches sensitive file patterns (security: prevents credential disclosure)
func ExtractSnippetWithContext(filePath string, line int, contextLines int) *CodeSnippet {
	// Validate inputs
	if filePath == "" || line <= 0 || contextLines < 0 {
		return nil
	}

	// Security: Clean the path to normalize it (removes redundant separators, . and ..)
	cleanPath := filepath.Clean(filePath)

	// Security: Check for sensitive file patterns to prevent credential disclosure
	if isSensitiveFile(cleanPath) {
		return nil
	}

	// Security: Use Lstat first to detect symlinks (prevents TOCTOU with symlinks)
	// This avoids the race condition where a file could be replaced with a symlink
	// between Stat and Open
	info, err := os.Lstat(cleanPath)
	if err != nil {
		return nil
	}

	// Security: Reject symlinks to prevent symlink attacks
	// An attacker could create a symlink to sensitive files like /etc/passwd
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	// Reject directories
	if info.IsDir() {
		return nil
	}

	// Reject files larger than MaxFileSize
	if info.Size() > MaxFileSize {
		return nil
	}

	// Open file
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	// Security: Verify the opened file matches what we stat'd using os.SameFile
	// This catches TOCTOU attacks where the file was replaced after Lstat
	// os.SameFile compares device and inode, not just size/mode
	openedInfo, err := file.Stat()
	if err != nil {
		return nil
	}

	// Use os.SameFile for robust comparison (compares device/inode on Unix)
	if !os.SameFile(info, openedInfo) {
		return nil
	}

	// Double-check size hasn't changed (defense in depth)
	if openedInfo.Size() > MaxFileSize {
		return nil
	}

	// Read lines around the error
	startLine := max(1, line-contextLines)
	endLine := line + contextLines

	// Pre-allocate slice for expected number of lines
	expectedLines := endLine - startLine + 1
	lines := make([]string, 0, expectedLines)

	scanner := bufio.NewScanner(file)
	// Use larger buffer to handle long lines without errors
	scanner.Buffer(make([]byte, ScannerBufferSize), ScannerBufferSize)

	currentLine := 0
	totalSize := 0

	for scanner.Scan() {
		currentLine++

		// Skip lines before our window
		if currentLine < startLine {
			continue
		}

		// Stop if we've passed our window
		if currentLine > endLine {
			break
		}

		lineText := scanner.Text()

		// Check for binary content (null bytes or high ratio of non-printable chars)
		if isBinaryLine(lineText) {
			return nil
		}

		// Truncate long lines
		if len(lineText) > MaxLineLength {
			lineText = truncateUTF8(lineText, MaxLineLength) + "..."
		}

		// Check total size limit
		totalSize += len(lineText) + 1 // +1 for conceptual newline
		if totalSize > MaxSnippetSize {
			break
		}

		lines = append(lines, lineText)
	}

	if err := scanner.Err(); err != nil {
		return nil
	}

	// Ensure we have at least one line
	if len(lines) == 0 {
		return nil
	}

	// Calculate error line position within snippet (1-indexed)
	errorLineInSnippet := line - startLine + 1
	if errorLineInSnippet < 1 {
		errorLineInSnippet = 1
	}
	if errorLineInSnippet > len(lines) {
		errorLineInSnippet = len(lines)
	}

	return &CodeSnippet{
		Lines:     lines,
		StartLine: startLine,
		ErrorLine: errorLineInSnippet,
		Language:  detectLanguage(filePath),
	}
}

// isSensitiveFile checks if a file path matches known sensitive file patterns
// to prevent disclosure of credentials and secrets in snippets.
func isSensitiveFile(filePath string) bool {
	// Get the base filename
	baseName := filepath.Base(filePath)

	// Check exact filename matches
	for _, pattern := range sensitiveFilePatterns {
		if baseName == pattern {
			return true
		}
	}

	// Check for .env prefix variants (e.g., .env.staging, .env.custom)
	if strings.HasPrefix(baseName, ".env") {
		return true
	}

	// Check file extension for sensitive types
	ext := strings.ToLower(filepath.Ext(baseName))
	switch ext {
	case ".pem", ".key", ".p12", ".pfx":
		return true
	}

	return false
}

// detectLanguage determines the programming language from file extension
func detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if lang, ok := extensionToLanguage[ext]; ok {
		return lang
	}
	return "text"
}

// isBinaryLine checks if a line appears to be from a binary file
func isBinaryLine(line string) bool {
	if len(line) == 0 {
		return false
	}

	// Check for null bytes (definite binary indicator)
	if strings.ContainsRune(line, 0) {
		return true
	}

	// Check ratio of non-printable characters
	nonPrintable := 0
	total := 0
	for _, r := range line {
		total++
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			nonPrintable++
		}
	}

	// More than 10% non-printable is likely binary
	if total > 0 && float64(nonPrintable)/float64(total) > 0.1 {
		return true
	}

	return false
}

// truncateUTF8 safely truncates a string to maxBytes without breaking UTF-8 sequences
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	// Find the last valid UTF-8 boundary before maxBytes
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}

	return s[:maxBytes]
}

// errorWithPath pairs an error with its resolved file path for batched processing
type errorWithPath struct {
	err      *ExtractedError
	filePath string
}

// ExtractSnippetsForErrors adds code snippets to all errors that have valid file+line.
// This modifies the errors in place.
// Returns counts of successes and failures for AIContext metrics.
// Security: When basePath is provided, paths are validated to prevent directory traversal attacks.
// Performance: Batches file reads so each file is only read once, even with multiple errors.
func ExtractSnippetsForErrors(errors []*ExtractedError, basePath string) (succeeded, failed int) {
	// Security: Clean and resolve basePath once if provided
	var cleanBasePath string
	if basePath != "" {
		var err error
		cleanBasePath, err = filepath.Abs(filepath.Clean(basePath))
		if err != nil {
			// If we can't resolve basePath, fail safely
			return 0, len(errors)
		}
	}

	// Group errors by resolved file path for batched reading
	fileGroups := make(map[string][]errorWithPath)

	for _, err := range errors {
		// Skip if no file or no valid line number
		// Note: Line > 0 is sufficient - LineKnown only disambiguates Line=0 (unknown vs actual line 0)
		if err.File == "" || err.Line <= 0 {
			continue
		}

		// Resolve file path
		filePath := err.File
		if cleanBasePath != "" && !filepath.IsAbs(filePath) {
			// Security: Join paths and then verify the result is still under basePath
			// This prevents path traversal attacks like "../../etc/passwd"
			filePath = filepath.Join(cleanBasePath, filePath)

			// Security: Clean the resulting path and verify it's still under basePath
			cleanFilePath := filepath.Clean(filePath)
			if !strings.HasPrefix(cleanFilePath, cleanBasePath+string(filepath.Separator)) &&
				cleanFilePath != cleanBasePath {
				// Path traversal attempt detected - skip this file
				failed++
				continue
			}
			filePath = cleanFilePath
		}

		fileGroups[filePath] = append(fileGroups[filePath], errorWithPath{err: err, filePath: filePath})
	}

	// Process each file once, extracting all needed snippets
	for filePath, errorsInFile := range fileGroups {
		// For single error, use the simple extraction path
		if len(errorsInFile) == 1 {
			snippet := ExtractSnippet(filePath, errorsInFile[0].err.Line)
			if snippet != nil {
				errorsInFile[0].err.CodeSnippet = snippet
				succeeded++
			} else {
				failed++
			}
			continue
		}

		// Multiple errors in same file - batch read
		s, f := extractSnippetsBatched(filePath, errorsInFile)
		succeeded += s
		failed += f
	}
	return succeeded, failed
}

// extractSnippetsBatched reads a file once and extracts snippets for multiple error locations.
// This is more efficient than reading the file multiple times.
func extractSnippetsBatched(filePath string, errorsInFile []errorWithPath) (succeeded, failed int) {
	// Security: Clean the path to normalize it
	cleanPath := filepath.Clean(filePath)

	// Security: Check for sensitive file patterns
	if isSensitiveFile(cleanPath) {
		return 0, len(errorsInFile)
	}

	// Security: Use Lstat first to detect symlinks
	info, err := os.Lstat(cleanPath)
	if err != nil {
		return 0, len(errorsInFile)
	}

	// Security: Reject symlinks to prevent symlink attacks
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, len(errorsInFile)
	}

	// Reject directories
	if info.IsDir() {
		return 0, len(errorsInFile)
	}

	// Reject files larger than MaxFileSize
	if info.Size() > MaxFileSize {
		return 0, len(errorsInFile)
	}

	// Open file
	file, err := os.Open(cleanPath)
	if err != nil {
		return 0, len(errorsInFile)
	}
	defer file.Close()

	// Security: Verify the opened file matches what we stat'd using os.SameFile
	openedInfo, err := file.Stat()
	if err != nil {
		return 0, len(errorsInFile)
	}

	if !os.SameFile(info, openedInfo) {
		return 0, len(errorsInFile)
	}

	// Sort errors by line number for efficient single-pass reading
	sort.Slice(errorsInFile, func(i, j int) bool {
		return errorsInFile[i].err.Line < errorsInFile[j].err.Line
	})

	// Calculate the overall range we need to read
	minLine := errorsInFile[0].err.Line - DefaultContextLines
	if minLine < 1 {
		minLine = 1
	}
	maxLine := errorsInFile[len(errorsInFile)-1].err.Line + DefaultContextLines

	// Read all lines in the range into memory
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, ScannerBufferSize), ScannerBufferSize)

	// Store lines indexed by line number
	lineCache := make(map[int]string)
	currentLine := 0
	isBinary := false

	for scanner.Scan() {
		currentLine++

		// Skip lines before our window
		if currentLine < minLine {
			continue
		}

		// Stop if we've passed our window
		if currentLine > maxLine {
			break
		}

		lineText := scanner.Text()

		// Check for binary content
		if isBinaryLine(lineText) {
			isBinary = true
			break
		}

		// Truncate long lines
		if len(lineText) > MaxLineLength {
			lineText = truncateUTF8(lineText, MaxLineLength) + "..."
		}

		lineCache[currentLine] = lineText
	}

	if err := scanner.Err(); err != nil || isBinary {
		return 0, len(errorsInFile)
	}

	// Detect language once for the file
	language := detectLanguage(filePath)

	// Now extract snippets for each error using the cached lines
	for _, ewp := range errorsInFile {
		line := ewp.err.Line
		startLine := max(1, line-DefaultContextLines)
		endLine := line + DefaultContextLines

		// Collect lines for this snippet
		var lines []string
		totalSize := 0
		for l := startLine; l <= endLine; l++ {
			lineText, ok := lineCache[l]
			if !ok {
				continue
			}
			totalSize += len(lineText) + 1
			if totalSize > MaxSnippetSize {
				break
			}
			lines = append(lines, lineText)
		}

		if len(lines) == 0 {
			failed++
			continue
		}

		// Calculate error line position within snippet
		errorLineInSnippet := line - startLine + 1
		if errorLineInSnippet < 1 {
			errorLineInSnippet = 1
		}
		if errorLineInSnippet > len(lines) {
			errorLineInSnippet = len(lines)
		}

		ewp.err.CodeSnippet = &CodeSnippet{
			Lines:     lines,
			StartLine: startLine,
			ErrorLine: errorLineInSnippet,
			Language:  language,
		}
		succeeded++
	}

	return succeeded, failed
}
