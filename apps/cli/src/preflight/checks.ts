import { execFile } from "node:child_process";
import { promisify } from "node:util";
import {
  validateGitRepository,
  validateNoEscapingSymlinks,
  validateNoSubmodules,
} from "@detent/git";
import { isInstalled } from "../act/index.js";
import {
  ActNotInstalledError,
  DockerNotRunningError,
  GitRepositoryNotFoundError,
  SubmodulesNotSupportedError,
  SymlinkEscapeError,
} from "./errors.js";
import type { PreflightResult } from "./types.js";

const execFileAsync = promisify(execFile);

/**
 * Checks if the current working directory is a valid git repository.
 *
 * @returns PreflightResult indicating if the check passed
 */
export const checkGitRepository = async (): Promise<PreflightResult> => {
  try {
    await validateGitRepository(process.cwd());
    return {
      passed: true,
      message: "git repository detected",
    };
  } catch {
    return {
      passed: false,
      message: "current directory is not a git repository",
      error: new GitRepositoryNotFoundError(process.cwd()),
    };
  }
};

/**
 * Checks if the act binary is installed and available.
 *
 * @returns PreflightResult indicating if the check passed
 */
export const checkActInstalled = async (): Promise<PreflightResult> => {
  try {
    const installed = isInstalled();
    if (installed) {
      return {
        passed: true,
        message: "act is installed",
      };
    }
    return {
      passed: false,
      message: "act binary not found",
      error: new ActNotInstalledError(),
    };
  } catch (error) {
    return {
      passed: false,
      message: "failed to check act installation",
      error:
        error instanceof Error
          ? new ActNotInstalledError(error.message)
          : new ActNotInstalledError(),
    };
  }
};

/**
 * Checks if Docker daemon is running and accessible.
 *
 * Uses `docker info` command to verify the daemon is responding.
 * This works regardless of whether Docker CLI exists but daemon is stopped.
 *
 * @returns PreflightResult indicating if the check passed
 */
export const checkDockerRunning = async (): Promise<PreflightResult> => {
  try {
    await execFileAsync("docker", ["info"], {
      timeout: 5000,
      env: { ...process.env },
    });

    return {
      passed: true,
      message: "Docker daemon is running",
    };
  } catch (error) {
    const errorMessage =
      error instanceof Error && "code" in error && error.code === "ENOENT"
        ? "docker command not found - please install Docker"
        : "Docker daemon is not running or not accessible";

    return {
      passed: false,
      message: errorMessage,
      error: new DockerNotRunningError(errorMessage),
    };
  }
};

/**
 * Checks if the repository contains any git submodules.
 *
 * Submodules are not yet supported by detent and must be removed or avoided.
 *
 * @param repoRoot - Absolute path to the repository root
 * @returns PreflightResult indicating if the check passed
 */
export const checkNoSubmodules = async (
  repoRoot: string
): Promise<PreflightResult> => {
  try {
    await validateNoSubmodules(repoRoot);
    return {
      passed: true,
      message: "no submodules detected",
    };
  } catch (error) {
    const message =
      error instanceof Error ? error.message : "submodules detected";
    return {
      passed: false,
      message,
      error:
        error instanceof Error
          ? new SubmodulesNotSupportedError(error.message)
          : new SubmodulesNotSupportedError(),
    };
  }
};

/**
 * Checks if any symlinks in the repository escape the repository boundary.
 *
 * Symlinks that point outside the repository are a security risk and are not allowed.
 *
 * @param repoRoot - Absolute path to the repository root
 * @returns PreflightResult indicating if the check passed
 */
export const checkNoEscapingSymlinks = async (
  repoRoot: string
): Promise<PreflightResult> => {
  try {
    await validateNoEscapingSymlinks(repoRoot);
    return {
      passed: true,
      message: "no escaping symlinks detected",
    };
  } catch (error) {
    const message =
      error instanceof Error
        ? error.message
        : "symlinks escape repository boundary";
    return {
      passed: false,
      message,
      error:
        error instanceof Error
          ? new SymlinkEscapeError(error.message)
          : new SymlinkEscapeError("symlinks escape repository boundary"),
    };
  }
};
