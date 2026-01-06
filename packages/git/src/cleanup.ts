import { lstatSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { checkLockStatus } from "./lock.js";

const ORPHAN_AGE_THRESHOLD = 60 * 60 * 1000;
const DETENT_DIR_PREFIX = "detent-" as const;

export const cleanupOrphanedClones = (repoRoot: string): number => {
  // No git worktree prune needed - shallow clones are self-contained
  return cleanOrphanedTempDirs(repoRoot);
};

// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: Security-critical symlink and path validation
const cleanOrphanedTempDirs = (repoRoot: string): number => {
  const tempDir = tmpdir();
  let entries: string[];

  try {
    entries = readdirSync(tempDir);
  } catch {
    return 0;
  }

  let removed = 0;

  for (const entry of entries) {
    if (!entry.startsWith(DETENT_DIR_PREFIX)) {
      continue;
    }

    if (entry.includes("..") || entry.includes("/") || entry.includes("\\")) {
      continue;
    }

    const fullPath = join(tempDir, entry);
    let info: ReturnType<typeof lstatSync> | undefined;

    try {
      info = lstatSync(fullPath);
    } catch {
      continue;
    }

    if (info.isSymbolicLink()) {
      continue;
    }

    if (!info.isDirectory()) {
      continue;
    }

    // Check lock status - skip if locked by a live process
    const lockStatus = checkLockStatus(fullPath);
    if (lockStatus === "busy") {
      continue;
    }

    // Only clean up if old enough (unless the lock check determined it's free)
    const age = Date.now() - info.mtimeMs;
    if (age < ORPHAN_AGE_THRESHOLD && lockStatus !== "free") {
      continue;
    }

    if (!isCloneForRepo(fullPath, repoRoot)) {
      continue;
    }

    try {
      const finalCheck = lstatSync(fullPath);
      if (finalCheck.isSymbolicLink()) {
        continue;
      }

      rmSync(fullPath, { recursive: true, force: true, maxRetries: 3 });
      removed++;
    } catch {
      // Best effort - ignore errors
    }
  }

  return removed;
};

/**
 * Checks if a clone directory belongs to the given repository.
 * For shallow clones, we check .git/config for the file:// origin URL.
 */
const isCloneForRepo = (clonePath: string, repoRoot: string): boolean => {
  const configPath = join(clonePath, ".git", "config");

  let info: ReturnType<typeof lstatSync> | undefined;
  try {
    info = lstatSync(configPath);
  } catch {
    return false;
  }

  if (info.isSymbolicLink()) {
    return false;
  }

  if (!info.isFile()) {
    return false;
  }

  let content: string;
  try {
    content = readFileSync(configPath, "utf-8");
  } catch {
    return false;
  }

  // Shallow clones from file:// URLs have the source repo path in config
  return content.includes(`file://${repoRoot}`) || content.includes(repoRoot);
};
