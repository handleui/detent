import { describe, expect, test } from "vitest";
import { computeRunID, createEphemeralWorktreePath } from "./run-id.js";
import type {
  CommitSHA,
  GitRefs,
  RunID,
  RunIDInfo,
  TreeHash,
} from "./types.js";
import {
  ErrNotGitRepository,
  ErrSubmodulesNotSupported,
  ErrSymlinkEscape,
  ErrSymlinkLimitExceeded,
  ErrWorktreeNotInitialized,
} from "./types.js";
import { isValidRunID, safeGitEnv } from "./utils.js";

const HEX_16_PATTERN = /^[0-9a-f]{16}$/;
const WORKTREE_PATH_PATTERN = /detent-[0-9a-f]+-[0-9a-z]+-[0-9a-z]+/;

describe("run-id", () => {
  test("computeRunID is deterministic", () => {
    const result1 = computeRunID("abc123" as TreeHash, "def456" as CommitSHA);
    const result2 = computeRunID("abc123" as TreeHash, "def456" as CommitSHA);
    expect(result1).toBe(result2);
    expect(result1).toHaveLength(16);
  });

  test("computeRunID generates different IDs for different inputs", () => {
    const result1 = computeRunID("abc123" as TreeHash, "def456" as CommitSHA);
    const result2 = computeRunID("xyz789" as TreeHash, "uvw012" as CommitSHA);
    expect(result1).not.toBe(result2);
  });

  test("computeRunID generates hex string", () => {
    const result = computeRunID("abc123" as TreeHash, "def456" as CommitSHA);
    expect(result).toMatch(HEX_16_PATTERN);
  });

  test("createEphemeralWorktreePath validates runID", () => {
    expect(() =>
      createEphemeralWorktreePath("../../etc/passwd" as RunID)
    ).toThrow("invalid run ID");
  });

  test("createEphemeralWorktreePath generates valid path", () => {
    const validRunID = "a1b2c3d4e5f60123" as RunID;
    const path = createEphemeralWorktreePath(validRunID);
    expect(path).toContain(`detent-${validRunID}`);
    expect(path).toMatch(WORKTREE_PATH_PATTERN);
  });

  test("createEphemeralWorktreePath rejects empty runID", () => {
    expect(() => createEphemeralWorktreePath("" as RunID)).toThrow(
      "invalid run ID"
    );
  });

  test("createEphemeralWorktreePath rejects non-hex runID", () => {
    expect(() => createEphemeralWorktreePath("invalid!@#" as RunID)).toThrow(
      "invalid run ID"
    );
  });
});

describe("utils", () => {
  test("safeGitEnv returns only allowed vars", () => {
    const env = safeGitEnv();
    expect(env.GIT_CONFIG_NOSYSTEM).toBe("1");
    expect(env.GIT_CONFIG_NOGLOBAL).toBe("1");
    expect(env.GIT_TERMINAL_PROMPT).toBe("0");
    expect(env.GIT_ASKPASS).toBe("/bin/true");
    expect(env.GIT_EDITOR).toBe("/bin/true");
    expect(env.GIT_PAGER).toBe("cat");
    expect(env.GIT_ATTR_NOSYSTEM).toBe("1");
  });

  test("safeGitEnv includes SSH command", () => {
    const env = safeGitEnv();
    expect(env.GIT_SSH_COMMAND).toContain("BatchMode=yes");
    expect(env.GIT_SSH_COMMAND).toContain("StrictHostKeyChecking=accept-new");
  });

  test("safeGitEnv preserves PATH if present", () => {
    const env = safeGitEnv();
    if (process.env.PATH) {
      expect(env.PATH).toBe(process.env.PATH);
    }
  });

  test("isValidRunID accepts valid lowercase hex", () => {
    expect(isValidRunID("a1b2c3d4e5f6")).toBe(true);
  });

  test("isValidRunID accepts valid uppercase hex", () => {
    expect(isValidRunID("A1B2C3D4E5F6")).toBe(true);
  });

  test("isValidRunID accepts mixed case hex", () => {
    expect(isValidRunID("aAbBcCdDeEfF")).toBe(true);
  });

  test("isValidRunID rejects path traversal", () => {
    expect(isValidRunID("../../etc/passwd")).toBe(false);
  });

  test("isValidRunID rejects empty string", () => {
    expect(isValidRunID("")).toBe(false);
  });

  test("isValidRunID rejects non-hex characters", () => {
    expect(isValidRunID("xyz123")).toBe(false);
    expect(isValidRunID("abc!@#")).toBe(false);
    expect(isValidRunID("abc-def")).toBe(false);
  });

  test("isValidRunID rejects overly long strings", () => {
    const longString = "a".repeat(65);
    expect(isValidRunID(longString)).toBe(false);
  });

  test("isValidRunID accepts maximum length", () => {
    const maxLengthString = "a".repeat(64);
    expect(isValidRunID(maxLengthString)).toBe(true);
  });
});

describe("types", () => {
  test("ErrWorktreeNotInitialized can be instantiated", () => {
    const err = new ErrWorktreeNotInitialized();
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("ErrWorktreeNotInitialized");
    expect(err.message).toContain("worktree not initialized");
  });

  test("ErrWorktreeNotInitialized accepts custom message", () => {
    const err = new ErrWorktreeNotInitialized("custom message");
    expect(err.message).toBe("custom message");
  });

  test("ErrNotGitRepository can be instantiated", () => {
    const err = new ErrNotGitRepository("/some/path");
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("ErrNotGitRepository");
    expect(err.message).toContain("/some/path");
  });

  test("ErrSymlinkEscape can be instantiated", () => {
    const err = new ErrSymlinkEscape("symlink escape detected");
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("ErrSymlinkEscape");
    expect(err.message).toBe("symlink escape detected");
  });

  test("ErrSubmodulesNotSupported can be instantiated", () => {
    const err = new ErrSubmodulesNotSupported();
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("ErrSubmodulesNotSupported");
    expect(err.message).toContain("submodules");
  });

  test("ErrSymlinkLimitExceeded can be instantiated", () => {
    const err = new ErrSymlinkLimitExceeded("too many symlinks");
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("ErrSymlinkLimitExceeded");
    expect(err.message).toBe("too many symlinks");
  });

  test("error classes can be thrown and caught", () => {
    expect(() => {
      throw new ErrNotGitRepository("/test");
    }).toThrow(ErrNotGitRepository);
  });
});

describe("type exports", () => {
  test("GitRefs type structure", () => {
    const refs: GitRefs = {
      commitSHA: "abc123" as CommitSHA,
      treeHash: "def456" as TreeHash,
    };
    expect(refs.commitSHA).toBe("abc123");
    expect(refs.treeHash).toBe("def456");
  });

  test("RunIDInfo type structure", () => {
    const info: RunIDInfo = {
      runID: "a1b2c3d4" as RunID,
      treeHash: "tree123" as TreeHash,
      commitSHA: "commit456" as CommitSHA,
    };
    expect(info.runID).toBe("a1b2c3d4");
    expect(info.treeHash).toBe("tree123");
    expect(info.commitSHA).toBe("commit456");
  });
});
