/**
 * Persistence types for Detent
 * Ported from Go: apps/go-cli/internal/persistence/types.go
 */

// ============================================================================
// Status Enums
// ============================================================================

export type HealStatus = "pending" | "applied" | "rejected" | "failed";

export type VerificationResult = "passed" | "failed";

export type AssignmentStatus =
  | "assigned"
  | "in_progress"
  | "completed"
  | "failed"
  | "expired";

export type FixStatus = "pending" | "applied" | "rejected" | "superseded";

export type ErrorStatus = "open" | "resolved";

export type SyncStatus = "pending" | "synced" | "failed";

// ============================================================================
// Core Data Types
// ============================================================================

/**
 * Run represents a workflow run stored in the database
 */
export interface Run {
  runId: string;
  workflowName: string;
  commitSha: string;
  treeHash?: string;
  executionMode: string;
  startedAt: Date;
  completedAt?: Date;
  totalErrors?: number;
  syncStatus?: SyncStatus;
}

/**
 * FindingRecord represents a single finding for persistence.
 * This is written as each error is extracted during scanning.
 */
export interface FindingRecord {
  timestamp: Date;
  runId: string;

  // File information
  filePath: string;
  fileHash?: string;

  // Error details
  message: string;
  line: number;
  column: number;
  severity: "error" | "warning";
  stackTrace?: string;
  ruleId?: string;
  category: string; // lint, type-check, test, compile, runtime
  source: string; // eslint, typescript, go, etc.

  // Workflow context
  workflowJob?: string;
  workflowStep?: string;

  // Raw output for compliance/debugging
  raw?: string;

  // Pre-computed content hash (optional, computed lazily if empty)
  contentHash?: string;
}

/**
 * ErrorRecord represents an error stored in the database
 */
export interface ErrorRecord {
  errorId: string;
  runId: string;
  filePath: string;
  lineNumber: number;
  columnNumber?: number;
  errorType: string;
  message: string;
  stackTrace?: string;
  fileHash?: string;
  contentHash: string;
  severity?: string;
  ruleId?: string;
  source?: string;
  workflowJob?: string;
  raw?: string;
  firstSeenAt: Date;
  lastSeenAt: Date;
  seenCount: number;
  status: ErrorStatus;
  syncStatus?: SyncStatus;
}

/**
 * HealRecord represents an AI-recommended fix for an error
 */
export interface HealRecord {
  healId: string;
  errorId: string;
  runId?: string;
  diffContent: string;
  diffContentHash?: string;
  filePath?: string;
  fileHash?: string;
  modelId?: string;
  promptHash?: string;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  costUsd: number;
  status: HealStatus;
  createdAt: Date;
  appliedAt?: Date;
  verifiedAt?: Date;
  verificationResult?: VerificationResult;
  attemptNumber: number;
  parentHealId?: string;
  failureReason?: string;
  syncStatus?: SyncStatus;
}

/**
 * ErrorLocation tracks where an error appears across runs
 */
export interface ErrorLocation {
  locationId: string;
  errorId: string;
  runId: string;
  filePath: string;
  lineNumber: number;
  columnNumber?: number;
  fileHash?: string;
  firstSeenAt: Date;
  lastSeenAt: Date;
  seenCount: number;
}

// ============================================================================
// Assignment Types (Phase 1 Fix Protocol)
// ============================================================================

/**
 * Assignment represents a batch of errors assigned to a single agent for resolution.
 * Each agent gets its own isolated environment (worktree, sandbox) to prevent conflicts.
 */
export interface Assignment {
  assignmentId: string;
  runId: string;
  agentId: string;
  worktreePath?: string;
  errorCount: number;
  errorIds: string[];
  status: AssignmentStatus;
  createdAt: Date;
  startedAt?: Date;
  completedAt?: Date;
  expiresAt: Date;
  fixId?: string;
  failureReason?: string;
}

// ============================================================================
// SuggestedFix Types (Phase 1 Fix Protocol)
// ============================================================================

/**
 * FileChange captures a single file modification.
 * Stores both full content (for apply/re-verify) and diff (for display).
 */
export interface FileChange {
  beforeContent?: string;
  afterContent?: string;
  unifiedDiff?: string;
  linesAdded: number;
  linesRemoved: number;
  isNew?: boolean;
  isDeleted?: boolean;
}

/**
 * VerificationRecord captures evidence that the fix works.
 */
export interface VerificationRecord {
  command: string;
  exitCode: number;
  output?: string;
  durationMs: number;
  errorsBefore: number;
  errorsAfter: number;
}

/**
 * SuggestedFix represents an AI-proposed fix for one or more errors.
 * The fix is content-addressed: same code changes = same FixID.
 */
export interface SuggestedFix {
  fixId: string;
  assignmentId: string;
  agentId?: string;
  worktreePath?: string;
  fileChanges: Record<string, FileChange>;
  explanation?: string;
  confidence: number; // 1-100
  verification: VerificationRecord;
  modelId?: string;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
  status: FixStatus;
  createdAt: Date;
  appliedAt?: Date;
  appliedBy?: string;
  appliedCommitSha?: string;
  rejectedAt?: Date;
  rejectedBy?: string;
  rejectionReason?: string;
  errorIds?: string[];
}

// ============================================================================
// Spend Tracking
// ============================================================================

/**
 * SpendLogEntry represents a single spend entry
 */
export interface SpendLogEntry {
  id: number;
  costUsd: number;
  createdAt: Date;
}

// ============================================================================
// Heal Locks
// ============================================================================

/**
 * HealLock represents an advisory lock for preventing concurrent heal processes
 */
export interface HealLock {
  repoPath: string;
  holderId: string;
  acquiredAt: Date;
  expiresAt: Date;
  pid?: number;
}

// ============================================================================
// Config Types
// ============================================================================

/**
 * GlobalConfig is the raw structure that gets persisted to disk
 * Used for both per-repo .detent/config.json and legacy ~/.detent/detent.json
 */
export interface GlobalConfig {
  $schema?: string;
  apiKey?: string;
  model?: string;
  budgetPerRunUsd?: number;
  budgetMonthlyUsd?: number;
  timeoutMins?: number;
}

/**
 * Config is the merged, resolved config used by the application
 */
export interface Config {
  apiKey: string;
  model: string;
  budgetPerRunUsd: number;
  budgetMonthlyUsd: number;
  timeoutMins: number;
}

// ============================================================================
// ExtractedError (from core/errors for recorder)
// ============================================================================

/**
 * WorkflowContext for recording findings
 */
export interface WorkflowContext {
  job: string;
  step: string;
}

/**
 * ExtractedError is the input format from error extraction
 */
export interface ExtractedError {
  file: string;
  line: number;
  column: number;
  message: string;
  severity: string;
  stackTrace?: string;
  ruleId?: string;
  category: string;
  source: string;
  workflowContext?: WorkflowContext;
  raw?: string;
}
