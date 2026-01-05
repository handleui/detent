/**
 * Error CRUD operations for Detent persistence
 * Includes content-hash deduplication and batch processing
 * Ported from Go: apps/go-cli/internal/persistence/sqlite.go
 */

import { createHash, randomUUID } from "node:crypto";
import type Database from "better-sqlite3";
import type { ErrorRecord, FindingRecord } from "./types.js";

// ============================================================================
// Constants
// ============================================================================

const BATCH_SIZE = 500; // Batch size for error inserts (matches Go)

// ============================================================================
// Content Hash Computation
// ============================================================================

/**
 * Compute content hash for error deduplication.
 * Normalizes the message and computes SHA256 hash.
 * Removes :line:col patterns to group errors across line number changes.
 */
export const computeContentHash = (message: string): string => {
  // Normalize: trim whitespace, lowercase, remove line:col numbers
  let normalized = message.trim().toLowerCase();
  normalized = normalized.replace(/:\d+:\d+/g, "");
  return createHash("sha256").update(normalized).digest("hex");
};

/**
 * Generate a unique error ID
 */
const generateErrorId = (): string => {
  return randomUUID();
};

// ============================================================================
// Error CRUD Operations
// ============================================================================

/**
 * Record a single error in the database with deduplication.
 * If an error with the same content hash exists, updates seen_count.
 */
export const recordError = (
  db: Database.Database,
  finding: FindingRecord,
  runId: string
): void => {
  const contentHash =
    finding.contentHash ?? computeContentHash(finding.message);
  const now = Math.floor(Date.now() / 1000);

  // Check if error already exists
  const existingStmt = db.prepare(
    "SELECT error_id FROM errors WHERE content_hash = ?"
  );
  const existing = existingStmt.get(contentHash) as
    | { error_id: string }
    | undefined;

  const transaction = db.transaction(() => {
    let errorId: string;

    if (existing) {
      // Update existing error
      errorId = existing.error_id;
      const updateStmt = db.prepare(`
        UPDATE errors
        SET seen_count = seen_count + 1, last_seen_at = ?
        WHERE error_id = ?
      `);
      updateStmt.run(now, errorId);
    } else {
      // Insert new error
      errorId = generateErrorId();
      const insertStmt = db.prepare(`
        INSERT INTO errors (
          error_id, run_id, file_path, line_number, column_number,
          error_type, message, stack_trace, file_hash, content_hash,
          severity, rule_id, source, workflow_job, raw,
          first_seen_at, last_seen_at, seen_count, status
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'open')
      `);
      insertStmt.run(
        errorId,
        runId,
        finding.filePath,
        finding.line,
        finding.column,
        finding.category,
        finding.message,
        finding.stackTrace ?? null,
        finding.fileHash ?? null,
        contentHash,
        finding.severity,
        finding.ruleId ?? null,
        finding.source,
        finding.workflowJob ?? null,
        finding.raw ?? null,
        now,
        now
      );
    }

    // Link error to current run in junction table
    const runErrorStmt = db.prepare(
      "INSERT OR IGNORE INTO run_errors (run_id, error_id) VALUES (?, ?)"
    );
    runErrorStmt.run(runId, errorId);
  });

  transaction();
};

/**
 * Record multiple findings in a batch operation with deduplication.
 * Processes in chunks of BATCH_SIZE for optimal performance.
 */
export const recordFindings = (
  db: Database.Database,
  findings: FindingRecord[],
  runId: string
): void => {
  if (findings.length === 0) {
    return;
  }

  // Process in chunks
  for (let i = 0; i < findings.length; i += BATCH_SIZE) {
    const chunk = findings.slice(i, i + BATCH_SIZE);
    recordFindingsChunk(db, chunk, runId);
  }
};

/**
 * Record a chunk of findings in a single transaction
 */
const recordFindingsChunk = (
  db: Database.Database,
  findings: FindingRecord[],
  runId: string
): void => {
  const now = Math.floor(Date.now() / 1000);

  // Pre-compute content hashes
  const findingHashes = new Map<FindingRecord, string>();
  const uniqueHashes: string[] = [];
  const seenHashes = new Set<string>();

  for (const finding of findings) {
    const contentHash =
      finding.contentHash ?? computeContentHash(finding.message);
    findingHashes.set(finding, contentHash);
    if (!seenHashes.has(contentHash)) {
      uniqueHashes.push(contentHash);
      seenHashes.add(contentHash);
    }
  }

  const transaction = db.transaction(() => {
    // Batch lookup existing errors
    const existingErrors = new Map<string, string>(); // content_hash -> error_id

    if (uniqueHashes.length > 0) {
      // Build query with placeholders for IN clause
      const placeholders = uniqueHashes.map(() => "?").join(",");
      const query = `SELECT content_hash, error_id FROM errors WHERE content_hash IN (${placeholders})`;
      const stmt = db.prepare(query);
      const rows = stmt.all(...uniqueHashes) as Array<{
        content_hash: string;
        error_id: string;
      }>;

      for (const row of rows) {
        existingErrors.set(row.content_hash, row.error_id);
      }
    }

    // Prepare statements
    const insertStmt = db.prepare(`
      INSERT INTO errors (
        error_id, run_id, file_path, line_number, column_number,
        error_type, message, stack_trace, file_hash, content_hash,
        severity, rule_id, source, workflow_job, raw,
        first_seen_at, last_seen_at, seen_count, status
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'open')
    `);

    const updateStmt = db.prepare(`
      UPDATE errors
      SET seen_count = seen_count + 1, last_seen_at = ?
      WHERE error_id = ?
    `);

    const runErrorStmt = db.prepare(
      "INSERT OR IGNORE INTO run_errors (run_id, error_id) VALUES (?, ?)"
    );

    // Process all findings
    for (const finding of findings) {
      const contentHash = findingHashes.get(finding);
      if (!contentHash) {
        continue;
      }
      let errorId: string;

      const existingId = existingErrors.get(contentHash);
      if (existingId) {
        // Update existing error
        errorId = existingId;
        updateStmt.run(now, existingId);
      } else {
        // Insert new error
        errorId = generateErrorId();
        insertStmt.run(
          errorId,
          runId,
          finding.filePath,
          finding.line,
          finding.column,
          finding.category,
          finding.message,
          finding.stackTrace ?? null,
          finding.fileHash ?? null,
          contentHash,
          finding.severity,
          finding.ruleId ?? null,
          finding.source,
          finding.workflowJob ?? null,
          finding.raw ?? null,
          now,
          now
        );
        // Add to map for subsequent duplicates in same batch
        existingErrors.set(contentHash, errorId);
      }

      // Link error to current run in junction table
      runErrorStmt.run(runId, errorId);
    }
  });

  transaction();
};

/**
 * Get all errors for a given run via the run_errors junction table.
 * Returns ALL errors that appeared in the run, including deduplicated ones.
 */
export const getErrorsByRunId = (
  db: Database.Database,
  runId: string
): ErrorRecord[] => {
  const stmt = db.prepare(`
    SELECT e.error_id, e.run_id, e.file_path, e.line_number, e.column_number,
           e.error_type, e.message, e.stack_trace, e.file_hash, e.content_hash,
           e.severity, e.rule_id, e.source, e.workflow_job, e.raw,
           e.first_seen_at, e.last_seen_at, e.seen_count, e.status
    FROM errors e
    INNER JOIN run_errors re ON e.error_id = re.error_id
    WHERE re.run_id = ?
    ORDER BY e.file_path, e.line_number
  `);

  const rows = stmt.all(runId) as Array<{
    error_id: string;
    run_id: string;
    file_path: string | null;
    line_number: number | null;
    column_number: number | null;
    error_type: string | null;
    message: string;
    stack_trace: string | null;
    file_hash: string | null;
    content_hash: string | null;
    severity: string | null;
    rule_id: string | null;
    source: string | null;
    workflow_job: string | null;
    raw: string | null;
    first_seen_at: number | null;
    last_seen_at: number | null;
    seen_count: number;
    status: string | null;
  }>;

  return rows.map((row) => ({
    errorId: row.error_id,
    runId: row.run_id,
    filePath: row.file_path ?? "",
    lineNumber: row.line_number ?? 0,
    columnNumber: row.column_number ?? undefined,
    errorType: row.error_type ?? "",
    message: row.message,
    stackTrace: row.stack_trace ?? undefined,
    fileHash: row.file_hash ?? undefined,
    contentHash: row.content_hash ?? "",
    severity: row.severity ?? undefined,
    ruleId: row.rule_id ?? undefined,
    source: row.source ?? undefined,
    workflowJob: row.workflow_job ?? undefined,
    raw: row.raw ?? undefined,
    firstSeenAt: row.first_seen_at
      ? new Date(row.first_seen_at * 1000)
      : new Date(),
    lastSeenAt: row.last_seen_at
      ? new Date(row.last_seen_at * 1000)
      : new Date(),
    seenCount: row.seen_count,
    status: (row.status as ErrorRecord["status"]) ?? "open",
  }));
};

/**
 * Get an error by its ID
 */
export const getErrorById = (
  db: Database.Database,
  errorId: string
): ErrorRecord | null => {
  const stmt = db.prepare(`
    SELECT error_id, run_id, file_path, line_number, column_number,
           error_type, message, stack_trace, file_hash, content_hash,
           severity, rule_id, source, workflow_job, raw,
           first_seen_at, last_seen_at, seen_count, status
    FROM errors
    WHERE error_id = ?
  `);

  const row = stmt.get(errorId) as
    | {
        error_id: string;
        run_id: string;
        file_path: string | null;
        line_number: number | null;
        column_number: number | null;
        error_type: string | null;
        message: string;
        stack_trace: string | null;
        file_hash: string | null;
        content_hash: string | null;
        severity: string | null;
        rule_id: string | null;
        source: string | null;
        workflow_job: string | null;
        raw: string | null;
        first_seen_at: number | null;
        last_seen_at: number | null;
        seen_count: number;
        status: string | null;
      }
    | undefined;

  if (!row) {
    return null;
  }

  return {
    errorId: row.error_id,
    runId: row.run_id,
    filePath: row.file_path ?? "",
    lineNumber: row.line_number ?? 0,
    columnNumber: row.column_number ?? undefined,
    errorType: row.error_type ?? "",
    message: row.message,
    stackTrace: row.stack_trace ?? undefined,
    fileHash: row.file_hash ?? undefined,
    contentHash: row.content_hash ?? "",
    severity: row.severity ?? undefined,
    ruleId: row.rule_id ?? undefined,
    source: row.source ?? undefined,
    workflowJob: row.workflow_job ?? undefined,
    raw: row.raw ?? undefined,
    firstSeenAt: row.first_seen_at
      ? new Date(row.first_seen_at * 1000)
      : new Date(),
    lastSeenAt: row.last_seen_at
      ? new Date(row.last_seen_at * 1000)
      : new Date(),
    seenCount: row.seen_count,
    status: (row.status as ErrorRecord["status"]) ?? "open",
  };
};

/**
 * Get error count for a run
 */
export const getErrorCountByRunId = (
  db: Database.Database,
  runId: string
): number => {
  const stmt = db.prepare(`
    SELECT COUNT(*) as count
    FROM run_errors
    WHERE run_id = ?
  `);
  const row = stmt.get(runId) as { count: number };
  return row.count;
};

/**
 * Update error status
 */
export const updateErrorStatus = (
  db: Database.Database,
  errorId: string,
  status: ErrorRecord["status"]
): void => {
  const stmt = db.prepare("UPDATE errors SET status = ? WHERE error_id = ?");
  stmt.run(status, errorId);
};

/**
 * Delete orphan errors (errors not linked to any run)
 * This is primarily for garbage collection
 */
export const deleteOrphanErrors = (db: Database.Database): number => {
  const stmt = db.prepare(`
    DELETE FROM errors
    WHERE error_id NOT IN (SELECT DISTINCT error_id FROM run_errors)
  `);
  const result = stmt.run();
  return result.changes;
};
