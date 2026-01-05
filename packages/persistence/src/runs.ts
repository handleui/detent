/**
 * Run CRUD operations for Detent persistence
 * Ported from Go: apps/go-cli/internal/persistence/sqlite.go
 */

import type Database from "better-sqlite3";
import type { Run } from "./types.js";

// ============================================================================
// Run CRUD Operations
// ============================================================================

/**
 * Record a new run in the database
 */
export const recordRun = (db: Database.Database, run: Run): void => {
  const stmt = db.prepare(`
    INSERT INTO runs (run_id, workflow_name, commit_sha, tree_hash, execution_mode, started_at, is_dirty)
    VALUES (?, ?, ?, ?, ?, ?, 0)
  `);

  stmt.run(
    run.runId,
    run.workflowName,
    run.commitSha,
    run.treeHash ?? null,
    run.executionMode,
    Math.floor(run.startedAt.getTime() / 1000)
  );
};

/**
 * Get a run by its ID
 */
export const getRunById = (
  db: Database.Database,
  runId: string
): Run | null => {
  const stmt = db.prepare(`
    SELECT run_id, workflow_name, commit_sha, tree_hash, execution_mode,
           started_at, completed_at, total_errors, sync_status
    FROM runs WHERE run_id = ?
  `);

  const row = stmt.get(runId) as
    | {
        run_id: string;
        workflow_name: string;
        commit_sha: string;
        tree_hash: string | null;
        execution_mode: string;
        started_at: number | null;
        completed_at: number | null;
        total_errors: number | null;
        sync_status: string | null;
      }
    | undefined;

  if (!row) {
    return null;
  }

  return {
    runId: row.run_id,
    workflowName: row.workflow_name,
    commitSha: row.commit_sha,
    treeHash: row.tree_hash ?? undefined,
    executionMode: row.execution_mode,
    startedAt: row.started_at ? new Date(row.started_at * 1000) : new Date(),
    completedAt: row.completed_at
      ? new Date(row.completed_at * 1000)
      : undefined,
    totalErrors: row.total_errors ?? undefined,
    syncStatus: (row.sync_status as Run["syncStatus"]) ?? undefined,
  };
};

/**
 * Get a run by its commit SHA (for deduplication)
 * Returns the most recent run for the given commit
 */
export const getRunByCommit = (
  db: Database.Database,
  commitSha: string
): Run | null => {
  const stmt = db.prepare(`
    SELECT run_id, workflow_name, commit_sha, tree_hash, execution_mode,
           started_at, completed_at, total_errors, sync_status
    FROM runs
    WHERE commit_sha = ?
    ORDER BY started_at DESC
    LIMIT 1
  `);

  const row = stmt.get(commitSha) as
    | {
        run_id: string;
        workflow_name: string;
        commit_sha: string;
        tree_hash: string | null;
        execution_mode: string;
        started_at: number | null;
        completed_at: number | null;
        total_errors: number | null;
        sync_status: string | null;
      }
    | undefined;

  if (!row) {
    return null;
  }

  return {
    runId: row.run_id,
    workflowName: row.workflow_name,
    commitSha: row.commit_sha,
    treeHash: row.tree_hash ?? undefined,
    executionMode: row.execution_mode,
    startedAt: row.started_at ? new Date(row.started_at * 1000) : new Date(),
    completedAt: row.completed_at
      ? new Date(row.completed_at * 1000)
      : undefined,
    totalErrors: row.total_errors ?? undefined,
    syncStatus: (row.sync_status as Run["syncStatus"]) ?? undefined,
  };
};

/**
 * Check if a run with the given ID exists in the database
 * More efficient than getRunById when you only need to check existence
 */
export const runExists = (db: Database.Database, runId: string): boolean => {
  const stmt = db.prepare("SELECT 1 FROM runs WHERE run_id = ? LIMIT 1");
  const row = stmt.get(runId);
  return row !== undefined;
};

/**
 * Finalize a run with completion information
 * Note: Caller should ensure errors are flushed before calling this
 */
export const finalizeRun = (
  db: Database.Database,
  runId: string,
  _exitCode: number,
  totalErrors: number
): void => {
  const stmt = db.prepare(`
    UPDATE runs
    SET completed_at = ?, total_errors = ?
    WHERE run_id = ?
  `);

  stmt.run(Math.floor(Date.now() / 1000), totalErrors, runId);
};

/**
 * Get all runs, ordered by most recent first
 */
export const getAllRuns = (db: Database.Database, limit = 100): Run[] => {
  const stmt = db.prepare(`
    SELECT run_id, workflow_name, commit_sha, tree_hash, execution_mode,
           started_at, completed_at, total_errors, sync_status
    FROM runs
    ORDER BY started_at DESC
    LIMIT ?
  `);

  const rows = stmt.all(limit) as Array<{
    run_id: string;
    workflow_name: string;
    commit_sha: string;
    tree_hash: string | null;
    execution_mode: string;
    started_at: number | null;
    completed_at: number | null;
    total_errors: number | null;
    sync_status: string | null;
  }>;

  return rows.map((row) => ({
    runId: row.run_id,
    workflowName: row.workflow_name,
    commitSha: row.commit_sha,
    treeHash: row.tree_hash ?? undefined,
    executionMode: row.execution_mode,
    startedAt: row.started_at ? new Date(row.started_at * 1000) : new Date(),
    completedAt: row.completed_at
      ? new Date(row.completed_at * 1000)
      : undefined,
    totalErrors: row.total_errors ?? undefined,
    syncStatus: (row.sync_status as Run["syncStatus"]) ?? undefined,
  }));
};

/**
 * Delete a run and its associated data
 * This is primarily for garbage collection
 */
export const deleteRun = (db: Database.Database, runId: string): void => {
  const deleteRunErrors = db.prepare("DELETE FROM run_errors WHERE run_id = ?");
  const deleteRun = db.prepare("DELETE FROM runs WHERE run_id = ?");

  const transaction = db.transaction(() => {
    deleteRunErrors.run(runId);
    deleteRun.run(runId);
  });

  transaction();
};
