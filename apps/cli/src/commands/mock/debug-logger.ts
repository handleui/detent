/**
 * Debug logger for per-run troubleshooting.
 * Stores debug logs in ~/.detent/debug/<run-id>.log
 *
 * Features:
 * - Per-run log files (no overwriting)
 * - Timestamped entries
 * - Act output capture
 * - Automatic log rotation (keeps last 10 logs)
 */

import {
  appendFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
  statSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { getDetentDir } from "../../lib/config.js";

const DEBUG_DIR_NAME = "debug";
const MAX_LOG_FILES = 10;

/**
 * Patterns that may indicate sensitive data in logs.
 * These are redacted before writing to debug files.
 */
const SENSITIVE_PATTERNS = [
  // API keys and tokens (common formats)
  /(?:api[_-]?key|apikey|api[_-]?token|auth[_-]?token|access[_-]?token|bearer)\s*[:=]\s*["']?[\w\-./+=]{20,}["']?/gi,
  // GitHub tokens
  /gh[pousr]_[A-Za-z0-9_]{36,}/g,
  // AWS credentials
  /(?:AKIA|ABIA|ACCA|ASIA)[A-Z0-9]{16}/g,
  /aws[_-]?(?:secret[_-]?)?(?:access[_-]?)?key\s*[:=]\s*["']?[A-Za-z0-9/+=]{40}["']?/gi,
  // Generic secrets with common naming
  /(?:password|passwd|secret|private[_-]?key|client[_-]?secret)\s*[:=]\s*["']?[^\s"']{8,}["']?/gi,
  // Base64-encoded secrets (heuristic: long base64 following secret keywords)
  /(?:secret|token|key|password)\s*[:=]\s*["']?[A-Za-z0-9+/]{50,}={0,2}["']?/gi,
  // npm tokens
  /npm_[A-Za-z0-9]{36}/g,
] as const;

/**
 * Redacts sensitive patterns from a string.
 * Replaces matched patterns with [REDACTED].
 */
const redactSecrets = (text: string): string => {
  let result = text;
  for (const pattern of SENSITIVE_PATTERNS) {
    result = result.replace(pattern, "[REDACTED]");
  }
  return result;
};

/**
 * Per-run debug logger that writes to ~/.detent/debug/<run-id>.log
 */
export class DebugLogger {
  private readonly logPath: string;
  private readonly runID: string;
  private closed = false;
  private readonly phaseStartTimes = new Map<string, number>();

  constructor(runID: string) {
    this.runID = runID;
    this.logPath = this.initializeLogFile(runID);
    this.log("DebugLogger initialized");
  }

  /**
   * Initializes the log file and ensures the debug directory exists.
   * Performs log rotation to keep only the most recent logs.
   */
  private initializeLogFile(runID: string): string {
    const debugDir = join(getDetentDir(), DEBUG_DIR_NAME);

    // Create debug directory if it doesn't exist
    if (!existsSync(debugDir)) {
      mkdirSync(debugDir, { recursive: true, mode: 0o700 });
    }

    // Rotate old logs before creating new one
    this.rotateLogs(debugDir);

    // Create new log file
    const logPath = join(debugDir, `${runID}.log`);
    const timestamp = this.formatTimestamp();
    const header = `${"=".repeat(80)}\nDetent Debug Log\nRun ID: ${runID}\nStarted: ${timestamp}\n${"=".repeat(80)}\n\n`;
    writeFileSync(logPath, header, { mode: 0o600 });

    return logPath;
  }

  /**
   * Rotates log files, keeping only the MAX_LOG_FILES most recent.
   */
  private rotateLogs(debugDir: string): void {
    try {
      if (!existsSync(debugDir)) {
        return;
      }

      // Get all .log files with their modification times
      const logFiles = readdirSync(debugDir)
        .filter((file) => file.endsWith(".log"))
        .map((file) => {
          const filePath = join(debugDir, file);
          const stats = statSync(filePath);
          return {
            path: filePath,
            mtime: stats.mtime.getTime(),
          };
        })
        .sort((a, b) => b.mtime - a.mtime); // Sort by newest first

      // Delete old logs if we exceed the limit
      if (logFiles.length >= MAX_LOG_FILES) {
        const logsToDelete = logFiles.slice(MAX_LOG_FILES - 1); // Keep space for new log
        for (const log of logsToDelete) {
          try {
            unlinkSync(log.path);
          } catch {
            // Ignore errors when deleting old logs
          }
        }
      }
    } catch {
      // Ignore errors during rotation
    }
  }

  /**
   * Formats a timestamp in ISO 8601 format with local timezone offset.
   */
  private formatTimestamp(): string {
    const now = new Date();
    const offset = -now.getTimezoneOffset();
    const offsetHours = String(Math.floor(Math.abs(offset) / 60)).padStart(
      2,
      "0"
    );
    const offsetMinutes = String(Math.abs(offset) % 60).padStart(2, "0");
    const offsetSign = offset >= 0 ? "+" : "-";

    const iso = now.toISOString().slice(0, -1); // Remove trailing 'Z'
    return `${iso}${offsetSign}${offsetHours}:${offsetMinutes}`;
  }

  /**
   * Logs a message with timestamp.
   */
  log(message: string): void {
    if (this.closed) {
      return;
    }

    try {
      const timestamp = this.formatTimestamp();
      const logLine = `${timestamp} ${message}\n`;
      appendFileSync(this.logPath, logLine);
    } catch {
      // Silently fail - we don't want logging to crash the app
    }
  }

  /**
   * Logs a phase transition (e.g., "[Preflight] Starting checks").
   */
  logPhase(phase: string, message: string): void {
    this.log(`[${phase}] ${message}`);
  }

  /**
   * Logs raw act output (stdout/stderr).
   * Sensitive data (API keys, tokens, passwords) is automatically redacted.
   */
  logActOutput(output: string): void {
    if (this.closed) {
      return;
    }

    try {
      // Redact sensitive data before logging
      const sanitized = redactSecrets(output);
      appendFileSync(this.logPath, sanitized);
    } catch {
      // Silently fail
    }
  }

  /**
   * Logs an error with stack trace.
   */
  logError(error: unknown, context?: string): void {
    const prefix = context ? `[${context}] ` : "";

    if (error instanceof Error) {
      this.log(`${prefix}Error: ${error.message}`);
      if (error.stack) {
        this.log(`Stack trace:\n${error.stack}`);
      }
    } else {
      this.log(`${prefix}Error: ${String(error)}`);
    }
  }

  /**
   * Closes the logger and flushes any pending writes.
   */
  close(): void {
    if (this.closed) {
      return;
    }

    this.log("DebugLogger closed");
    this.log(`${"=".repeat(80)}\n`);
    this.closed = true;
  }

  /**
   * Gets the absolute path to the log file.
   */
  get path(): string {
    return this.logPath;
  }

  /**
   * Gets the run ID for this logger.
   */
  get id(): string {
    return this.runID;
  }

  /**
   * Logs configuration information at the start of a run.
   */
  logHeader(config: {
    verbose?: boolean;
    workflow?: string;
    job?: string;
    repoRoot: string;
  }): void {
    this.log("=".repeat(80));
    this.log("Configuration:");
    this.log(`  Verbose: ${config.verbose ?? false}`);
    this.log(`  Workflow: ${config.workflow ?? "all"}`);
    this.log(`  Job: ${config.job ?? "all"}`);
    this.log(`  Repository: ${config.repoRoot}`);
    this.log("=".repeat(80));
    this.log("");
  }

  /**
   * Logs environment information (OS, act version, docker version).
   */
  async logEnvironment(): Promise<void> {
    this.log("Environment:");
    this.log(`  OS: ${process.platform} ${process.arch}`);
    this.log(`  Node: ${process.version}`);

    // Act version
    try {
      const { execFile } = await import("node:child_process");
      const { promisify } = await import("node:util");
      const execFileAsync = promisify(execFile);
      const { stdout } = await execFileAsync("act", ["--version"]);
      this.log(`  Act version: ${stdout.trim()}`);
    } catch {
      this.log("  Act version: unknown");
    }

    // Docker version
    try {
      const { execFile } = await import("node:child_process");
      const { promisify } = await import("node:util");
      const execFileAsync = promisify(execFile);
      const { stdout } = await execFileAsync("docker", ["--version"]);
      this.log(`  Docker version: ${stdout.trim()}`);
    } catch {
      this.log("  Docker version: unknown");
    }

    this.log("");
  }

  /**
   * Logs a section separator with title.
   */
  logSection(title: string): void {
    this.log("");
    this.log("=".repeat(80));
    this.log(title);
    this.log("=".repeat(80));
    this.log("");
  }

  /**
   * Starts timing a phase.
   */
  startPhase(phase: string): void {
    this.phaseStartTimes.set(phase, Date.now());
    this.logPhase(phase, "Starting");
  }

  /**
   * Ends timing a phase and logs the duration.
   */
  endPhase(phase: string): void {
    const start = this.phaseStartTimes.get(phase);
    if (start) {
      const duration = Date.now() - start;
      this.logPhase(phase, `Completed in ${duration}ms`);
      this.phaseStartTimes.delete(phase);
    }
  }
}
