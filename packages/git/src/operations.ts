import type { CommitSHA, GitRefs, TreeHash } from "./types.js";
import { execGit } from "./utils.js";

export const getCurrentRefs = async (repoRoot: string): Promise<GitRefs> => {
  const result = await execGit(["rev-parse", "HEAD", "HEAD^{tree}"], {
    cwd: repoRoot,
  });

  const lines = result.stdout.split("\n");
  if (lines.length !== 2) {
    throw new Error(
      `unexpected git rev-parse output: expected 2 lines, got ${lines.length}`
    );
  }

  const commitSHA = lines[0];
  const treeHash = lines[1];

  if (!(commitSHA && treeHash)) {
    throw new Error("failed to parse git refs: empty output");
  }

  return {
    commitSHA: commitSHA as CommitSHA,
    treeHash: treeHash as TreeHash,
  };
};

export const getRemoteUrl = async (
  repoRoot: string
): Promise<string | null> => {
  try {
    const result = await execGit(["remote", "get-url", "origin"], {
      cwd: repoRoot,
    });
    const url = result.stdout;
    return url ? url : null;
  } catch {
    return null;
  }
};

export const getFirstCommitSha = async (
  repoRoot: string
): Promise<string | null> => {
  try {
    const result = await execGit(["rev-list", "--max-parents=0", "HEAD"], {
      cwd: repoRoot,
    });
    const lines = result.stdout.split("\n");
    const firstLine = lines[0];
    if (lines.length === 0 || !firstLine || firstLine === "") {
      return null;
    }
    return firstLine;
  } catch {
    return null;
  }
};

export const getCurrentBranch = async (repoRoot: string): Promise<string> => {
  try {
    const result = await execGit(["symbolic-ref", "--short", "HEAD"], {
      cwd: repoRoot,
    });
    return result.stdout;
  } catch {
    return "(HEAD detached)";
  }
};

export const getDirtyFilesList = async (
  repoRoot: string
): Promise<string[]> => {
  const result = await execGit(["status", "--porcelain", "-uall"], {
    cwd: repoRoot,
  });

  if (result.stdout === "") {
    return [];
  }

  return result.stdout.split("\n");
};

/**
 * Finds the git repository root from a starting path.
 * Traverses upward to find the .git directory.
 *
 * @param startPath - Directory to start searching from
 * @returns Absolute path to git root, or null if not in a git repo
 */
export const findGitRoot = async (
  startPath: string
): Promise<string | null> => {
  try {
    const result = await execGit(["rev-parse", "--show-toplevel"], {
      cwd: startPath,
    });
    const root = result.stdout;
    return root || null;
  } catch {
    return null;
  }
};

export const commitAllChanges = async (
  repoRoot: string,
  message: string
): Promise<void> => {
  if (!message || typeof message !== "string") {
    throw new Error("commit message must be a non-empty string");
  }

  if (message.includes("\0")) {
    throw new Error("commit message contains null bytes");
  }

  if (message.length > 10_000) {
    throw new Error(
      "commit message exceeds maximum length of 10000 characters"
    );
  }

  if (message.startsWith("-")) {
    throw new Error("commit message must not start with a dash");
  }

  await execGit(["add", "--", "."], { cwd: repoRoot });
  await execGit(["commit", "-m", message, "--"], { cwd: repoRoot });
};
