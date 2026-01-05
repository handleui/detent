package persistence

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

// Validation errors
var (
	ErrInvalidID         = errors.New("invalid ID format")
	ErrIDTooLong         = errors.New("ID exceeds maximum length")
	ErrPathTraversal     = errors.New("path traversal detected")
	ErrInvalidPath       = errors.New("invalid path")
	ErrFieldTooLong      = errors.New("field exceeds maximum length")
	ErrInvalidConfidence = errors.New("confidence must be between 0 and 100")
	ErrInvalidStatus     = errors.New("invalid status value")
	ErrEmptyRequired     = errors.New("required field is empty")
)

// Field length limits to prevent DoS via oversized inputs
const (
	MaxIDLength          = 128
	MaxPathLength        = 4096
	MaxExplanationLength = 65536    // 64KB for explanations
	MaxReasonLength      = 4096     // 4KB for rejection/failure reasons
	MaxOutputLength      = 1048576  // 1MB for verification output
	MaxDiffLength        = 10485760 // 10MB for diffs
)

// idPattern validates IDs: alphanumeric, hyphens, underscores only
var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ValidateID checks that an ID is safe and well-formed.
// IDs must be alphanumeric with hyphens/underscores, max 128 chars.
func ValidateID(id, fieldName string) error {
	if id == "" {
		return ErrEmptyRequired
	}
	if len(id) > MaxIDLength {
		return ErrIDTooLong
	}
	if !idPattern.MatchString(id) {
		return ErrInvalidID
	}
	return nil
}

// ValidateOptionalID checks an optional ID (allows empty).
func ValidateOptionalID(id string) error {
	if id == "" {
		return nil
	}
	return ValidateID(id, "")
}

// ValidatePath checks for path traversal attacks and validates path format.
// basePath is optional - if provided, path must be within basePath.
func ValidatePath(path, basePath string) error {
	if path == "" {
		return nil // Empty paths are allowed (optional fields)
	}

	if len(path) > MaxPathLength {
		return ErrFieldTooLong
	}

	// Check for null bytes (common injection vector)
	if strings.ContainsRune(path, 0) {
		return ErrInvalidPath
	}

	// Clean the path to normalize it
	cleanPath := filepath.Clean(path)

	// Reject paths that try to escape via ..
	if strings.Contains(cleanPath, "..") {
		return ErrPathTraversal
	}

	// If base path is provided, verify containment
	if basePath != "" {
		cleanBase := filepath.Clean(basePath)
		// Absolute path check
		if filepath.IsAbs(cleanPath) {
			if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) && cleanPath != cleanBase {
				return ErrPathTraversal
			}
		}
	}

	return nil
}

// ValidateFilePaths validates all file paths in a FileChanges map.
// Returns error if any path is invalid or attempts traversal.
func ValidateFilePaths(fileChanges map[string]FileChange, basePath string) error {
	for path := range fileChanges {
		if err := ValidatePath(path, basePath); err != nil {
			return err
		}
	}
	return nil
}

// ValidateStringLength checks that a string doesn't exceed the given length.
func ValidateStringLength(s string, maxLen int) error {
	if len(s) > maxLen {
		return ErrFieldTooLong
	}
	return nil
}

// ValidateConfidence checks that confidence is in valid range [0, 100].
// 0 means "use default" (database default is 80).
func ValidateConfidence(confidence int) error {
	if confidence < 0 || confidence > 100 {
		return ErrInvalidConfidence
	}
	return nil
}

// ValidateAssignmentStatus checks that status is a valid AssignmentStatus value.
func ValidateAssignmentStatus(status AssignmentStatus) error {
	switch status {
	case AssignmentStatusAssigned, AssignmentStatusInProgress,
		AssignmentStatusCompleted, AssignmentStatusFailed, AssignmentStatusExpired:
		return nil
	default:
		return ErrInvalidStatus
	}
}

// ValidateFixStatus checks that status is a valid FixStatus value.
func ValidateFixStatus(status FixStatus) error {
	switch status {
	case FixStatusPending, FixStatusApplied, FixStatusRejected, FixStatusSuperseded:
		return nil
	default:
		return ErrInvalidStatus
	}
}

// ValidateAssignment performs full validation on an Assignment struct.
func ValidateAssignment(a *Assignment) error {
	if err := ValidateID(a.AssignmentID, "assignment_id"); err != nil {
		return err
	}
	if err := ValidateID(a.RunID, "run_id"); err != nil {
		return err
	}
	if err := ValidateID(a.AgentID, "agent_id"); err != nil {
		return err
	}
	if err := ValidatePath(a.WorktreePath, ""); err != nil {
		return err
	}
	if err := ValidateAssignmentStatus(a.Status); err != nil {
		return err
	}
	if err := ValidateOptionalID(a.FixID); err != nil {
		return err
	}
	if err := ValidateStringLength(a.FailureReason, MaxReasonLength); err != nil {
		return err
	}
	// Validate error IDs
	for _, errorID := range a.ErrorIDs {
		if err := ValidateID(errorID, "error_id"); err != nil {
			return err
		}
	}
	return nil
}

// ValidateSuggestedFix performs full validation on a SuggestedFix struct.
func ValidateSuggestedFix(fix *SuggestedFix) error {
	// FixID may be empty (computed later) but if set must be valid
	if fix.FixID != "" {
		if err := ValidateID(fix.FixID, "fix_id"); err != nil {
			return err
		}
	}
	if err := ValidateID(fix.AssignmentID, "assignment_id"); err != nil {
		return err
	}
	if err := ValidateOptionalID(fix.AgentID); err != nil {
		return err
	}
	if err := ValidatePath(fix.WorktreePath, ""); err != nil {
		return err
	}
	if err := ValidateFilePaths(fix.FileChanges, ""); err != nil {
		return err
	}
	if err := ValidateStringLength(fix.Explanation, MaxExplanationLength); err != nil {
		return err
	}
	if err := ValidateConfidence(fix.Confidence); err != nil {
		return err
	}
	if err := ValidateFixStatus(fix.Status); err != nil {
		return err
	}
	if err := ValidateStringLength(fix.Verification.Output, MaxOutputLength); err != nil {
		return err
	}
	if err := ValidateStringLength(fix.RejectionReason, MaxReasonLength); err != nil {
		return err
	}
	// Validate error IDs
	for _, errorID := range fix.ErrorIDs {
		if err := ValidateID(errorID, "error_id"); err != nil {
			return err
		}
	}
	// Validate file change content sizes
	for _, change := range fix.FileChanges {
		if err := ValidateStringLength(change.UnifiedDiff, MaxDiffLength); err != nil {
			return err
		}
		if err := ValidateStringLength(change.BeforeContent, MaxDiffLength); err != nil {
			return err
		}
		if err := ValidateStringLength(change.AfterContent, MaxDiffLength); err != nil {
			return err
		}
	}
	return nil
}
