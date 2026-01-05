package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// ComputeFixID generates a content-addressed fix ID from file changes.
// The ID is deterministic: same changes = same ID, enabling deduplication.
// Returns first 20 hex chars (80 bits - sufficient for uniqueness).
// Returns empty string for nil or empty maps (no changes = no ID).
func ComputeFixID(fileChanges map[string]FileChange) string {
	if len(fileChanges) == 0 {
		return ""
	}

	h := sha256.New()

	// Sort file paths for deterministic ordering
	paths := make([]string, 0, len(fileChanges))
	for path := range fileChanges {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	// Hash each file's content change
	for _, path := range paths {
		change := fileChanges[path]
		h.Write([]byte(path))
		h.Write([]byte{0}) // null separator

		// Use unified diff if available, else use after content
		content := change.UnifiedDiff
		if content == "" {
			content = change.AfterContent
		}
		h.Write([]byte(content))
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))[:20]
}
