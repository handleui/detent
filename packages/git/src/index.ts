export { cleanupOrphanedWorktrees } from "./cleanup.js";
export {
  commitAllChanges,
  getCurrentBranch,
  getCurrentRefs,
  getDirtyFilesList,
  getFirstCommitSha,
  getRemoteUrl,
} from "./operations.js";
export {
  computeCurrentRunID,
  computeRunID,
  createEphemeralWorktreePath,
} from "./run-id.js";
export type {
  CommitSHA,
  GitExecOptions,
  GitExecResult,
  GitRefs,
  RunID,
  RunIDInfo,
  TreeHash,
  WorktreeInfo,
} from "./types.js";
export {
  ErrGitTimeout,
  ErrInvalidInput,
  ErrNotGitRepository,
  ErrSubmodulesNotSupported,
  ErrSymlinkEscape,
  ErrSymlinkLimitExceeded,
  ErrWorktreeNotInitialized,
} from "./types.js";
export { execGit, isValidRunID, safeGitEnv } from "./utils.js";
export {
  validateGitRepository,
  validateNoEscapingSymlinks,
  validateNoSubmodules,
} from "./validation.js";
export type {
  PrepareWorktreeOptions,
  PrepareWorktreeResult,
} from "./worktree.js";
export { prepareWorktree } from "./worktree.js";
