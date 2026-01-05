/**
 * @detent/persistence
 *
 * SQLite persistence layer for Detent CLI
 * Provides database management, schema migrations, and CRUD operations
 */

// biome-ignore-all lint/performance/noBarrelFile: This is the package's public API

// ============================================================================
// Types
// ============================================================================

export type {
  Assignment,
  AssignmentStatus,
  Config,
  ErrorLocation,
  ErrorRecord,
  ErrorStatus,
  ExtractedError,
  FileChange,
  FindingRecord,
  FixStatus,
  GlobalConfig,
  HealLock,
  HealRecord,
  HealStatus,
  RepoCommands,
  RepoJobOverrides,
  Run,
  SpendLogEntry,
  SuggestedFix,
  SyncStatus,
  TrustedRepo,
  VerificationRecord,
  VerificationResult,
  WorkflowContext,
} from "./types.js";

// ============================================================================
// Database
// ============================================================================

export {
  closeDatabase,
  computeRepoId,
  createDatabase,
  createInMemoryDatabase,
  databaseExists,
  ErrHealLockHeld,
  getDatabasePath,
  getDatabaseSize,
  getDetentDir,
} from "./database.js";

// ============================================================================
// Schema
// ============================================================================

export {
  CURRENT_SCHEMA_VERSION,
  getSchemaVersion,
  initSchema,
  migrations,
} from "./schema.js";

// ============================================================================
// Runs
// ============================================================================

export {
  deleteRun,
  finalizeRun,
  getAllRuns,
  getRunByCommit,
  getRunById,
  recordRun,
  runExists,
} from "./runs.js";

// ============================================================================
// Errors
// ============================================================================

export {
  computeContentHash,
  deleteOrphanErrors,
  getErrorById,
  getErrorCountByRunId,
  getErrorsByRunId,
  recordError,
  recordFindings,
  updateErrorStatus,
} from "./errors.js";

// ============================================================================
// Assignments
// ============================================================================

export {
  createAssignment,
  getAssignment,
  listAssignmentsByRun,
  updateAssignmentStatus,
} from "./assignments.js";

// ============================================================================
// Suggested Fixes
// ============================================================================

export {
  computeFixId,
  getSuggestedFix,
  listPendingFixes,
  storeSuggestedFix,
  updateFixStatus,
} from "./fixes.js";

// ============================================================================
// Heals (Legacy Protocol)
// ============================================================================

export {
  getHealsForError,
  getLatestHealForError,
  getPendingHealByFileHash,
  recordHeal,
  recordHealVerification,
  updateHealStatus,
} from "./heals.js";

// ============================================================================
// Spend Tracking (Separate Database)
// ============================================================================

export {
  closeSpendDb,
  getDetentDir as getSpendDetentDir,
  getMonthlySpend,
  getSpendDb,
  recordSpend,
} from "./spend.js";

// ============================================================================
// Heal Locks
// ============================================================================

export type { HealLockInfo } from "./locks.js";
export {
  acquireHealLock,
  cleanExpiredLocks,
  generateHolderId,
  HealLockHeldError,
  isHealLockHeld,
  releaseHealLock,
} from "./locks.js";

// ============================================================================
// Recorder
// ============================================================================

export { Recorder } from "./recorder.js";

// ============================================================================
// Config
// ============================================================================

export {
  formatBudget,
  getAllowedCommands,
  getConfigPath,
  getDetentDir as getConfigDetentDir,
  isTrustedRepo,
  loadConfig,
  loadGlobalConfig,
  maskApiKey,
  matchesCommand,
  saveConfig,
  trustRepo,
} from "./config.js";

// ============================================================================
// Validation
// ============================================================================

export {
  ErrEmptyRequired,
  ErrFieldTooLong,
  ErrIDTooLong,
  ErrInvalidConfidence,
  ErrInvalidID,
  ErrInvalidPath,
  ErrInvalidStatus,
  ErrPathTraversal,
  MaxDiffLength,
  MaxExplanationLength,
  // Constants
  MaxIDLength,
  MaxOutputLength,
  MaxPathLength,
  MaxReasonLength,
  // Error classes
  ValidationError,
  validateAssignment,
  validateAssignmentStatus,
  validateConfidence,
  validateFilePaths,
  validateFixStatus,
  // Validation functions
  validateID,
  validateOptionalID,
  validatePath,
  validateStringLength,
  validateSuggestedFix,
} from "./validation.js";
