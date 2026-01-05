// biome-ignore-all lint/performance/noBarrelFile: index file for preflight module exports
import {
  checkActInstalled,
  checkDockerRunning,
  checkGitRepository,
  checkNoEscapingSymlinks,
  checkNoSubmodules,
} from "./checks.js";
import type { PreflightSummary } from "./types.js";

/**
 * Runs all preflight checks in two phases to validate the environment.
 *
 * Phase 1 (parallel): git repository, act installed, docker running
 * Phase 2 (parallel): no submodules, no escaping symlinks
 *
 * Phase 2 only runs if all Phase 1 checks pass. This ensures we don't
 * waste time checking repository structure if basic requirements aren't met.
 *
 * @param repoRoot - Absolute path to the repository root
 * @returns PreflightSummary with all check results and overall status
 */
export const runPreflightChecks = async (
  repoRoot: string
): Promise<PreflightSummary> => {
  const phase1Results = await Promise.all([
    checkGitRepository(),
    checkActInstalled(),
    checkDockerRunning(),
  ]);

  const phase1Checks = [
    { name: "git repository", result: phase1Results[0] },
    { name: "act installed", result: phase1Results[1] },
    { name: "docker running", result: phase1Results[2] },
  ];

  const phase1Passed = phase1Results.every((result) => result.passed);

  if (!phase1Passed) {
    return {
      checks: phase1Checks.map(({ name, result }) => ({
        name,
        passed: result.passed,
        message: result.message,
      })),
      allPassed: false,
    };
  }

  const phase2Results = await Promise.all([
    checkNoSubmodules(repoRoot),
    checkNoEscapingSymlinks(repoRoot),
  ]);

  const phase2Checks = [
    { name: "no submodules", result: phase2Results[0] },
    { name: "no escaping symlinks", result: phase2Results[1] },
  ];

  const allChecks = [...phase1Checks, ...phase2Checks];
  const allPassed = allChecks.every((check) => check.result.passed);

  return {
    checks: allChecks.map(({ name, result }) => ({
      name,
      passed: result.passed,
      message: result.message,
    })),
    allPassed,
  };
};

export {
  checkActInstalled,
  checkDockerRunning,
  checkGitRepository,
  checkNoEscapingSymlinks,
  checkNoSubmodules,
} from "./checks.js";
export {
  ActNotInstalledError,
  DockerNotRunningError,
  GitRepositoryNotFoundError,
  PreflightError,
  SubmodulesNotSupportedError,
  SymlinkEscapeError,
} from "./errors.js";
export type {
  PreflightCheck,
  PreflightResult,
  PreflightSummary,
} from "./types.js";
