/**
 * SuggestedFix CRUD operations with content-addressed IDs
 * Ported from Go: apps/go-cli/internal/persistence/sqlite.go + suggested_fix.go
 */

import { createHash } from "node:crypto";
import type Database from "better-sqlite3";
import type { FileChange, FixStatus, SuggestedFix } from "./types.js";

// ============================================================================
// Content-Addressed Fix ID
// ============================================================================

/**
 * Computes a content-addressed fix ID from file changes.
 * The ID is deterministic: same changes = same ID, enabling deduplication.
 * Returns first 20 hex chars (80 bits - sufficient for uniqueness).
 * Returns empty string for empty maps (no changes = no ID).
 */
export const computeFixId = (
  fileChanges: Record<string, FileChange>
): string => {
  const paths = Object.keys(fileChanges);
  if (paths.length === 0) {
    return "";
  }

  const hash = createHash("sha256");

  // Sort file paths for deterministic ordering
  paths.sort();

  for (const path of paths) {
    const change = fileChanges[path];
    if (!change) {
      continue;
    }

    hash.update(path);
    hash.update("\0"); // null separator

    // Use unified diff if available, else use after content
    const content = change.unifiedDiff ?? change.afterContent ?? "";
    hash.update(content);
    hash.update("\0");
  }

  return hash.digest("hex").slice(0, 20);
};

// ============================================================================
// SuggestedFix CRUD
// ============================================================================

/**
 * Stores a suggested fix in the database.
 * Uses INSERT OR REPLACE for idempotent upserts (same fix proposed again = update).
 */
export const storeSuggestedFix = (
  db: Database.Database,
  fix: SuggestedFix
): void => {
  // Compute fix ID if not set
  const fixId = fix.fixId || computeFixId(fix.fileChanges);
  if (!fixId) {
    throw new Error("invalid suggested fix: file_changes cannot be empty");
  }

  const fileChangesJson = JSON.stringify(fix.fileChanges);

  const stmt = db.prepare(`
    INSERT OR REPLACE INTO suggested_fixes (
      fix_id, assignment_id, agent_id, worktree_path,
      file_changes_json, explanation, confidence,
      verification_command, verification_exit_code, verification_output, verification_duration_ms,
      errors_before, errors_after,
      model_id, input_tokens, output_tokens, cost_usd,
      status, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `);

  stmt.run(
    fixId,
    fix.assignmentId,
    fix.agentId ?? null,
    fix.worktreePath ?? null,
    fileChangesJson,
    fix.explanation ?? null,
    fix.confidence,
    fix.verification.command,
    fix.verification.exitCode,
    fix.verification.output ?? null,
    fix.verification.durationMs,
    fix.verification.errorsBefore,
    fix.verification.errorsAfter,
    fix.modelId ?? null,
    fix.inputTokens,
    fix.outputTokens,
    fix.costUsd,
    fix.status,
    Math.floor(fix.createdAt.getTime() / 1000)
  );

  // Insert error associations
  if (fix.errorIds && fix.errorIds.length > 0) {
    const insertErrorStmt = db.prepare(
      "INSERT OR IGNORE INTO fix_errors (fix_id, error_id) VALUES (?, ?)"
    );
    for (const errorId of fix.errorIds) {
      insertErrorStmt.run(fixId, errorId);
    }
  }
};

/**
 * Retrieves a suggested fix by its ID
 */
export const getSuggestedFix = (
  db: Database.Database,
  fixId: string
): SuggestedFix | null => {
  const stmt = db.prepare(`
    SELECT fix_id, assignment_id, agent_id, worktree_path,
           file_changes_json, explanation, confidence,
           verification_command, verification_exit_code, verification_output, verification_duration_ms,
           errors_before, errors_after,
           model_id, input_tokens, output_tokens, cost_usd,
           status, created_at, applied_at, applied_by, applied_commit_sha,
           rejected_at, rejected_by, rejection_reason
    FROM suggested_fixes WHERE fix_id = ?
  `);

  const row = stmt.get(fixId) as SuggestedFixRow | undefined;
  if (!row) {
    return null;
  }

  const fix = scanSuggestedFix(row);

  // Load associated error IDs
  fix.errorIds = getFixErrorIds(db, fixId);

  return fix;
};

/**
 * Lists all pending suggested fixes, optionally filtered by run
 */
export const listPendingFixes = (
  db: Database.Database,
  runId?: string
): SuggestedFix[] => {
  let rows: SuggestedFixRow[];

  if (runId) {
    const stmt = db.prepare(`
      SELECT sf.fix_id, sf.assignment_id, sf.agent_id, sf.worktree_path,
             sf.file_changes_json, sf.explanation, sf.confidence,
             sf.verification_command, sf.verification_exit_code, sf.verification_output, sf.verification_duration_ms,
             sf.errors_before, sf.errors_after,
             sf.model_id, sf.input_tokens, sf.output_tokens, sf.cost_usd,
             sf.status, sf.created_at, sf.applied_at, sf.applied_by, sf.applied_commit_sha,
             sf.rejected_at, sf.rejected_by, sf.rejection_reason
      FROM suggested_fixes sf
      INNER JOIN assignments a ON sf.assignment_id = a.assignment_id
      WHERE sf.status = ? AND a.run_id = ?
      ORDER BY sf.created_at DESC
    `);
    rows = stmt.all("pending", runId) as SuggestedFixRow[];
  } else {
    const stmt = db.prepare(`
      SELECT fix_id, assignment_id, agent_id, worktree_path,
             file_changes_json, explanation, confidence,
             verification_command, verification_exit_code, verification_output, verification_duration_ms,
             errors_before, errors_after,
             model_id, input_tokens, output_tokens, cost_usd,
             status, created_at, applied_at, applied_by, applied_commit_sha,
             rejected_at, rejected_by, rejection_reason
      FROM suggested_fixes
      WHERE status = ?
      ORDER BY created_at DESC
    `);
    rows = stmt.all("pending") as SuggestedFixRow[];
  }

  const fixes = rows.map(scanSuggestedFix);

  // Load associated error IDs for each fix
  for (const fix of fixes) {
    fix.errorIds = getFixErrorIds(db, fix.fixId);
  }

  return fixes;
};

/**
 * Updates the status of a suggested fix
 */
export const updateFixStatus = (
  db: Database.Database,
  fixId: string,
  status: FixStatus,
  appliedBy?: string,
  commitSha?: string,
  rejectedBy?: string,
  rejectionReason?: string
): void => {
  const now = Math.floor(Date.now() / 1000);

  let stmt: Database.Statement;

  switch (status) {
    case "applied":
      stmt = db.prepare(
        "UPDATE suggested_fixes SET status = ?, applied_at = ?, applied_by = ?, applied_commit_sha = ? WHERE fix_id = ?"
      );
      stmt.run(status, now, appliedBy ?? null, commitSha ?? null, fixId);
      break;
    case "rejected":
      stmt = db.prepare(
        "UPDATE suggested_fixes SET status = ?, rejected_at = ?, rejected_by = ?, rejection_reason = ? WHERE fix_id = ?"
      );
      stmt.run(status, now, rejectedBy ?? null, rejectionReason ?? null, fixId);
      break;
    default:
      stmt = db.prepare(
        "UPDATE suggested_fixes SET status = ? WHERE fix_id = ?"
      );
      stmt.run(status, fixId);
  }
};

// ============================================================================
// Internal Types and Helpers
// ============================================================================

interface SuggestedFixRow {
  fix_id: string;
  assignment_id: string;
  agent_id: string | null;
  worktree_path: string | null;
  file_changes_json: string;
  explanation: string | null;
  confidence: number;
  verification_command: string;
  verification_exit_code: number;
  verification_output: string | null;
  verification_duration_ms: number | null;
  errors_before: number;
  errors_after: number;
  model_id: string | null;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  status: string;
  created_at: number;
  applied_at: number | null;
  applied_by: string | null;
  applied_commit_sha: string | null;
  rejected_at: number | null;
  rejected_by: string | null;
  rejection_reason: string | null;
}

const scanSuggestedFix = (row: SuggestedFixRow): SuggestedFix => {
  const fileChanges = JSON.parse(row.file_changes_json) as Record<
    string,
    FileChange
  >;

  return {
    fixId: row.fix_id,
    assignmentId: row.assignment_id,
    agentId: row.agent_id ?? undefined,
    worktreePath: row.worktree_path ?? undefined,
    fileChanges,
    explanation: row.explanation ?? undefined,
    confidence: row.confidence,
    verification: {
      command: row.verification_command,
      exitCode: row.verification_exit_code,
      output: row.verification_output ?? undefined,
      durationMs: row.verification_duration_ms ?? 0,
      errorsBefore: row.errors_before,
      errorsAfter: row.errors_after,
    },
    modelId: row.model_id ?? undefined,
    inputTokens: row.input_tokens,
    outputTokens: row.output_tokens,
    costUsd: row.cost_usd,
    status: row.status as FixStatus,
    createdAt: new Date(row.created_at * 1000),
    appliedAt: row.applied_at ? new Date(row.applied_at * 1000) : undefined,
    appliedBy: row.applied_by ?? undefined,
    appliedCommitSha: row.applied_commit_sha ?? undefined,
    rejectedAt: row.rejected_at ? new Date(row.rejected_at * 1000) : undefined,
    rejectedBy: row.rejected_by ?? undefined,
    rejectionReason: row.rejection_reason ?? undefined,
  };
};

const getFixErrorIds = (db: Database.Database, fixId: string): string[] => {
  const stmt = db.prepare("SELECT error_id FROM fix_errors WHERE fix_id = ?");
  const rows = stmt.all(fixId) as Array<{ error_id: string }>;
  return rows.map((row) => row.error_id);
};
