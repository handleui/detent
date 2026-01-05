import { lstatSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execGit } from "./utils.js";

const ORPHAN_AGE_THRESHOLD = 60 * 60 * 1000;
const DETENT_DIR_PREFIX = "detent-" as const;

export const cleanupOrphanedWorktrees = async (
  repoRoot: string
): Promise<number> => {
  try {
    await execGit(["worktree", "prune"], { cwd: repoRoot });
  } catch {
    // Best effort
  }

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

    const age = Date.now() - info.mtimeMs;
    if (age < ORPHAN_AGE_THRESHOLD) {
      continue;
    }

    if (!isWorktreeForRepo(fullPath, repoRoot)) {
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

const isWorktreeForRepo = (worktreePath: string, repoRoot: string): boolean => {
  const gitPath = join(worktreePath, ".git");

  let info: ReturnType<typeof lstatSync> | undefined;
  try {
    info = lstatSync(gitPath);
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
    content = readFileSync(gitPath, "utf-8");
  } catch {
    return false;
  }

  const repoGitDir = join(repoRoot, ".git");
  return content.includes(repoGitDir);
};
