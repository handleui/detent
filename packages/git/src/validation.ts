import {
  ErrNotGitRepository,
  ErrSubmodulesNotSupported,
  ErrSymlinkEscape,
  ErrSymlinkLimitExceeded,
} from "./types.js";
import { execGit } from "./utils.js";

const MAX_SYMLINK_DEPTH = 100;
const MAX_SYMLINKS_CHECKED = 10_000;
const MAX_CONCURRENT_VALIDATIONS = 20;

const SKIP_DIRECTORIES = [".git", "node_modules", "vendor", ".venv"] as const;

export const validateGitRepository = async (path: string): Promise<void> => {
  if (!path || typeof path !== "string") {
    throw new ErrNotGitRepository(path);
  }

  if (path.includes("\0")) {
    throw new ErrNotGitRepository("path contains null bytes");
  }

  if (path.length > 4096) {
    throw new ErrNotGitRepository("path exceeds maximum length of 4096 bytes");
  }

  try {
    await execGit(["rev-parse", "--git-dir"], { cwd: path });
  } catch {
    throw new ErrNotGitRepository(path);
  }
};

export const validateNoSubmodules = async (repoRoot: string): Promise<void> => {
  const { readFile } = await import("node:fs/promises");
  const { join } = await import("node:path");

  const gitmodulesPath = join(repoRoot, ".gitmodules");

  try {
    const content = await readFile(gitmodulesPath, "utf8");

    if (content.includes("\r")) {
      throw new ErrSubmodulesNotSupported(
        ".gitmodules contains carriage return characters (potential CVE-2025-48384 attack)"
      );
    }

    throw new ErrSubmodulesNotSupported(
      "please remove submodules or use a repository without them"
    );
  } catch (error) {
    if (error instanceof ErrSubmodulesNotSupported) {
      throw error;
    }
    if (error instanceof Error && "code" in error && error.code === "ENOENT") {
      return;
    }
    throw new Error(`reading .gitmodules: ${error}`);
  }
};

export const validateNoEscapingSymlinks = async (
  repoRoot: string
): Promise<void> => {
  const { readdir, readlink, realpath } = await import("node:fs/promises");
  const { join, relative, sep, isAbsolute, dirname, basename } = await import(
    "node:path"
  );

  let absRepoRoot: string;
  try {
    absRepoRoot = await realpath(repoRoot);
  } catch {
    const { resolve } = await import("node:path");
    absRepoRoot = resolve(repoRoot);
  }

  let symlinksChecked = 0;

  const walkDir = async (currentPath: string, depth: number): Promise<void> => {
    const rel = relative(absRepoRoot, currentPath);

    const currentDepth = rel === "." ? 0 : rel.split(sep).length;
    if (currentDepth > MAX_SYMLINK_DEPTH) {
      throw new ErrSymlinkLimitExceeded(
        `maximum traversal depth (${MAX_SYMLINK_DEPTH}) exceeded`
      );
    }

    let entries: {
      name: string;
      isDirectory: () => boolean;
      isSymbolicLink: () => boolean;
    }[];
    try {
      entries = await readdir(currentPath, { withFileTypes: true });
    } catch {
      return;
    }

    const subdirPromises: Promise<void>[] = [];
    let activeTasks = 0;

    for (const entry of entries) {
      const entryPath = join(currentPath, entry.name);

      if (
        entry.isDirectory() &&
        SKIP_DIRECTORIES.includes(
          entry.name as (typeof SKIP_DIRECTORIES)[number]
        )
      ) {
        continue;
      }

      if (entry.isSymbolicLink()) {
        symlinksChecked++;
        if (symlinksChecked > MAX_SYMLINKS_CHECKED) {
          throw new ErrSymlinkLimitExceeded(
            `maximum symlink count (${MAX_SYMLINKS_CHECKED}) exceeded`
          );
        }

        let target: string;
        try {
          target = await realpath(entryPath);
        } catch {
          const rawTarget = await readlink(entryPath);

          if (isAbsolute(rawTarget)) {
            let absTarget = rawTarget;

            const parentDir = dirname(rawTarget);
            try {
              const resolvedParent = await realpath(parentDir);
              absTarget = join(resolvedParent, basename(rawTarget));
            } catch {
              // ignore
            }

            const relTarget = relative(absRepoRoot, absTarget);
            if (relTarget.startsWith("..")) {
              throw new ErrSymlinkEscape(
                `broken symlink ${entryPath} points outside repository (target: ${rawTarget})`
              );
            }
          }

          continue;
        }

        const relTarget = relative(absRepoRoot, target);
        if (relTarget.startsWith("..")) {
          throw new ErrSymlinkEscape(`${entryPath} points to ${target}`);
        }
      }

      if (entry.isDirectory() && !entry.isSymbolicLink()) {
        if (activeTasks >= MAX_CONCURRENT_VALIDATIONS) {
          await Promise.all(subdirPromises);
          subdirPromises.length = 0;
          activeTasks = 0;
        }
        subdirPromises.push(walkDir(entryPath, depth + 1));
        activeTasks++;
      }
    }

    await Promise.all(subdirPromises);
  };

  await walkDir(absRepoRoot, 0);
};
