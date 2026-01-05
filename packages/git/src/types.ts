export type RunID = string & { readonly __brand: "RunID" };
export type CommitSHA = string & { readonly __brand: "CommitSHA" };
export type TreeHash = string & { readonly __brand: "TreeHash" };

export interface WorktreeInfo {
  readonly path: string;
  readonly commitSHA: CommitSHA;
}

export interface GitRefs {
  readonly commitSHA: CommitSHA;
  readonly treeHash: TreeHash;
}

export interface GitExecOptions {
  readonly cwd?: string;
  readonly timeout?: number;
  readonly maxBuffer?: number;
}

export interface GitExecResult {
  readonly stdout: string;
  readonly stderr: string;
}

export class ErrWorktreeNotInitialized extends Error {
  constructor(
    message = "worktree not initialized - Prepare() must be called before Run()"
  ) {
    super(message);
    this.name = "ErrWorktreeNotInitialized";
    Object.setPrototypeOf(this, ErrWorktreeNotInitialized.prototype);
  }
}

export class ErrNotGitRepository extends Error {
  constructor(path: string) {
    super(`not a git repository: ${path}`);
    this.name = "ErrNotGitRepository";
    Object.setPrototypeOf(this, ErrNotGitRepository.prototype);
  }
}

export class ErrSymlinkEscape extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ErrSymlinkEscape";
    Object.setPrototypeOf(this, ErrSymlinkEscape.prototype);
  }
}

export class ErrSubmodulesNotSupported extends Error {
  constructor(message = "submodules are not yet supported") {
    super(message);
    this.name = "ErrSubmodulesNotSupported";
    Object.setPrototypeOf(this, ErrSubmodulesNotSupported.prototype);
  }
}

export class ErrSymlinkLimitExceeded extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ErrSymlinkLimitExceeded";
    Object.setPrototypeOf(this, ErrSymlinkLimitExceeded.prototype);
  }
}

export class ErrInvalidInput extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ErrInvalidInput";
    Object.setPrototypeOf(this, ErrInvalidInput.prototype);
  }
}

export class ErrGitTimeout extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ErrGitTimeout";
    Object.setPrototypeOf(this, ErrGitTimeout.prototype);
  }
}

export interface RunIDInfo {
  readonly runID: RunID;
  readonly treeHash: TreeHash;
  readonly commitSHA: CommitSHA;
}
