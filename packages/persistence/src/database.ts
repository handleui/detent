/**
 * Database connection management for Detent persistence
 * Uses better-sqlite3 for synchronous SQLite operations
 *
 * Database path: ~/.detent/repos/<repoId>.db
 */

import { createHash } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, resolve } from "node:path";
import Database from "better-sqlite3";
import { initSchema } from "./schema.js";

// ============================================================================
// Constants
// ============================================================================

const FILE_PERMISSIONS = 0o600; // Owner read/write only
const DIR_PERMISSIONS = 0o700; // Owner read/write/execute only
const GIT_SUFFIX_REGEX = /\.git$/;

// ============================================================================
// Errors
// ============================================================================

export class ErrHealLockHeld extends Error {
  constructor(message = "heal lock is held by another process") {
    super(message);
    this.name = "ErrHealLockHeld";
  }
}

// ============================================================================
// Directory and Path Management
// ============================================================================

/**
 * Get the detent directory path: ~/.detent
 */
export const getDetentDir = (): string => {
  return join(homedir(), ".detent");
};

/**
 * Create a directory if it doesn't exist with proper permissions
 */
const createDirIfNotExists = (path: string): void => {
  if (!existsSync(path)) {
    mkdirSync(path, { recursive: true, mode: DIR_PERMISSIONS });
  }
};

/**
 * Compute a stable identifier for a repository.
 * Priority: 1) git remote URL, 2) first commit SHA, 3) repo path
 * Returns a 20-character hex string suitable for directory names.
 */
export const computeRepoId = async (repoRoot: string): Promise<string> => {
  const absPath = resolve(repoRoot);

  // Priority 1: git remote origin URL (works across machines)
  try {
    const remoteUrl = await getGitRemoteUrl(absPath);
    if (remoteUrl) {
      // Normalize: strip .git suffix for consistent IDs
      const normalized = remoteUrl.replace(GIT_SUFFIX_REGEX, "");
      return hashToId(normalized);
    }
  } catch {
    // Fall through to next priority
  }

  // Priority 2: first commit SHA (immutable, works offline)
  try {
    const firstCommit = await getFirstCommitSha(absPath);
    if (firstCommit) {
      return hashToId(firstCommit);
    }
  } catch {
    // Fall through to next priority
  }

  // Priority 3: repo path (last resort - breaks if repo moves)
  return hashToId(absPath);
};

/**
 * Compute SHA256 hash and return first 20 hex characters (80 bits)
 */
const hashToId = (input: string): string => {
  const hash = createHash("sha256").update(input).digest("hex");
  return hash.slice(0, 20);
};

/**
 * Get git remote origin URL
 */
const getGitRemoteUrl = async (repoRoot: string): Promise<string | null> => {
  const { exec } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execAsync = promisify(exec);

  try {
    const { stdout } = await execAsync("git remote get-url origin", {
      cwd: repoRoot,
    });
    return stdout.trim() || null;
  } catch {
    return null;
  }
};

/**
 * Get first commit SHA
 */
const getFirstCommitSha = async (repoRoot: string): Promise<string | null> => {
  const { exec } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execAsync = promisify(exec);

  try {
    const { stdout } = await execAsync(
      "git rev-list --max-parents=0 HEAD | head -1",
      { cwd: repoRoot }
    );
    return stdout.trim() || null;
  } catch {
    return null;
  }
};

/**
 * Get the database path for a given repo
 * Uses consolidated directory: ~/.detent/repos/<repoId>.db
 */
export const getDatabasePath = async (repoRoot: string): Promise<string> => {
  const detentDir = getDetentDir();
  const repoId = await computeRepoId(repoRoot);
  return join(detentDir, "repos", `${repoId}.db`);
};

// ============================================================================
// Database Connection
// ============================================================================

/**
 * Set secure file permissions on the database and related WAL/SHM files
 */
const secureDbFiles = (dbPath: string): void => {
  // Main database file
  if (existsSync(dbPath)) {
    chmodSync(dbPath, FILE_PERMISSIONS);
  }

  // WAL and SHM files (may not exist yet)
  const walFiles = [`${dbPath}-wal`, `${dbPath}-shm`];
  for (const file of walFiles) {
    if (existsSync(file)) {
      chmodSync(file, FILE_PERMISSIONS);
    }
  }
};

/**
 * Create and configure a database connection
 */
export const createDatabase = async (
  repoRoot: string
): Promise<Database.Database> => {
  const dbPath = await getDatabasePath(repoRoot);

  // Create repos directory
  const reposDir = dirname(dbPath);
  createDirIfNotExists(reposDir);

  // Open database connection
  const db = new Database(dbPath);

  // Apply performance pragmas for 2-5x speedup
  db.pragma("journal_mode = WAL"); // Write-Ahead Logging for better concurrency
  db.pragma("synchronous = NORMAL"); // Faster writes, still safe with WAL
  db.pragma("cache_size = -64000"); // 64MB cache for better performance
  db.pragma("busy_timeout = 5000"); // Wait 5s on lock instead of failing immediately
  db.pragma("mmap_size = 268435456"); // 256MB memory-mapped I/O for faster reads
  db.pragma("temp_store = MEMORY"); // Store temp tables in memory
  db.pragma("page_size = 4096"); // Optimal page size for most filesystems

  // Initialize schema (creates tables and applies migrations)
  initSchema(db);

  // Set secure file permissions on database and related files
  secureDbFiles(dbPath);

  return db;
};

/**
 * Create an in-memory database (for testing)
 */
export const createInMemoryDatabase = (): Database.Database => {
  const db = new Database(":memory:");

  // Apply pragmas (some don't apply to in-memory but won't hurt)
  db.pragma("cache_size = -64000");
  db.pragma("temp_store = MEMORY");

  // Initialize schema
  initSchema(db);

  return db;
};

/**
 * Close a database connection
 */
export const closeDatabase = (db: Database.Database): void => {
  db.close();
};

/**
 * Check if a database file exists for a repo
 */
export const databaseExists = async (repoRoot: string): Promise<boolean> => {
  const dbPath = await getDatabasePath(repoRoot);
  return existsSync(dbPath);
};

/**
 * Get database file size in bytes
 */
export const getDatabaseSize = async (
  repoRoot: string
): Promise<number | null> => {
  const dbPath = await getDatabasePath(repoRoot);
  if (!existsSync(dbPath)) {
    return null;
  }
  const stats = statSync(dbPath);
  return stats.size;
};
