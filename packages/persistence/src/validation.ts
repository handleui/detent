/**
 * Validation utilities for Detent persistence layer
 * Ported from Go: apps/go-cli/internal/persistence/validation.go
 *
 * Provides input validation to prevent DoS via oversized inputs,
 * path traversal attacks, and invalid data.
 */

import { isAbsolute, normalize, sep } from "node:path";
import type {
  Assignment,
  AssignmentStatus,
  FileChange,
  FixStatus,
  SuggestedFix,
} from "./types.js";

// ============================================================================
// Constants - Field length limits to prevent DoS via oversized inputs
// ============================================================================

export const MaxIDLength = 128;
export const MaxPathLength = 4096;
export const MaxExplanationLength = 65_536; // 64KB for explanations
export const MaxReasonLength = 4096; // 4KB for rejection/failure reasons
export const MaxOutputLength = 1_048_576; // 1MB for verification output
export const MaxDiffLength = 10_485_760; // 10MB for diffs

// ============================================================================
// Error Classes
// ============================================================================

export class ValidationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ValidationError";
  }
}

export class ErrInvalidID extends ValidationError {
  constructor(message = "invalid ID format") {
    super(message);
    this.name = "ErrInvalidID";
  }
}

export class ErrIDTooLong extends ValidationError {
  constructor(message = "ID exceeds maximum length") {
    super(message);
    this.name = "ErrIDTooLong";
  }
}

export class ErrPathTraversal extends ValidationError {
  constructor(message = "path traversal detected") {
    super(message);
    this.name = "ErrPathTraversal";
  }
}

export class ErrInvalidPath extends ValidationError {
  constructor(message = "invalid path") {
    super(message);
    this.name = "ErrInvalidPath";
  }
}

export class ErrFieldTooLong extends ValidationError {
  constructor(message = "field exceeds maximum length") {
    super(message);
    this.name = "ErrFieldTooLong";
  }
}

export class ErrInvalidConfidence extends ValidationError {
  constructor(message = "confidence must be between 0 and 100") {
    super(message);
    this.name = "ErrInvalidConfidence";
  }
}

export class ErrInvalidStatus extends ValidationError {
  constructor(message = "invalid status value") {
    super(message);
    this.name = "ErrInvalidStatus";
  }
}

export class ErrEmptyRequired extends ValidationError {
  constructor(message = "required field is empty") {
    super(message);
    this.name = "ErrEmptyRequired";
  }
}

// ============================================================================
// ID Validation
// ============================================================================

// idPattern validates IDs: alphanumeric, hyphens, underscores only
const idPattern = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/;

/**
 * Validates that an ID is safe and well-formed.
 * IDs must be alphanumeric with hyphens/underscores, max 128 chars.
 * @throws {ErrEmptyRequired} if ID is empty
 * @throws {ErrIDTooLong} if ID exceeds MaxIDLength
 * @throws {ErrInvalidID} if ID contains invalid characters
 */
export const validateID = (id: string, _fieldName?: string): void => {
  if (id === "") {
    throw new ErrEmptyRequired();
  }
  if (id.length > MaxIDLength) {
    throw new ErrIDTooLong();
  }
  if (!idPattern.test(id)) {
    throw new ErrInvalidID();
  }
};

/**
 * Validates an optional ID (allows empty).
 * @throws {ErrIDTooLong} if ID exceeds MaxIDLength
 * @throws {ErrInvalidID} if ID contains invalid characters
 */
export const validateOptionalID = (id: string): void => {
  if (id === "") {
    return;
  }
  validateID(id);
};

// ============================================================================
// Path Validation
// ============================================================================

/**
 * Validates a path for traversal attacks and validates path format.
 * @param path - The path to validate (empty paths are allowed for optional fields)
 * @param basePath - Optional base path; if provided, path must be within basePath
 * @throws {ErrFieldTooLong} if path exceeds MaxPathLength
 * @throws {ErrInvalidPath} if path contains null bytes
 * @throws {ErrPathTraversal} if path attempts traversal via ..
 */
export const validatePath = (path: string, basePath?: string): void => {
  if (path === "") {
    return; // Empty paths are allowed (optional fields)
  }

  if (path.length > MaxPathLength) {
    throw new ErrFieldTooLong();
  }

  // Check for null bytes (common injection vector)
  if (path.includes("\0")) {
    throw new ErrInvalidPath();
  }

  // Clean the path to normalize it
  const cleanPath = normalize(path);

  // Reject paths that try to escape via ..
  if (cleanPath.includes("..")) {
    throw new ErrPathTraversal();
  }

  // If base path is provided, verify containment
  if (basePath) {
    const cleanBase = normalize(basePath);
    // Absolute path check
    if (
      isAbsolute(cleanPath) &&
      !cleanPath.startsWith(cleanBase + sep) &&
      cleanPath !== cleanBase
    ) {
      throw new ErrPathTraversal();
    }
  }
};

/**
 * Validates all file paths in a FileChanges map.
 * @throws {ErrFieldTooLong} if any path exceeds MaxPathLength
 * @throws {ErrInvalidPath} if any path contains null bytes
 * @throws {ErrPathTraversal} if any path attempts traversal
 */
export const validateFilePaths = (
  fileChanges: Record<string, FileChange>,
  basePath?: string
): void => {
  for (const path of Object.keys(fileChanges)) {
    validatePath(path, basePath);
  }
};

// ============================================================================
// String Length Validation
// ============================================================================

/**
 * Validates that a string doesn't exceed the given length.
 * @throws {ErrFieldTooLong} if string exceeds maxLen
 */
export const validateStringLength = (s: string, maxLen: number): void => {
  if (s.length > maxLen) {
    throw new ErrFieldTooLong();
  }
};

// ============================================================================
// Confidence Validation
// ============================================================================

/**
 * Validates that confidence is in valid range [0, 100].
 * 0 means "use default" (database default is 80).
 * @throws {ErrInvalidConfidence} if confidence is outside valid range
 */
export const validateConfidence = (confidence: number): void => {
  if (confidence < 0 || confidence > 100) {
    throw new ErrInvalidConfidence();
  }
};

// ============================================================================
// Status Validation
// ============================================================================

const validAssignmentStatuses: AssignmentStatus[] = [
  "assigned",
  "in_progress",
  "completed",
  "failed",
  "expired",
];

/**
 * Validates that status is a valid AssignmentStatus value.
 * @throws {ErrInvalidStatus} if status is not a valid AssignmentStatus
 */
export const validateAssignmentStatus = (status: AssignmentStatus): void => {
  if (!validAssignmentStatuses.includes(status)) {
    throw new ErrInvalidStatus();
  }
};

const validFixStatuses: FixStatus[] = [
  "pending",
  "applied",
  "rejected",
  "superseded",
];

/**
 * Validates that status is a valid FixStatus value.
 * @throws {ErrInvalidStatus} if status is not a valid FixStatus
 */
export const validateFixStatus = (status: FixStatus): void => {
  if (!validFixStatuses.includes(status)) {
    throw new ErrInvalidStatus();
  }
};

// ============================================================================
// Assignment Validation
// ============================================================================

/**
 * Performs full validation on an Assignment struct.
 * @throws {ValidationError} if any field is invalid
 */
export const validateAssignment = (a: Assignment): void => {
  validateID(a.assignmentId, "assignment_id");
  validateID(a.runId, "run_id");
  validateID(a.agentId, "agent_id");
  if (a.worktreePath) {
    validatePath(a.worktreePath);
  }
  validateAssignmentStatus(a.status);
  if (a.fixId) {
    validateOptionalID(a.fixId);
  }
  if (a.failureReason) {
    validateStringLength(a.failureReason, MaxReasonLength);
  }
  // Validate error IDs
  for (const errorId of a.errorIds) {
    validateID(errorId, "error_id");
  }
};

// ============================================================================
// SuggestedFix Validation
// ============================================================================

/**
 * Performs full validation on a SuggestedFix struct.
 * @throws {ValidationError} if any field is invalid
 */
export const validateSuggestedFix = (fix: SuggestedFix): void => {
  // FixID may be empty (computed later) but if set must be valid
  if (fix.fixId !== "") {
    validateID(fix.fixId, "fix_id");
  }
  validateID(fix.assignmentId, "assignment_id");
  if (fix.agentId) {
    validateOptionalID(fix.agentId);
  }
  if (fix.worktreePath) {
    validatePath(fix.worktreePath);
  }
  validateFilePaths(fix.fileChanges);
  if (fix.explanation) {
    validateStringLength(fix.explanation, MaxExplanationLength);
  }
  validateConfidence(fix.confidence);
  validateFixStatus(fix.status);
  if (fix.verification.output) {
    validateStringLength(fix.verification.output, MaxOutputLength);
  }
  if (fix.rejectionReason) {
    validateStringLength(fix.rejectionReason, MaxReasonLength);
  }
  // Validate error IDs
  if (fix.errorIds) {
    for (const errorId of fix.errorIds) {
      validateID(errorId, "error_id");
    }
  }
  // Validate file change content sizes
  for (const change of Object.values(fix.fileChanges)) {
    if (change.unifiedDiff) {
      validateStringLength(change.unifiedDiff, MaxDiffLength);
    }
    if (change.beforeContent) {
      validateStringLength(change.beforeContent, MaxDiffLength);
    }
    if (change.afterContent) {
      validateStringLength(change.afterContent, MaxDiffLength);
    }
  }
};
