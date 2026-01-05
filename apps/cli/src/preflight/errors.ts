/**
 * Base error class for all preflight check failures.
 */
export class PreflightError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "PreflightError";
    Object.setPrototypeOf(this, PreflightError.prototype);
  }
}

/**
 * Error thrown when the current directory is not a git repository.
 */
export class GitRepositoryNotFoundError extends PreflightError {
  constructor(path: string) {
    super(`not a git repository: ${path}`);
    this.name = "GitRepositoryNotFoundError";
    Object.setPrototypeOf(this, GitRepositoryNotFoundError.prototype);
  }
}

/**
 * Error thrown when the act binary is not installed.
 */
export class ActNotInstalledError extends PreflightError {
  constructor(message = "act is not installed - run installation to continue") {
    super(message);
    this.name = "ActNotInstalledError";
    Object.setPrototypeOf(this, ActNotInstalledError.prototype);
  }
}

/**
 * Error thrown when Docker daemon is not running or not accessible.
 */
export class DockerNotRunningError extends PreflightError {
  constructor(
    message = "Docker daemon is not running or not accessible - please start Docker"
  ) {
    super(message);
    this.name = "DockerNotRunningError";
    Object.setPrototypeOf(this, DockerNotRunningError.prototype);
  }
}

/**
 * Error thrown when git submodules are detected in the repository.
 */
export class SubmodulesNotSupportedError extends PreflightError {
  constructor(message = "submodules are not yet supported") {
    super(message);
    this.name = "SubmodulesNotSupportedError";
    Object.setPrototypeOf(this, SubmodulesNotSupportedError.prototype);
  }
}

/**
 * Error thrown when symlinks escape the repository boundary.
 */
export class SymlinkEscapeError extends PreflightError {
  constructor(message: string) {
    super(message);
    this.name = "SymlinkEscapeError";
    Object.setPrototypeOf(this, SymlinkEscapeError.prototype);
  }
}
