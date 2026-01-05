/**
 * Advisory heal locks with TTL
 * Ported from Go: apps/go-cli/internal/persistence/sqlite.go
 *
 * Provides advisory locking to prevent concurrent heal processes on the same repository.
 * Locks automatically expire after a TTL for crash recovery.
 */

import { randomUUID } from "node:crypto";
import type Database from "better-sqlite3";

// ============================================================================
// Types
// ============================================================================

export interface HealLockInfo {
  held: boolean;
  holderId?: string;
  expiresAt?: number;
}

export class HealLockHeldError extends Error {
  readonly holderId: string;
  readonly pid?: number;

  constructor(holderId: string, pid?: number) {
    const pidInfo = pid ? ` (pid: ${pid})` : "";
    const holderPreview = holderId.length > 8 ? holderId.slice(0, 8) : holderId;
    super(`heal lock is held by another process: ${holderPreview}${pidInfo}`);
    this.name = "HealLockHeldError";
    this.holderId = holderId;
    this.pid = pid;
  }
}

// ============================================================================
// Lock Operations
// ============================================================================

/**
 * Attempts to acquire an exclusive heal lock for a repository.
 * Returns true on success, false if the lock is already held.
 *
 * @param db - Database instance
 * @param repoPath - Repository path to lock
 * @param holderId - Unique identifier for this lock holder
 * @param ttlSeconds - Time-to-live in seconds for automatic expiration
 * @returns true if lock was acquired, false if already held
 * @throws HealLockHeldError if lock is held by another process
 */
export const acquireHealLock = (
  db: Database.Database,
  repoPath: string,
  holderId: string,
  ttlSeconds: number
): boolean => {
  if (ttlSeconds <= 0) {
    throw new Error("ttlSeconds must be positive");
  }

  const now = Math.floor(Date.now() / 1000);
  const expiresAt = now + ttlSeconds;
  const pid = process.pid;

  // Use a transaction for atomicity
  const transaction = db.transaction(() => {
    // Clean up only this repo's expired lock
    db.prepare(
      "DELETE FROM heal_locks WHERE repo_path = ? AND expires_at < ?"
    ).run(repoPath, now);

    // Check if lock exists
    const existing = db
      .prepare("SELECT holder_id, pid FROM heal_locks WHERE repo_path = ?")
      .get(repoPath) as { holder_id: string; pid: number | null } | undefined;

    if (existing) {
      throw new HealLockHeldError(
        existing.holder_id,
        existing.pid ?? undefined
      );
    }

    // Try to insert the lock
    db.prepare(
      "INSERT INTO heal_locks (repo_path, holder_id, acquired_at, expires_at, pid) VALUES (?, ?, ?, ?, ?)"
    ).run(repoPath, holderId, now, expiresAt, pid);

    return true;
  });

  try {
    return transaction();
  } catch (error) {
    if (error instanceof HealLockHeldError) {
      throw error;
    }
    // Handle SQLite UNIQUE constraint violation
    const message = error instanceof Error ? error.message : String(error);
    if (
      message.includes("UNIQUE constraint failed") ||
      message.includes("PRIMARY KEY constraint failed")
    ) {
      const existing = db
        .prepare("SELECT holder_id, pid FROM heal_locks WHERE repo_path = ?")
        .get(repoPath) as { holder_id: string; pid: number | null } | undefined;
      throw new HealLockHeldError(
        existing?.holder_id ?? "unknown",
        existing?.pid ?? undefined
      );
    }
    throw error;
  }
};

/**
 * Releases a previously acquired heal lock.
 * Only the holder (matching holderId) can release the lock.
 * This operation is idempotent - releasing a non-existent lock is not an error.
 */
export const releaseHealLock = (
  db: Database.Database,
  repoPath: string,
  holderId: string
): void => {
  db.prepare(
    "DELETE FROM heal_locks WHERE repo_path = ? AND holder_id = ?"
  ).run(repoPath, holderId);
};

/**
 * Checks if a valid (non-expired) heal lock exists for a repository.
 * WARNING: For informational purposes only. Do NOT use to decide whether to acquire
 * a lock - use acquireHealLock directly which handles races atomically.
 */
export const isHealLockHeld = (
  db: Database.Database,
  repoPath: string
): HealLockInfo => {
  const now = Math.floor(Date.now() / 1000);

  const row = db
    .prepare(
      "SELECT holder_id, expires_at FROM heal_locks WHERE repo_path = ? AND expires_at > ?"
    )
    .get(repoPath, now) as
    | { holder_id: string; expires_at: number }
    | undefined;

  if (!row) {
    return { held: false };
  }

  return {
    held: true,
    holderId: row.holder_id,
    expiresAt: row.expires_at,
  };
};

/**
 * Cleans up all expired locks across all repositories.
 * Call this periodically to prevent lock table bloat.
 */
export const cleanExpiredLocks = (db: Database.Database): number => {
  const now = Math.floor(Date.now() / 1000);
  const result = db
    .prepare("DELETE FROM heal_locks WHERE expires_at < ?")
    .run(now);
  return result.changes;
};

/**
 * Generates a unique holder ID for lock acquisition
 */
export const generateHolderId = (): string => {
  return randomUUID();
};
