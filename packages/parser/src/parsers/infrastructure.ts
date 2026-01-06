/**
 * Infrastructure error parser for build/runtime infrastructure failures.
 * Parses errors from package managers, shell commands, Docker, git, etc.
 *
 * This parser is CI-AGNOSTIC - it handles universal tool/shell error patterns
 * that appear the same whether running locally or in CI. The patterns come from
 * the tools themselves (npm, Docker, git, shells), not from CI runners.
 *
 * Supported patterns:
 * - Package manager script failures: `error: script "X" exited with code N`
 * - Command/tool failures: `"X" exited with code N`
 * - Context cancellation: `Error: context canceled`
 * - Shell command not found: `bash: X: command not found`
 * - Permission denied: `./script.sh: Permission denied`
 * - npm errors: `npm ERR! code ELIFECYCLE`
 * - Docker errors: `docker: Error response from daemon: ...`
 * - Git fatal errors: `fatal: ...`
 * - Node.js version errors (from npm/yarn/frameworks)
 * - Network/registry failures
 * - Disk/memory resource errors
 * - Authentication failures
 *
 * CI-specific patterns (like GitHub Actions "Process completed with exit code")
 * are handled separately by CI context parsers, not here.
 */

import { classifyExitCode } from "../exit-codes.js";
import {
  applyWorkflowContext,
  BaseParser,
  type NoisePatternProvider,
  type NoisePatterns,
  type ParseContext,
  type ParseResult,
} from "../parser-types.js";
import type { MutableExtractedError } from "../types.js";
import { stripAnsi } from "../utils.js";

// ============================================================================
// Constants
// ============================================================================

const PARSER_ID = "infrastructure";
/**
 * Priority 70: Lower than language parsers (80) so that tool-specific
 * errors (TypeScript, Go, etc.) are matched first. Infrastructure errors
 * are more generic and should only match if no language parser claims the line.
 */
const PARSER_PRIORITY = 70;

/** Maximum line length to process to prevent ReDoS */
const MAX_LINE_LENGTH = 2048;

// ============================================================================
// Error Patterns
// ============================================================================

/**
 * Package manager script failure.
 * Format: error: script "X" exited with code N
 * Groups: 1=script name, 2=exit code
 */
const packageManagerScriptPattern =
  /^error:\s*script\s*"([^"]+)"\s*exited\s*with\s*code\s*(\d+)/i;

/**
 * Generic command/tool exit code failure.
 * Format: "X" exited with code N
 * Groups: 1=command name, 2=exit code
 */
const commandExitPattern = /^"([^"]+)"\s*exited\s*with\s*code\s*(\d+)/i;

/**
 * Context cancellation error.
 * Format: Error: context canceled
 */
const contextCanceledPattern = /^Error:\s*context\s*canceled\s*$/i;

/**
 * Shell command not found.
 * Format: bash: X: command not found
 * Groups: 1=command name
 */
const shellNotFoundPattern =
  /^(?:bash|sh|zsh):\s*([^:]+):\s*command\s*not\s*found/i;

/**
 * Permission denied error.
 * Format: ./script.sh: Permission denied
 * Groups: 1=file path
 */
const permissionDeniedPattern = /^([^\s:]+):\s*Permission\s*denied\s*$/i;

/**
 * npm error with code.
 * Format: npm ERR! code ELIFECYCLE (or other error codes)
 * Groups: 1=error code
 */
const npmErrorPattern = /^npm\s+ERR!\s*code\s+(\S+)/i;

/**
 * Docker daemon error.
 * Format: docker: Error response from daemon: ...
 * Groups: 1=error message
 */
const dockerErrorPattern =
  /^docker:\s*Error\s*response\s*from\s*daemon:\s*(.+)/i;

/**
 * Git fatal error.
 * Format: fatal: ...
 * Groups: 1=error message
 */
const gitFatalPattern = /^fatal:\s*(.+)/i;

// ============================================================================
// Node.js Version Patterns
// ============================================================================

/**
 * Node.js version requirement error (Yarn/npm style).
 * Format: error You are running Node 14.x but this package requires Node >= 18
 * Groups: 1=current version, 2=required version
 */
const nodeVersionRequirementPattern =
  /^error\s+You\s+are\s+running\s+Node\s+(\S+)\s+but\s+this\s+package\s+requires\s+Node\s+(.+)/i;

/**
 * Engine incompatibility error.
 * Format: The engine "node" is incompatible with this module
 */
const engineIncompatiblePattern =
  /^(?:error\s+)?(?:@\S+:\s*)?The\s+engine\s+"node"\s+is\s+incompatible/i;

/**
 * Next.js Node.js version mismatch error.
 * Format: You are using Node.js 18.20.8. For Next.js, Node.js version ">=20.9.0" is required.
 * Groups: 1=current version, 2=framework name, 3=required version
 */
const nextjsNodeVersionPattern =
  /^You\s+are\s+using\s+Node\.js\s+([\d.]+)\.\s+For\s+([^,]+),\s+Node\.js\s+version\s+"([^"]+)"\s+is\s+required/i;

// ============================================================================
// Missing Scripts/Dependencies Patterns
// ============================================================================

/**
 * Missing script error (Yarn/Bun).
 * Format: error Missing script: "build"
 * Groups: 1=script name
 */
const missingScriptYarnPattern = /^error\s+Missing\s+script:\s*"([^"]+)"/i;

/**
 * Missing script error (npm).
 * Format: npm ERR! missing script: test
 * Groups: 1=script name
 */
const missingScriptNpmPattern = /^npm\s+ERR!\s*missing\s+script:\s*(\S+)/i;

/**
 * Missing script error (pnpm/generic).
 * Format: error: no such script: lint
 * Groups: 1=script name
 */
const missingScriptGenericPattern = /^error:\s*no\s+such\s+script:\s*(\S+)/i;

/**
 * Command not found (sh style).
 * Format: sh: 1: eslint: not found
 * Groups: 1=command name
 */
const shCommandNotFoundPattern = /^sh:\s*\d+:\s*(\S+):\s*not\s+found/i;

// ============================================================================
// Network/Registry Patterns
// ============================================================================

/**
 * npm network error codes.
 * Format: npm ERR! code ETIMEDOUT or npm ERR! code ECONNREFUSED
 * Groups: 1=error code
 */
const npmNetworkErrorPattern =
  /^npm\s+ERR!\s*code\s+(ETIMEDOUT|ECONNREFUSED|ECONNRESET|ENETUNREACH)/i;

/**
 * Registry request failed.
 * Format: error: request to https://registry.npmjs.org/... failed
 * Groups: 1=URL
 */
const registryRequestFailedPattern =
  /^error:\s*request\s+to\s+(https?:\/\/\S+)\s+failed/i;

/**
 * DNS resolution error.
 * Format: Error: getaddrinfo ENOTFOUND registry.npmjs.org
 * Groups: 1=hostname
 */
const dnsResolutionErrorPattern = /^Error:\s*getaddrinfo\s+ENOTFOUND\s+(\S+)/i;

// ============================================================================
// Disk/Resource Patterns
// ============================================================================

/**
 * No space left on device.
 * Format: ENOSPC: no space left on device
 */
const noSpaceLeftPattern = /ENOSPC:\s*no\s+space\s+left\s+on\s+device/i;

/**
 * Out of memory error.
 * Format: ENOMEM Cannot allocate memory
 */
const outOfMemoryPattern = /ENOMEM[:\s]+Cannot\s+allocate\s+memory/i;

/**
 * File/directory not found (npm).
 * Format: npm ERR! code ENOENT
 */
const npmEnoentPattern = /^npm\s+ERR!\s*code\s+ENOENT/i;

// ============================================================================
// Authentication Patterns
// ============================================================================

/**
 * npm 401 Unauthorized.
 * Format: npm ERR! 401 Unauthorized
 */
const npmUnauthorizedPattern = /^npm\s+ERR!\s*401\s+Unauthorized/i;

/**
 * npm 403 Forbidden.
 * Format: npm ERR! 403 Forbidden
 */
const npmForbiddenPattern = /^npm\s+ERR!\s*403\s+Forbidden/i;

/**
 * Generic authentication required.
 * Format: error: Authentication required
 */
const authRequiredPattern = /^error:\s*Authentication\s+required/i;

// ============================================================================
// Noise Patterns
// ============================================================================

/**
 * Fast prefix checks for noise detection (lowercase).
 */
const NOISE_FAST_PREFIXES: readonly string[] = [
  // npm informational messages
  "npm warn",
  "npm notice",
  "npm info",
  // Docker build progress
  "step ",
  "layer ",
  "#",
  // Git progress
  "remote:",
  "receiving objects:",
  "resolving deltas:",
  "unpacking objects:",
  // Package manager progress
  "downloading",
  "installing",
  "resolving",
];

/**
 * Fast substring checks for noise detection (lowercase).
 */
const NOISE_FAST_CONTAINS: readonly string[] = [
  // Success indicators (be careful not to match "completed with exit code")
  "successfully",
  "completed successfully",
  "installation completed",
  // Progress indicators
  "progress",
  "% done",
];

/**
 * Regex patterns for noise detection.
 */
const NOISE_REGEX_PATTERNS: readonly RegExp[] = [
  // npm timing and audit info
  /^npm\s+(timing|http|audit)/i,
  // Docker layer progress
  /^[a-f0-9]{12}:\s*(Pulling|Waiting|Downloading|Extracting|Verifying)/i,
  // Git progress with percentages
  /^\s*\d+%\s*\(/,
  // Empty or whitespace
  /^\s*$/,
];

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Check if a line matches any noise pattern.
 */
const matchesNoisePattern = (line: string, lowerTrimmed: string): boolean => {
  for (const prefix of NOISE_FAST_PREFIXES) {
    if (lowerTrimmed.startsWith(prefix)) {
      return true;
    }
  }

  for (const substr of NOISE_FAST_CONTAINS) {
    if (lowerTrimmed.includes(substr)) {
      return true;
    }
  }

  for (const pattern of NOISE_REGEX_PATTERNS) {
    if (pattern.test(line)) {
      return true;
    }
  }

  return false;
};

// ============================================================================
// Pattern Confidence Mapping
// ============================================================================

/**
 * Pattern-confidence pairs for quick confidence checks.
 * Higher confidence = more specific pattern match.
 */
interface PatternConfidence {
  pattern: RegExp;
  confidence: number;
}

const PATTERN_CONFIDENCE_LIST: readonly PatternConfidence[] = [
  // Core patterns
  { pattern: packageManagerScriptPattern, confidence: 0.9 },
  { pattern: commandExitPattern, confidence: 0.88 },
  { pattern: contextCanceledPattern, confidence: 0.95 },
  { pattern: shellNotFoundPattern, confidence: 0.92 },
  { pattern: permissionDeniedPattern, confidence: 0.91 },
  { pattern: npmErrorPattern, confidence: 0.89 },
  { pattern: dockerErrorPattern, confidence: 0.93 },
  { pattern: gitFatalPattern, confidence: 0.9 },
  // Node.js version patterns
  { pattern: nodeVersionRequirementPattern, confidence: 0.94 },
  { pattern: nextjsNodeVersionPattern, confidence: 0.95 },
  { pattern: engineIncompatiblePattern, confidence: 0.93 },
  // Missing script patterns
  { pattern: missingScriptYarnPattern, confidence: 0.92 },
  { pattern: missingScriptNpmPattern, confidence: 0.92 },
  { pattern: missingScriptGenericPattern, confidence: 0.91 },
  { pattern: shCommandNotFoundPattern, confidence: 0.92 },
  // Network/registry patterns
  { pattern: npmNetworkErrorPattern, confidence: 0.91 },
  { pattern: registryRequestFailedPattern, confidence: 0.9 },
  { pattern: dnsResolutionErrorPattern, confidence: 0.92 },
  // Disk/resource patterns
  { pattern: noSpaceLeftPattern, confidence: 0.95 },
  { pattern: outOfMemoryPattern, confidence: 0.95 },
  { pattern: npmEnoentPattern, confidence: 0.89 },
  // Authentication patterns
  { pattern: npmUnauthorizedPattern, confidence: 0.93 },
  { pattern: npmForbiddenPattern, confidence: 0.93 },
  { pattern: authRequiredPattern, confidence: 0.91 },
];

/**
 * Get confidence for a line by checking all patterns.
 */
const getPatternConfidence = (trimmed: string): number => {
  for (const { pattern, confidence } of PATTERN_CONFIDENCE_LIST) {
    if (pattern.test(trimmed)) {
      return confidence;
    }
  }
  return 0;
};

/**
 * Quick check if a line might be an infrastructure error.
 * Note: lowerTrimmed is already lowercase, so all checks use lowercase.
 *
 * These checks are for TOOL-LEVEL errors (npm, git, docker, shells), NOT
 * CI-runner-specific patterns. CI patterns are handled by context parsers.
 */
const mightBeInfrastructureError = (lowerTrimmed: string): boolean =>
  // Package manager and tool error prefixes
  lowerTrimmed.startsWith("error:") ||
  lowerTrimmed.startsWith("error ") ||
  lowerTrimmed.startsWith("fatal:") ||
  lowerTrimmed.startsWith("docker:") ||
  lowerTrimmed.startsWith("npm err!") ||
  // Shell error prefixes
  lowerTrimmed.startsWith("bash:") ||
  lowerTrimmed.startsWith("sh:") ||
  lowerTrimmed.startsWith("zsh:") ||
  // Tool exit code patterns (from package managers, not CI runners)
  lowerTrimmed.includes("exited with code") ||
  // Permission and path errors
  lowerTrimmed.includes("permission denied") ||
  lowerTrimmed.includes("context canceled") ||
  lowerTrimmed.includes("command not found") ||
  lowerTrimmed.includes("not found") ||
  // Node.js version patterns (from npm/yarn/frameworks)
  lowerTrimmed.includes("running node") ||
  lowerTrimmed.startsWith("you are using node") ||
  lowerTrimmed.includes("engine") ||
  // Missing script patterns
  lowerTrimmed.includes("missing script") ||
  lowerTrimmed.includes("no such script") ||
  // System error codes
  lowerTrimmed.includes("enospc") ||
  lowerTrimmed.includes("enomem") ||
  lowerTrimmed.includes("enoent") ||
  // Network error codes
  lowerTrimmed.includes("etimedout") ||
  lowerTrimmed.includes("econnrefused") ||
  lowerTrimmed.includes("enotfound") ||
  // Authentication errors
  lowerTrimmed.includes("401 unauthorized") ||
  lowerTrimmed.includes("403 forbidden") ||
  lowerTrimmed.includes("authentication required") ||
  // Registry/network request errors
  lowerTrimmed.includes("request to") ||
  lowerTrimmed.includes("getaddrinfo");

// ============================================================================
// Parser Implementation
// ============================================================================

/**
 * InfrastructureParser handles build/runtime infrastructure errors.
 * These are tool-level errors (npm, Docker, git, shells) that appear identically
 * whether running locally or in CI. CI-runner-specific patterns are NOT handled here.
 */
class InfrastructureParser extends BaseParser implements NoisePatternProvider {
  readonly id = PARSER_ID;
  readonly priority = PARSER_PRIORITY;

  /**
   * Returns confidence score for parsing the line.
   */
  canParse = (line: string, _ctx: ParseContext): number => {
    if (line.length > MAX_LINE_LENGTH) {
      return 0;
    }

    const stripped = stripAnsi(line);
    const trimmed = stripped.trim();
    const lowerTrimmed = trimmed.toLowerCase();

    if (!mightBeInfrastructureError(lowerTrimmed)) {
      return 0;
    }

    if (matchesNoisePattern(stripped, lowerTrimmed)) {
      return 0;
    }

    return getPatternConfidence(trimmed);
  };

  /**
   * Parse the line and extract an infrastructure error.
   */
  parse = (line: string, ctx: ParseContext): ParseResult => {
    if (line.length > MAX_LINE_LENGTH) {
      return null;
    }

    const stripped = stripAnsi(line);
    const trimmed = stripped.trim();

    // Order matters: try more specific patterns first
    return (
      this.parseNetworkPatterns(trimmed, line, ctx) ??
      this.parseResourcePatterns(trimmed, line, ctx) ??
      this.parseAuthPatterns(trimmed, line, ctx) ??
      this.parseCorePatterns(trimmed, line, ctx) ??
      this.parseNodePatterns(trimmed, line, ctx) ??
      this.parseMissingScriptPatterns(trimmed, line, ctx)
    );
  };

  /**
   * Parse core infrastructure patterns.
   */
  private readonly parseCorePatterns = (
    trimmed: string,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult => {
    const scriptMatch = packageManagerScriptPattern.exec(trimmed);
    if (scriptMatch) {
      return this.buildScriptError(scriptMatch, rawLine, ctx);
    }

    const commandMatch = commandExitPattern.exec(trimmed);
    if (commandMatch) {
      return this.buildCommandError(commandMatch, rawLine, ctx);
    }

    if (contextCanceledPattern.test(trimmed)) {
      return this.buildContextCanceledError(rawLine, ctx);
    }

    const shellMatch = shellNotFoundPattern.exec(trimmed);
    if (shellMatch) {
      return this.buildShellNotFoundError(shellMatch, rawLine, ctx);
    }

    const permissionMatch = permissionDeniedPattern.exec(trimmed);
    if (permissionMatch) {
      return this.buildPermissionDeniedError(permissionMatch, rawLine, ctx);
    }

    const npmMatch = npmErrorPattern.exec(trimmed);
    if (npmMatch) {
      return this.buildNpmError(npmMatch, rawLine, ctx);
    }

    const dockerMatch = dockerErrorPattern.exec(trimmed);
    if (dockerMatch) {
      return this.buildDockerError(dockerMatch, rawLine, ctx);
    }

    const gitMatch = gitFatalPattern.exec(trimmed);
    if (gitMatch) {
      return this.buildGitFatalError(gitMatch, rawLine, ctx);
    }

    return null;
  };

  /**
   * Parse Node.js version-related patterns.
   */
  private readonly parseNodePatterns = (
    trimmed: string,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult => {
    const nodeVersionMatch = nodeVersionRequirementPattern.exec(trimmed);
    if (nodeVersionMatch) {
      return this.buildNodeVersionError(nodeVersionMatch, rawLine, ctx);
    }

    const nextjsNodeMatch = nextjsNodeVersionPattern.exec(trimmed);
    if (nextjsNodeMatch) {
      return this.buildNextjsNodeVersionError(nextjsNodeMatch, rawLine, ctx);
    }

    if (engineIncompatiblePattern.test(trimmed)) {
      return this.buildEngineIncompatibleError(rawLine, ctx);
    }

    return null;
  };

  /**
   * Parse missing script patterns.
   */
  private readonly parseMissingScriptPatterns = (
    trimmed: string,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult => {
    const missingScriptYarnMatch = missingScriptYarnPattern.exec(trimmed);
    if (missingScriptYarnMatch) {
      return this.buildMissingScriptError(
        missingScriptYarnMatch[1] ?? "",
        rawLine,
        ctx
      );
    }

    const missingScriptNpmMatch = missingScriptNpmPattern.exec(trimmed);
    if (missingScriptNpmMatch) {
      return this.buildMissingScriptError(
        missingScriptNpmMatch[1] ?? "",
        rawLine,
        ctx
      );
    }

    const missingScriptGenericMatch = missingScriptGenericPattern.exec(trimmed);
    if (missingScriptGenericMatch) {
      return this.buildMissingScriptError(
        missingScriptGenericMatch[1] ?? "",
        rawLine,
        ctx
      );
    }

    const shNotFoundMatch = shCommandNotFoundPattern.exec(trimmed);
    if (shNotFoundMatch) {
      return this.buildShNotFoundError(shNotFoundMatch, rawLine, ctx);
    }

    return null;
  };

  /**
   * Parse network/registry patterns.
   */
  private readonly parseNetworkPatterns = (
    trimmed: string,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult => {
    const networkMatch = npmNetworkErrorPattern.exec(trimmed);
    if (networkMatch) {
      return this.buildNetworkError(networkMatch, rawLine, ctx);
    }

    const registryMatch = registryRequestFailedPattern.exec(trimmed);
    if (registryMatch) {
      return this.buildRegistryRequestFailedError(registryMatch, rawLine, ctx);
    }

    const dnsMatch = dnsResolutionErrorPattern.exec(trimmed);
    if (dnsMatch) {
      return this.buildDnsResolutionError(dnsMatch, rawLine, ctx);
    }

    return null;
  };

  /**
   * Parse disk/resource patterns.
   * Note: npmEnoentPattern is checked here before the generic npm pattern in parseCorePatterns.
   */
  private readonly parseResourcePatterns = (
    trimmed: string,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult => {
    // Check npm ENOENT first (more specific than generic npm error)
    if (npmEnoentPattern.test(trimmed)) {
      return this.buildEnoentError(rawLine, ctx);
    }

    if (noSpaceLeftPattern.test(trimmed)) {
      return this.buildNoSpaceLeftError(rawLine, ctx);
    }

    if (outOfMemoryPattern.test(trimmed)) {
      return this.buildOutOfMemoryError(rawLine, ctx);
    }

    return null;
  };

  /**
   * Parse authentication patterns.
   */
  private readonly parseAuthPatterns = (
    trimmed: string,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult => {
    if (npmUnauthorizedPattern.test(trimmed)) {
      return this.buildUnauthorizedError(rawLine, ctx);
    }

    if (npmForbiddenPattern.test(trimmed)) {
      return this.buildForbiddenError(rawLine, ctx);
    }

    if (authRequiredPattern.test(trimmed)) {
      return this.buildAuthRequiredError(rawLine, ctx);
    }

    return null;
  };

  /**
   * Check if the line is infrastructure-specific noise.
   */
  isNoise = (line: string): boolean => {
    const stripped = stripAnsi(line);
    const lowerTrimmed = stripped.trim().toLowerCase();
    return matchesNoisePattern(stripped, lowerTrimmed);
  };

  /**
   * Returns noise patterns for registry-level optimization.
   */
  noisePatterns = (): NoisePatterns => ({
    fastPrefixes: NOISE_FAST_PREFIXES,
    fastContains: NOISE_FAST_CONTAINS,
    regex: NOISE_REGEX_PATTERNS,
  });

  // ============================================================================
  // Private Error Builders
  // ============================================================================

  private readonly buildScriptError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const scriptName = match[1] ?? "";
    const exitCode = Number.parseInt(match[2] ?? "1", 10);
    const exitInfo = classifyExitCode(exitCode);

    const err: MutableExtractedError = {
      message: `Script "${scriptName}" failed with exit code ${exitCode}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: `exit-${exitCode}`,
      suggestions: exitInfo.hint ? [exitInfo.hint] : undefined,
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildCommandError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const commandName = match[1] ?? "";
    const exitCode = Number.parseInt(match[2] ?? "1", 10);
    const exitInfo = classifyExitCode(exitCode);

    const err: MutableExtractedError = {
      message: `Command "${commandName}" failed with exit code ${exitCode}: ${exitInfo.message}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: `exit-${exitCode}`,
      suggestions: exitInfo.hint ? [exitInfo.hint] : undefined,
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildContextCanceledError = (
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: "Operation canceled (context canceled)",
      severity: "error",
      raw: rawLine,
      category: "metadata",
      source: "infrastructure",
      suggestions: [
        "The operation was canceled, possibly due to timeout or user interruption",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildShellNotFoundError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const commandName = match[1] ?? "";
    const exitInfo = classifyExitCode(127);

    const err: MutableExtractedError = {
      message: `Command not found: ${commandName}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "exit-127",
      suggestions: exitInfo.hint
        ? [exitInfo.hint, `Ensure "${commandName}" is installed and in PATH`]
        : [`Ensure "${commandName}" is installed and in PATH`],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildPermissionDeniedError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const filePath = match[1] ?? "";
    const exitInfo = classifyExitCode(126);

    const err: MutableExtractedError = {
      message: `Permission denied: ${filePath}`,
      file: filePath,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "exit-126",
      suggestions: exitInfo.hint
        ? [exitInfo.hint]
        : [`Run chmod +x ${filePath} to make it executable`],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildNpmError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const errorCode = match[1] ?? "";

    const err: MutableExtractedError = {
      message: `npm error: ${errorCode}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: `npm-${errorCode}`,
      suggestions:
        errorCode === "ELIFECYCLE"
          ? ["Check the script output above for the actual error"]
          : undefined,
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildDockerError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const errorMessage = match[1]?.trim() ?? "";

    const err: MutableExtractedError = {
      message: `Docker daemon error: ${errorMessage}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "docker-daemon",
      suggestions: ["Check Docker daemon status and image availability"],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildGitFatalError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const errorMessage = match[1]?.trim() ?? "";

    const err: MutableExtractedError = {
      message: `Git fatal error: ${errorMessage}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "git-fatal",
      suggestions: ["Check repository state and git configuration"],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  // ============================================================================
  // New Error Builders
  // ============================================================================

  private readonly buildNodeVersionError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const currentVersion = match[1] ?? "";
    const requiredVersion = match[2] ?? "";

    const err: MutableExtractedError = {
      message: `Node.js version mismatch: running ${currentVersion}, requires ${requiredVersion}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "node-version",
      suggestions: [
        `Update Node.js to ${requiredVersion}`,
        "Consider using nvm or fnm to manage Node.js versions",
        "Check the engines field in package.json",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildNextjsNodeVersionError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const currentVersion = match[1] ?? "";
    const framework = match[2] ?? "the framework";
    const requiredVersion = match[3] ?? "";

    const err: MutableExtractedError = {
      message: `Node.js version mismatch: running ${currentVersion}, ${framework} requires ${requiredVersion}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "node-version",
      suggestions: [
        `Update Node.js to version ${requiredVersion}`,
        "Use actions/setup-node@v4 with node-version: 20 in CI",
        "Consider using nvm or fnm to manage Node.js versions locally",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildEngineIncompatibleError = (
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: "Node.js engine incompatible with this module",
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "node-engine-incompatible",
      suggestions: [
        "Check the engines field in package.json for required Node.js version",
        "Update Node.js to a compatible version",
        "Use --ignore-engines flag as a workaround (not recommended)",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildMissingScriptError = (
    scriptName: string,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: `Missing script: "${scriptName}"`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "missing-script",
      suggestions: [
        `Add "${scriptName}" script to package.json`,
        "Check for typos in the script name",
        "Ensure you're in the correct directory",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildShNotFoundError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const commandName = match[1] ?? "";
    const exitInfo = classifyExitCode(127);

    const err: MutableExtractedError = {
      message: `Command not found: ${commandName}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "exit-127",
      suggestions: exitInfo.hint
        ? [exitInfo.hint, `Install "${commandName}" or add it to PATH`]
        : [`Install "${commandName}" or add it to PATH`],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildNetworkError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const errorCode = match[1] ?? "";

    const hints: Record<string, string> = {
      ETIMEDOUT:
        "Request timed out - check network connectivity or increase timeout",
      ECONNREFUSED: "Connection refused - check if the registry is accessible",
      ECONNRESET:
        "Connection reset - retry the operation or check network stability",
      ENETUNREACH: "Network unreachable - check internet connectivity",
    };

    const err: MutableExtractedError = {
      message: `Network error: ${errorCode}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: `npm-${errorCode}`,
      suggestions: [
        hints[errorCode] ?? "Check network connectivity",
        "Retry the operation after a short delay",
        "Check if using a proxy or VPN that may be blocking connections",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildRegistryRequestFailedError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const url = match[1] ?? "";

    const err: MutableExtractedError = {
      message: `Registry request failed: ${url}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "registry-request-failed",
      suggestions: [
        "Check if the registry is accessible",
        "Verify your network connection",
        "Check if you need to authenticate with the registry",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildDnsResolutionError = (
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const hostname = match[1] ?? "";

    const err: MutableExtractedError = {
      message: `DNS resolution failed: ${hostname}`,
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "dns-enotfound",
      suggestions: [
        "Check your internet connection",
        "Verify DNS settings",
        `Ensure "${hostname}" is a valid hostname`,
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildNoSpaceLeftError = (
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: "No space left on device",
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "enospc",
      suggestions: [
        "Free up disk space",
        "Clear npm/yarn cache: npm cache clean --force",
        "Remove node_modules and reinstall",
        "Check for large log files or temporary files",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildOutOfMemoryError = (
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: "Cannot allocate memory",
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "enomem",
      suggestions: [
        "Increase available memory or reduce memory usage",
        "Try setting NODE_OPTIONS=--max-old-space-size=4096",
        "Close other applications to free memory",
        "Consider using swap space if available",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildEnoentError = (
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: "File or directory not found (ENOENT)",
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "npm-ENOENT",
      suggestions: [
        "Check if the file or directory exists",
        "Run npm install to restore dependencies",
        "Verify the path is correct",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildUnauthorizedError = (
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: "Authentication failed: 401 Unauthorized",
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "npm-401",
      suggestions: [
        "Check your npm authentication token",
        "Run npm login to authenticate",
        "Verify your credentials are correct",
        "Check if the token has expired",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildForbiddenError = (
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: "Access denied: 403 Forbidden",
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "npm-403",
      suggestions: [
        "Check if you have permission to access this package",
        "Verify your npm organization membership",
        "Contact the package maintainer for access",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly buildAuthRequiredError = (
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError => {
    const err: MutableExtractedError = {
      message: "Authentication required",
      severity: "error",
      raw: rawLine,
      category: "infrastructure",
      source: "infrastructure",
      ruleId: "auth-required",
      suggestions: [
        "Provide valid authentication credentials",
        "Check environment variables for tokens",
        "Verify CI/CD secrets are configured correctly",
      ],
    };

    applyWorkflowContext(err, ctx);
    return err;
  };
}

/**
 * Factory function to create an InfrastructureParser instance.
 */
export const createInfrastructureParser = (): InfrastructureParser =>
  new InfrastructureParser();
