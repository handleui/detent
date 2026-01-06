// biome-ignore-all lint/performance/noBarrelFile: This is the package entry point
export { cleanupOrphanedClones } from "./cleanup.js";
export type { PrepareCloneOptions, PrepareCloneResult } from "./clone.js";
export { prepareClone } from "./clone.js";
export type { LockResult } from "./lock.js";
export {
  checkLockStatus,
  isProcessAlive,
  LOCK_FILE_NAME,
  readLockPid,
  tryAcquireLock,
} from "./lock.js";
export {
  commitAllChanges,
  findGitRoot,
  getCurrentBranch,
  getCurrentRefs,
  getDirtyFilesList,
  getFirstCommitSha,
  getRemoteUrl,
} from "./operations.js";
export {
  computeCurrentRunID,
  computeRunID,
  createEphemeralClonePath,
} from "./run-id.js";
export type {
  CloneInfo,
  CommitSHA,
  GitExecOptions,
  GitExecResult,
  GitRefs,
  RunID,
  RunIDInfo,
  TreeHash,
} from "./types.js";
export {
  ErrCloneNotInitialized,
  ErrGitTimeout,
  ErrInvalidInput,
  ErrNotGitRepository,
  ErrSubmodulesNotSupported,
  ErrSymlinkEscape,
  ErrSymlinkLimitExceeded,
} from "./types.js";
export { execGit, isValidRunID, safeGitEnv } from "./utils.js";
export {
  validateGitRepository,
  validateNoEscapingSymlinks,
  validateNoSubmodules,
} from "./validation.js";
