/**
 * High-level batch recording
 * Ported from Go: apps/go-cli/internal/persistence/recorder.go
 *
 * Provides a high-level interface for recording scan findings to the database.
 * Handles batching, file hash caching, and run lifecycle management.
 */

import { createHash } from "node:crypto";
import { readFileSync, statSync } from "node:fs";
import { isAbsolute, join, normalize, sep } from "node:path";
import type Database from "better-sqlite3";
import type { ExtractedError, FindingRecord } from "./types.js";

// ============================================================================
// Recorder Class
// ============================================================================

/**
 * Recorder persists scan results to SQLite
 */
export class Recorder {
  private readonly db: Database.Database;
  private readonly runId: string;
  private readonly repoRoot: string;
  private readonly fileHashCache: Map<string, string> = new Map();
  private errorCount = 0;

  constructor(db: Database.Database, runId: string, repoRoot: string) {
    this.db = db;
    this.runId = runId;
    this.repoRoot = normalize(repoRoot);
  }

  /**
   * Records multiple findings in a single transaction
   */
  recordFindings(errors: ExtractedError[]): void {
    if (errors.length === 0) {
      return;
    }

    const findings = errors.map((err) => this.buildFindingRecord(err));
    this.insertFindings(findings);
  }

  /**
   * Records a single finding to the database
   */
  recordFinding(error: ExtractedError): void {
    const finding = this.buildFindingRecord(error);
    this.insertFindings([finding]);
  }

  /**
   * Finalizes the run and records the total error count
   */
  finalize(_exitCode: number): void {
    const stmt = this.db.prepare(`
      UPDATE runs
      SET completed_at = ?, total_errors = ?
      WHERE run_id = ?
    `);
    stmt.run(Math.floor(Date.now() / 1000), this.errorCount, this.runId);
  }

  /**
   * Returns the current error count
   */
  getErrorCount(): number {
    return this.errorCount;
  }

  /**
   * Returns the run ID
   */
  getRunId(): string {
    return this.runId;
  }

  // ============================================================================
  // Private Methods
  // ============================================================================

  private buildFindingRecord(err: ExtractedError): FindingRecord {
    return {
      timestamp: new Date(),
      runId: this.runId,
      filePath: err.file,
      fileHash: this.computeFileHash(err.file),
      message: err.message,
      line: err.line,
      column: err.column,
      severity: err.severity === "error" ? "error" : "warning",
      stackTrace: err.stackTrace,
      ruleId: err.ruleId,
      category: err.category,
      source: err.source,
      workflowJob: err.workflowContext?.job,
      workflowStep: err.workflowContext?.step,
      raw: err.raw,
    };
  }

  private insertFindings(findings: FindingRecord[]): void {
    const now = Math.floor(Date.now() / 1000);

    // Compute content hashes
    const findingHashes = new Map<FindingRecord, string>();
    const uniqueHashes = new Set<string>();

    for (const finding of findings) {
      const hash = finding.contentHash || computeContentHash(finding.message);
      findingHashes.set(finding, hash);
      uniqueHashes.add(hash);
    }

    // Batch lookup existing errors
    const existingErrors = new Map<string, string>();
    if (uniqueHashes.size > 0) {
      const placeholders = Array.from(uniqueHashes)
        .map(() => "?")
        .join(",");
      const stmt = this.db.prepare(`
        SELECT content_hash, error_id FROM errors WHERE content_hash IN (${placeholders})
      `);
      const rows = stmt.all(...uniqueHashes) as Array<{
        content_hash: string;
        error_id: string;
      }>;
      for (const row of rows) {
        existingErrors.set(row.content_hash, row.error_id);
      }
    }

    // Insert or update in a transaction
    const insertStmt = this.db.prepare(`
      INSERT INTO errors (
        error_id, run_id, file_path, line_number, column_number,
        error_type, message, stack_trace, file_hash, content_hash,
        severity, rule_id, source, workflow_job, raw,
        first_seen_at, last_seen_at, seen_count, status
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'open')
    `);

    const updateStmt = this.db.prepare(`
      UPDATE errors SET seen_count = seen_count + 1, last_seen_at = ? WHERE error_id = ?
    `);

    const runErrorStmt = this.db.prepare(`
      INSERT OR IGNORE INTO run_errors (run_id, error_id) VALUES (?, ?)
    `);

    const transaction = this.db.transaction(() => {
      for (const finding of findings) {
        const contentHash = findingHashes.get(finding) ?? "";
        let errorId: string;

        const existingId = existingErrors.get(contentHash);
        if (existingId) {
          errorId = existingId;
          updateStmt.run(now, existingId);
        } else {
          errorId = generateUuid();
          insertStmt.run(
            errorId,
            finding.runId,
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
          this.errorCount++;
          existingErrors.set(contentHash, errorId);
        }

        // Link error to current run
        runErrorStmt.run(finding.runId, errorId);
      }
    });

    transaction();
  }

  private computeFileHash(filePath: string): string | undefined {
    if (!filePath) {
      return undefined;
    }

    let absPath = filePath;
    if (!isAbsolute(filePath)) {
      absPath = join(this.repoRoot, filePath);
    }
    absPath = normalize(absPath);

    // Prevent path traversal
    const repoRootClean = normalize(this.repoRoot);
    if (!absPath.startsWith(repoRootClean + sep) && absPath !== repoRootClean) {
      return undefined;
    }

    // Check cache
    const cached = this.fileHashCache.get(absPath);
    if (cached) {
      return cached;
    }

    // Compute hash
    try {
      const stat = statSync(absPath);
      if (!stat.isFile()) {
        return undefined;
      }

      const content = readFileSync(absPath);
      const hash = createHash("sha256").update(content).digest("hex");
      this.fileHashCache.set(absPath, hash);
      return hash;
    } catch {
      return undefined;
    }
  }
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Computes a normalized content hash for deduplication.
 * Normalizes by lowercasing, trimming whitespace, and removing file:line:col patterns.
 */
export const computeContentHash = (message: string): string => {
  // Normalize: lowercase, trim whitespace, remove line:col numbers
  let normalized = message.toLowerCase().trim();
  normalized = normalized.replace(/:\d+:\d+/g, "");

  return createHash("sha256").update(normalized).digest("hex");
};

/**
 * Generates a UUID v4
 */
const generateUuid = (): string => {
  // Use crypto.randomUUID if available (Node 16.7+)
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }

  // Fallback implementation
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    // biome-ignore lint/suspicious/noBitwiseOperators: intentional bitwise for UUID generation
    const r = (Math.random() * 16) | 0;
    // biome-ignore lint/suspicious/noBitwiseOperators: intentional bitwise for UUID generation
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
};
