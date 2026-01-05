/**
 * Legacy heal protocol operations
 * Ported from Go: apps/go-cli/internal/persistence/sqlite.go
 */

import type Database from "better-sqlite3";
import type { HealRecord, HealStatus, VerificationResult } from "./types.js";

// ============================================================================
// Heal CRUD
// ============================================================================

/**
 * Records a new heal in the database
 */
export const recordHeal = (db: Database.Database, heal: HealRecord): void => {
  const stmt = db.prepare(`
    INSERT INTO heals (
      heal_id, error_id, run_id, diff_content, diff_content_hash, file_path, file_hash,
      model_id, prompt_hash, input_tokens, output_tokens,
      cache_read_tokens, cache_write_tokens, cost_usd,
      status, created_at, applied_at, verified_at, verification_result,
      attempt_number, parent_heal_id, failure_reason
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `);

  stmt.run(
    heal.healId,
    heal.errorId,
    heal.runId ?? null,
    heal.diffContent,
    heal.diffContentHash ?? null,
    heal.filePath ?? null,
    heal.fileHash ?? null,
    heal.modelId ?? null,
    heal.promptHash ?? null,
    heal.inputTokens,
    heal.outputTokens,
    heal.cacheReadTokens,
    heal.cacheWriteTokens,
    heal.costUsd,
    heal.status,
    Math.floor(heal.createdAt.getTime() / 1000),
    heal.appliedAt ? Math.floor(heal.appliedAt.getTime() / 1000) : null,
    heal.verifiedAt ? Math.floor(heal.verifiedAt.getTime() / 1000) : null,
    heal.verificationResult ?? null,
    heal.attemptNumber,
    heal.parentHealId ?? null,
    heal.failureReason ?? null
  );
};

/**
 * Updates the status of a heal
 */
export const updateHealStatus = (
  db: Database.Database,
  healId: string,
  status: HealStatus,
  appliedAt?: Date
): void => {
  let stmt: Database.Statement;

  if (appliedAt) {
    stmt = db.prepare(
      "UPDATE heals SET status = ?, applied_at = ? WHERE heal_id = ?"
    );
    stmt.run(status, Math.floor(appliedAt.getTime() / 1000), healId);
  } else {
    stmt = db.prepare("UPDATE heals SET status = ? WHERE heal_id = ?");
    stmt.run(status, healId);
  }
};

/**
 * Records the verification result for a heal
 */
export const recordHealVerification = (
  db: Database.Database,
  healId: string,
  result: VerificationResult
): void => {
  const stmt = db.prepare(
    "UPDATE heals SET verified_at = ?, verification_result = ? WHERE heal_id = ?"
  );
  stmt.run(Math.floor(Date.now() / 1000), result, healId);
};

/**
 * Retrieves all heals for a given error, ordered by attempt number
 */
export const getHealsForError = (
  db: Database.Database,
  errorId: string
): HealRecord[] => {
  const stmt = db.prepare(`
    SELECT heal_id, error_id, run_id, diff_content, diff_content_hash, file_path, file_hash,
           model_id, prompt_hash, input_tokens, output_tokens,
           cache_read_tokens, cache_write_tokens, cost_usd,
           status, created_at, applied_at, verified_at, verification_result,
           attempt_number, parent_heal_id, failure_reason
    FROM heals WHERE error_id = ?
    ORDER BY attempt_number ASC
  `);

  const rows = stmt.all(errorId) as HealRow[];
  return rows.map(scanHeal);
};

/**
 * Retrieves the most recent heal for a given error
 */
export const getLatestHealForError = (
  db: Database.Database,
  errorId: string
): HealRecord | null => {
  const stmt = db.prepare(`
    SELECT heal_id, error_id, run_id, diff_content, diff_content_hash, file_path, file_hash,
           model_id, prompt_hash, input_tokens, output_tokens,
           cache_read_tokens, cache_write_tokens, cost_usd,
           status, created_at, applied_at, verified_at, verification_result,
           attempt_number, parent_heal_id, failure_reason
    FROM heals WHERE error_id = ?
    ORDER BY attempt_number DESC LIMIT 1
  `);

  const row = stmt.get(errorId) as HealRow | undefined;
  if (!row) {
    return null;
  }

  return scanHeal(row);
};

/**
 * Finds a reusable pending heal for the given file and hash
 */
export const getPendingHealByFileHash = (
  db: Database.Database,
  filePath: string,
  fileHash: string
): HealRecord | null => {
  const stmt = db.prepare(`
    SELECT heal_id, error_id, run_id, diff_content, diff_content_hash, file_path, file_hash,
           model_id, prompt_hash, input_tokens, output_tokens,
           cache_read_tokens, cache_write_tokens, cost_usd,
           status, created_at, applied_at, verified_at, verification_result,
           attempt_number, parent_heal_id, failure_reason
    FROM heals
    WHERE file_path = ? AND file_hash = ? AND status = 'pending'
    ORDER BY created_at DESC LIMIT 1
  `);

  const row = stmt.get(filePath, fileHash) as HealRow | undefined;
  if (!row) {
    return null;
  }

  return scanHeal(row);
};

// ============================================================================
// Internal Types and Helpers
// ============================================================================

interface HealRow {
  heal_id: string;
  error_id: string;
  run_id: string | null;
  diff_content: string;
  diff_content_hash: string | null;
  file_path: string | null;
  file_hash: string | null;
  model_id: string | null;
  prompt_hash: string | null;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  cost_usd: number;
  status: string;
  created_at: number;
  applied_at: number | null;
  verified_at: number | null;
  verification_result: string | null;
  attempt_number: number;
  parent_heal_id: string | null;
  failure_reason: string | null;
}

const scanHeal = (row: HealRow): HealRecord => ({
  healId: row.heal_id,
  errorId: row.error_id,
  runId: row.run_id ?? undefined,
  diffContent: row.diff_content,
  diffContentHash: row.diff_content_hash ?? undefined,
  filePath: row.file_path ?? undefined,
  fileHash: row.file_hash ?? undefined,
  modelId: row.model_id ?? undefined,
  promptHash: row.prompt_hash ?? undefined,
  inputTokens: row.input_tokens,
  outputTokens: row.output_tokens,
  cacheReadTokens: row.cache_read_tokens,
  cacheWriteTokens: row.cache_write_tokens,
  costUsd: row.cost_usd,
  status: row.status as HealStatus,
  createdAt: new Date(row.created_at * 1000),
  appliedAt: row.applied_at ? new Date(row.applied_at * 1000) : undefined,
  verifiedAt: row.verified_at ? new Date(row.verified_at * 1000) : undefined,
  verificationResult: row.verification_result as VerificationResult | undefined,
  attemptNumber: row.attempt_number,
  parentHealId: row.parent_heal_id ?? undefined,
  failureReason: row.failure_reason ?? undefined,
});
