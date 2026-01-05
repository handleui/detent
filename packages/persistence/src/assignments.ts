/**
 * Assignment CRUD operations
 * Ported from Go: apps/go-cli/internal/persistence/sqlite.go
 */

import type Database from "better-sqlite3";
import type { Assignment, AssignmentStatus } from "./types.js";

// ============================================================================
// Assignment CRUD
// ============================================================================

/**
 * Creates a new assignment record in the database
 */
export const createAssignment = (
  db: Database.Database,
  assignment: Assignment
): void => {
  const errorIdsJson = JSON.stringify(assignment.errorIds);

  const stmt = db.prepare(`
    INSERT INTO assignments (
      assignment_id, run_id, agent_id, worktree_path,
      error_count, error_ids_json, status,
      created_at, expires_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
  `);

  stmt.run(
    assignment.assignmentId,
    assignment.runId,
    assignment.agentId,
    assignment.worktreePath ?? null,
    assignment.errorCount,
    errorIdsJson,
    assignment.status,
    Math.floor(assignment.createdAt.getTime() / 1000),
    Math.floor(assignment.expiresAt.getTime() / 1000)
  );
};

/**
 * Retrieves an assignment by its ID
 */
export const getAssignment = (
  db: Database.Database,
  assignmentId: string
): Assignment | null => {
  const stmt = db.prepare(`
    SELECT assignment_id, run_id, agent_id, worktree_path,
           error_count, error_ids_json, status,
           created_at, started_at, completed_at, expires_at,
           fix_id, failure_reason
    FROM assignments WHERE assignment_id = ?
  `);

  const row = stmt.get(assignmentId) as AssignmentRow | undefined;
  if (!row) {
    return null;
  }

  return scanAssignment(row);
};

/**
 * Updates the status of an assignment
 */
export const updateAssignmentStatus = (
  db: Database.Database,
  assignmentId: string,
  status: AssignmentStatus,
  fixId?: string,
  failureReason?: string
): void => {
  const now = Math.floor(Date.now() / 1000);

  let stmt: Database.Statement;

  switch (status) {
    case "in_progress":
      stmt = db.prepare(
        "UPDATE assignments SET status = ?, started_at = ? WHERE assignment_id = ?"
      );
      stmt.run(status, now, assignmentId);
      break;
    case "completed":
      stmt = db.prepare(
        "UPDATE assignments SET status = ?, completed_at = ?, fix_id = ? WHERE assignment_id = ?"
      );
      stmt.run(status, now, fixId ?? null, assignmentId);
      break;
    case "failed":
      stmt = db.prepare(
        "UPDATE assignments SET status = ?, completed_at = ?, failure_reason = ? WHERE assignment_id = ?"
      );
      stmt.run(status, now, failureReason ?? null, assignmentId);
      break;
    default:
      stmt = db.prepare(
        "UPDATE assignments SET status = ? WHERE assignment_id = ?"
      );
      stmt.run(status, assignmentId);
  }
};

/**
 * Lists all assignments for a given run
 */
export const listAssignmentsByRun = (
  db: Database.Database,
  runId: string
): Assignment[] => {
  const stmt = db.prepare(`
    SELECT assignment_id, run_id, agent_id, worktree_path,
           error_count, error_ids_json, status,
           created_at, started_at, completed_at, expires_at,
           fix_id, failure_reason
    FROM assignments WHERE run_id = ?
    ORDER BY created_at DESC
  `);

  const rows = stmt.all(runId) as AssignmentRow[];
  return rows.map(scanAssignment);
};

// ============================================================================
// Internal Types and Helpers
// ============================================================================

interface AssignmentRow {
  assignment_id: string;
  run_id: string;
  agent_id: string;
  worktree_path: string | null;
  error_count: number;
  error_ids_json: string;
  status: string;
  created_at: number;
  started_at: number | null;
  completed_at: number | null;
  expires_at: number;
  fix_id: string | null;
  failure_reason: string | null;
}

const scanAssignment = (row: AssignmentRow): Assignment => {
  const errorIds = JSON.parse(row.error_ids_json) as string[];

  return {
    assignmentId: row.assignment_id,
    runId: row.run_id,
    agentId: row.agent_id,
    worktreePath: row.worktree_path ?? undefined,
    errorCount: row.error_count,
    errorIds,
    status: row.status as AssignmentStatus,
    createdAt: new Date(row.created_at * 1000),
    startedAt: row.started_at ? new Date(row.started_at * 1000) : undefined,
    completedAt: row.completed_at
      ? new Date(row.completed_at * 1000)
      : undefined,
    expiresAt: new Date(row.expires_at * 1000),
    fixId: row.fix_id ?? undefined,
    failureReason: row.failure_reason ?? undefined,
  };
};
