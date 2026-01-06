/**
 * Global spend tracking (SEPARATE DATABASE)
 * Ported from Go: apps/go-cli/internal/persistence/spend.go
 *
 * Unlike the per-repo database, the spend database is stored at ~/.detent/spend.db
 * and tracks spend globally across all repositories.
 */

import { chmodSync, existsSync, mkdirSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import Database from "better-sqlite3";

// ============================================================================
// Constants
// ============================================================================

const SPEND_DB_FILE = "spend.db";
const DETENT_DIR_NAME = ".detent";
const WINDOWS_DRIVE_PATTERN = /^[A-Za-z]:\\/;

// ============================================================================
// Singleton Pattern
// ============================================================================

let globalSpendDb: Database.Database | null = null;

/**
 * Validates an override path from environment variable.
 * Returns null if path is invalid (contains traversal sequences or is not absolute).
 */
const validateOverridePath = (path: string): string | null => {
  // Reject paths with traversal sequences
  if (path.includes("..")) {
    return null;
  }

  // Reject non-absolute paths
  if (!(path.startsWith("/") || WINDOWS_DRIVE_PATTERN.test(path))) {
    return null;
  }

  return path;
};

/**
 * Gets the detent directory path (~/.detent)
 */
export const getDetentDir = (): string => {
  const override = process.env.DETENT_HOME;
  if (override) {
    const validated = validateOverridePath(override);
    if (validated) {
      return validated;
    }
    // Invalid override path - fall back to default
  }
  return join(homedir(), DETENT_DIR_NAME);
};

/**
 * Gets or creates the global spend database singleton
 */
export const getSpendDb = (): Database.Database => {
  if (globalSpendDb) {
    return globalSpendDb;
  }

  const detentDir = getDetentDir();

  // Ensure detent directory exists
  if (!existsSync(detentDir)) {
    mkdirSync(detentDir, { mode: 0o700, recursive: true });
  }

  const dbPath = join(detentDir, SPEND_DB_FILE);
  const db = new Database(dbPath);

  // Configure for optimal SQLite performance
  db.pragma("journal_mode = WAL");
  db.pragma("synchronous = NORMAL");
  db.pragma("busy_timeout = 5000");

  // Initialize schema
  initSpendSchema(db);

  // Set secure permissions
  try {
    chmodSync(dbPath, 0o600);
  } catch {
    // Ignore permission errors on Windows
  }

  globalSpendDb = db;
  return db;
};

/**
 * Closes the global spend database
 */
export const closeSpendDb = (): void => {
  if (globalSpendDb) {
    globalSpendDb.close();
    globalSpendDb = null;
  }
};

// ============================================================================
// Schema
// ============================================================================

const initSpendSchema = (db: Database.Database): void => {
  db.exec(`
    CREATE TABLE IF NOT EXISTS schema_version (
      version INTEGER PRIMARY KEY,
      applied_at INTEGER NOT NULL
    );

    CREATE TABLE IF NOT EXISTS spend_log (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      cost_usd REAL NOT NULL,
      repo_id TEXT,
      created_at INTEGER NOT NULL
    );

    CREATE INDEX IF NOT EXISTS idx_spend_log_created_at ON spend_log(created_at);
  `);

  // Check/update schema version
  const versionRow = db
    .prepare("SELECT COALESCE(MAX(version), 0) as version FROM schema_version")
    .get() as { version: number };

  if (versionRow.version < 1) {
    db.prepare(
      "INSERT INTO schema_version (version, applied_at) VALUES (?, ?)"
    ).run(1, Math.floor(Date.now() / 1000));
  }
};

// ============================================================================
// Spend Operations
// ============================================================================

/**
 * Records a spend amount to the global spend log.
 * repoId is optional and used for auditing purposes.
 */
export const recordSpend = (costUsd: number, repoId?: string): void => {
  if (costUsd <= 0) {
    return;
  }

  const db = getSpendDb();
  const stmt = db.prepare(
    "INSERT INTO spend_log (cost_usd, repo_id, created_at) VALUES (?, ?, ?)"
  );
  stmt.run(costUsd, repoId ?? null, Math.floor(Date.now() / 1000));
};

/**
 * Returns the total spend for the given month (format: "YYYY-MM").
 * If month is empty, uses the current month.
 */
export const getMonthlySpend = (month?: string): number => {
  const db = getSpendDb();

  // Parse or default to current month
  const targetMonth = month || formatMonth(new Date());
  const [startTs, endTs] = getMonthBounds(targetMonth);

  const result = db
    .prepare(
      "SELECT COALESCE(SUM(cost_usd), 0) as total FROM spend_log WHERE created_at >= ? AND created_at < ?"
    )
    .get(startTs, endTs) as { total: number };

  return result.total;
};

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Formats a date as "YYYY-MM"
 */
const formatMonth = (date: Date): string => {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  return `${year}-${month}`;
};

/**
 * Returns start and end timestamps for a month (format: "YYYY-MM")
 */
const getMonthBounds = (month: string): [number, number] => {
  const [yearStr, monthStr] = month.split("-");
  const year = Number.parseInt(yearStr ?? "", 10);
  const monthNum = Number.parseInt(monthStr ?? "", 10);

  if (Number.isNaN(year) || Number.isNaN(monthNum)) {
    throw new Error(`Invalid month format: ${month}. Expected YYYY-MM`);
  }

  const start = new Date(year, monthNum - 1, 1, 0, 0, 0, 0);
  const end = new Date(year, monthNum, 1, 0, 0, 0, 0); // First of next month

  return [Math.floor(start.getTime() / 1000), Math.floor(end.getTime() / 1000)];
};
