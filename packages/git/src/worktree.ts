import { lstatSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { getDirtyFilesList } from "./operations.js";
import type { CommitSHA, WorktreeInfo } from "./types.js";
import { execGit } from "./utils.js";

const CLEANUP_TIMEOUT = 30_000;
const DIRECTORY_MODE = 0o700;

export interface PrepareWorktreeOptions {
  readonly repoRoot: string;
  readonly worktreePath?: string;
}

export interface PrepareWorktreeResult {
  readonly worktreeInfo: WorktreeInfo;
  readonly cleanup: () => Promise<void>;
}

export const prepareWorktree = async (
  options: PrepareWorktreeOptions
): Promise<PrepareWorktreeResult> => {
  const { repoRoot, worktreePath } = options;

  const commitResult = await execGit(["rev-parse", "HEAD"], { cwd: repoRoot });
  const commitSHA = commitResult.stdout as CommitSHA;

  if (!worktreePath) {
    throw new Error("worktreePath is required");
  }

  if (typeof worktreePath !== "string") {
    throw new Error("worktreePath must be a string");
  }

  if (worktreePath.includes("\0")) {
    throw new Error("worktreePath must not contain null bytes");
  }

  if (worktreePath.length > 4096) {
    throw new Error("worktreePath exceeds maximum length of 4096 bytes");
  }

  const finalPath = worktreePath;

  const validatePath = (path: string): void => {
    try {
      const info = lstatSync(path);
      if (info.isSymbolicLink()) {
        throw new Error(
          `worktree path ${path} is a symlink, refusing to proceed`
        );
      }
      if (!info.isDirectory()) {
        throw new Error(`worktree path ${path} is not a directory`);
      }
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code !== "ENOENT") {
        throw err;
      }
    }
  };

  validatePath(finalPath);

  try {
    mkdirSync(finalPath, { recursive: true, mode: DIRECTORY_MODE });
  } catch (err) {
    const error = err as NodeJS.ErrnoException;
    if (error.code !== "EEXIST") {
      throw new Error(`creating worktree directory: ${error.message}`);
    }
  }

  validatePath(finalPath);

  try {
    await execGit(["worktree", "add", "-d", finalPath, commitSHA], {
      cwd: repoRoot,
    });
  } catch (err) {
    const error = err as Error;
    if (error.message.includes("already exists")) {
      try {
        await execGit(["worktree", "remove", "--force", finalPath], {
          cwd: repoRoot,
        });
      } catch {
        // Ignore cleanup errors
      }

      await execGit(["worktree", "add", "-d", finalPath, commitSHA], {
        cwd: repoRoot,
      });
    } else {
      throw error;
    }
  }

  await syncDirtyFiles(repoRoot, finalPath);

  const worktreeInfo: WorktreeInfo = {
    path: finalPath,
    commitSHA,
  };

  const cleanup = async (): Promise<void> => {
    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => {
        reject(
          new Error(`worktree cleanup timed out after ${CLEANUP_TIMEOUT}ms`)
        );
      }, CLEANUP_TIMEOUT);
    });

    const cleanupPromise = execGit(
      ["worktree", "remove", "--force", finalPath],
      {
        cwd: repoRoot,
      }
    );

    try {
      await Promise.race([cleanupPromise, timeoutPromise]);
    } catch (err) {
      throw new Error(`failed to remove worktree at ${finalPath}: ${err}`);
    }
  };

  return { worktreeInfo, cleanup };
};

const syncDirtyFiles = async (
  repoRoot: string,
  worktreePath: string
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
      const dst = join(worktreePath, filePath);

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
