import { lstatSync, mkdirSync } from "node:fs";
import { rm } from "node:fs/promises";
import { dirname, join } from "node:path";
import { checkLockStatus, tryAcquireLock } from "./lock.js";
import { getDirtyFilesList } from "./operations.js";
import type { CloneInfo, CommitSHA } from "./types.js";
import { execGit } from "./utils.js";

const CLEANUP_TIMEOUT = 30_000;
const DIRECTORY_MODE = 0o700;

export interface PrepareCloneOptions {
  readonly repoRoot: string;
  readonly clonePath?: string;
}

export interface PrepareCloneResult {
  readonly cloneInfo: CloneInfo;
  readonly cleanup: () => Promise<void>;
}

const validateClonePath = (clonePath: string | undefined): string => {
  if (!clonePath) {
    throw new Error("clonePath is required");
  }

  if (typeof clonePath !== "string") {
    throw new Error("clonePath must be a string");
  }

  if (clonePath.includes("\0")) {
    throw new Error("clonePath must not contain null bytes");
  }

  if (clonePath.length > 4096) {
    throw new Error("clonePath exceeds maximum length of 4096 bytes");
  }

  return clonePath;
};

const validatePathSecurity = (path: string): void => {
  try {
    const info = lstatSync(path);
    if (info.isSymbolicLink()) {
      throw new Error(`clone path ${path} is a symlink, refusing to proceed`);
    }
    if (!info.isDirectory()) {
      throw new Error(`clone path ${path} is not a directory`);
    }
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code !== "ENOENT") {
      throw err;
    }
  }
};

const checkExistingCloneLock = (finalPath: string): void => {
  try {
    lstatSync(finalPath);
    const lockStatus = checkLockStatus(finalPath);
    if (lockStatus === "busy") {
      throw new Error(
        `Clone ${finalPath} is locked by another process. ` +
          "If the process has died, remove the .detent.lock file manually."
      );
    }
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code !== "ENOENT") {
      throw err;
    }
  }
};

const createCloneDirectory = (finalPath: string): void => {
  try {
    mkdirSync(finalPath, { recursive: true, mode: DIRECTORY_MODE });
  } catch (err) {
    const error = err as NodeJS.ErrnoException;
    if (error.code !== "EEXIST") {
      throw new Error(`creating clone directory: ${error.message}`);
    }
  }
};

/**
 * Creates a shallow clone of the repository.
 * Uses `git clone --depth 1 file://...` for a self-contained .git directory
 * that works with Docker containers (unlike worktrees).
 */
const createShallowClone = async (
  repoRoot: string,
  clonePath: string,
  commitSHA: CommitSHA
): Promise<void> => {
  try {
    // Clone with depth 1 for minimal .git size, no checkout initially
    await execGit(
      [
        "clone",
        "--depth",
        "1",
        "--no-checkout",
        `file://${repoRoot}`,
        clonePath,
      ],
      { cwd: repoRoot }
    );

    // Checkout the specific commit
    await execGit(["checkout", commitSHA], { cwd: clonePath });
    return;
  } catch (err) {
    const error = err as Error;
    if (!error.message.includes("already exists")) {
      throw error;
    }
  }

  // If clone already exists, remove and retry
  try {
    await rm(clonePath, { recursive: true, force: true, maxRetries: 3 });
  } catch {
    // Ignore cleanup errors
  }

  await execGit(
    ["clone", "--depth", "1", "--no-checkout", `file://${repoRoot}`, clonePath],
    { cwd: repoRoot }
  );
  await execGit(["checkout", commitSHA], { cwd: clonePath });
};

const removeCloneSilently = async (clonePath: string): Promise<void> => {
  try {
    await rm(clonePath, { recursive: true, force: true, maxRetries: 3 });
  } catch {
    // Best effort
  }
};

const acquireCloneLock = async (
  clonePath: string
): Promise<{ release: () => void }> => {
  const lockResult = tryAcquireLock(clonePath);
  if (lockResult.success) {
    return { release: lockResult.release };
  }

  await removeCloneSilently(clonePath);
  throw new Error(
    `Failed to acquire lock on clone: ${lockResult.reason}${
      lockResult.error ? ` - ${lockResult.error.message}` : ""
    }`
  );
};

const createCleanupFunction = (
  clonePath: string,
  release: () => void
): (() => Promise<void>) => {
  return async (): Promise<void> => {
    release();

    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => {
        reject(new Error(`clone cleanup timed out after ${CLEANUP_TIMEOUT}ms`));
      }, CLEANUP_TIMEOUT);
    });

    const cleanupPromise = rm(clonePath, {
      recursive: true,
      force: true,
      maxRetries: 3,
    });

    try {
      await Promise.race([cleanupPromise, timeoutPromise]);
    } catch (err) {
      throw new Error(`failed to remove clone at ${clonePath}: ${err}`);
    }
  };
};

export const prepareClone = async (
  options: PrepareCloneOptions
): Promise<PrepareCloneResult> => {
  const { repoRoot, clonePath } = options;

  const commitResult = await execGit(["rev-parse", "HEAD"], { cwd: repoRoot });
  const commitSHA = commitResult.stdout as CommitSHA;

  const finalPath = validateClonePath(clonePath);
  validatePathSecurity(finalPath);
  checkExistingCloneLock(finalPath);
  createCloneDirectory(finalPath);
  validatePathSecurity(finalPath);

  await createShallowClone(repoRoot, finalPath, commitSHA);
  await syncDirtyFiles(repoRoot, finalPath);

  const { release } = await acquireCloneLock(finalPath);

  const cloneInfo: CloneInfo = {
    path: finalPath,
    commitSHA,
  };

  const cleanup = createCleanupFunction(finalPath, release);

  return { cloneInfo, cleanup };
};

const syncDirtyFiles = async (
  repoRoot: string,
  clonePath: string
): Promise<void> => {
  const files = await getDirtyFilesList(repoRoot);
  if (files.length === 0) {
    return;
  }

  const directoriesCreated = new Set<string>();

  const createDirIfNeeded = (dirPath: string): void => {
    if (directoriesCreated.has(dirPath)) {
      return;
    }
    try {
      mkdirSync(dirPath, { recursive: true, mode: 0o700 });
      directoriesCreated.add(dirPath);
    } catch (err) {
      const error = err as NodeJS.ErrnoException;
      if (error.code !== "EEXIST") {
        throw error;
      }
      directoriesCreated.add(dirPath);
    }
  };

  const BATCH_SIZE = 100;
  const batches: string[][] = [];
  for (let i = 0; i < files.length; i += BATCH_SIZE) {
    batches.push(files.slice(i, i + BATCH_SIZE));
  }

  for (const batch of batches) {
    const copyPromises: Promise<void>[] = [];

    for (const entry of batch) {
      if (entry.length < 3) {
        continue;
      }

      const status = entry.substring(0, 2);
      let filePath = entry.substring(3).trim();

      if (status[0] === "D" || status[1] === "D") {
        continue;
      }

      if (filePath.includes(" -> ")) {
        const parts = filePath.split(" -> ");
        if (parts.length === 2 && parts[1]) {
          filePath = parts[1].trim();
        }
      }

      const src = join(repoRoot, filePath);
      const dst = join(clonePath, filePath);

      const copyTask = (async (): Promise<void> => {
        try {
          const dstDir = dirname(dst);
          createDirIfNeeded(dstDir);

          const { copyFile } = await import("node:fs/promises");
          await copyFile(src, dst);
        } catch (err) {
          const error = err as NodeJS.ErrnoException;
          if (error.code !== "ENOENT") {
            console.error(
              `Warning: failed to copy ${filePath}: ${error.message}`
            );
          }
        }
      })();

      copyPromises.push(copyTask);
    }

    await Promise.allSettled(copyPromises);
  }
};
