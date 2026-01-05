/**
 * Code snippet extraction for error context.
 * Migrated from packages/core/errors/snippet.go
 */

import { lstat, open, realpath, stat } from "node:fs/promises";
import { basename, extname, isAbsolute, join, normalize, sep } from "node:path";
import { createInterface } from "node:readline";
import type { CodeSnippet, ExtractedError } from "./types.js";

// ============================================================================
// Constants
// ============================================================================

/** Default number of context lines before and after error (total: 7 lines) */
export const DefaultContextLines = 3;

/** Maximum length for a single line before truncation */
export const MaxLineLength = 500;

/** Maximum total snippet size in bytes */
export const MaxSnippetSize = 2048;

/** Skip files larger than 1MB */
export const MaxFileSize = 1024 * 1024;

// ============================================================================
// Sensitive File Patterns
// ============================================================================

/**
 * File patterns that should never be read for snippets
 * to prevent information disclosure of secrets and credentials.
 */
const sensitiveFilePatterns = new Set([
  ".env",
  ".env.local",
  ".env.development",
  ".env.production",
  ".env.test",
  "credentials.json",
  "secrets.json",
  "secrets.yaml",
  "secrets.yml",
  ".netrc",
  ".npmrc",
  ".pypirc",
  "id_rsa",
  "id_ed25519",
  "id_ecdsa",
  "id_dsa",
  ".pem",
  ".key",
  ".p12",
  ".pfx",
  "htpasswd",
  "shadow",
  "passwd",
  // Additional sensitive patterns
  ".aws",
  ".ssh",
  ".gnupg",
  ".pgpass",
  "kubeconfig",
  ".kube",
  "token",
  "token.json",
  ".git-credentials",
  ".docker",
  "service-account.json",
  "gcloud",
]);

/**
 * File extensions that are always sensitive.
 */
const sensitiveExtensions = new Set([
  ".pem",
  ".key",
  ".p12",
  ".pfx",
  ".keystore",
  ".jks",
  ".pub", // Could contain private key material if misnamed
]);

/**
 * Path segments that indicate sensitive directories.
 */
const sensitivePathSegments = new Set([
  ".ssh",
  ".gnupg",
  ".aws",
  ".kube",
  ".docker",
  "secrets",
  "credentials",
  "private",
]);

// ============================================================================
// Language Detection
// ============================================================================

/**
 * Map of file extensions to language identifiers.
 */
const extensionToLanguage: Readonly<Record<string, string>> = {
  ".go": "go",
  ".ts": "typescript",
  ".tsx": "typescript",
  ".js": "javascript",
  ".jsx": "javascript",
  ".mjs": "javascript",
  ".cjs": "javascript",
  ".mts": "typescript",
  ".cts": "typescript",
  ".py": "python",
  ".pyi": "python",
  ".pyw": "python",
  ".rs": "rust",
  ".rb": "ruby",
  ".java": "java",
  ".kt": "kotlin",
  ".kts": "kotlin",
  ".swift": "swift",
  ".c": "c",
  ".h": "c",
  ".cpp": "cpp",
  ".hpp": "cpp",
  ".cc": "cpp",
  ".cxx": "cpp",
  ".cs": "csharp",
  ".php": "php",
  ".vue": "vue",
  ".svelte": "svelte",
  ".astro": "astro",
  ".json": "json",
  ".yaml": "yaml",
  ".yml": "yaml",
  ".toml": "toml",
  ".md": "markdown",
  ".sql": "sql",
  ".sh": "shell",
  ".bash": "shell",
  ".zsh": "shell",
};

/**
 * Detect the programming language from a file extension.
 */
const detectLanguage = (filePath: string): string => {
  const ext = extname(filePath).toLowerCase();
  return extensionToLanguage[ext] ?? "text";
};

// ============================================================================
// Security Checks
// ============================================================================

/**
 * Check if a file path matches known sensitive file patterns.
 */
const isSensitiveFile = (filePath: string): boolean => {
  const base = basename(filePath);
  const lowerBase = base.toLowerCase();

  // Check exact filename matches
  if (sensitiveFilePatterns.has(base)) {
    return true;
  }

  // Check for .env prefix variants (e.g., .env.staging, .env.custom)
  if (lowerBase.startsWith(".env")) {
    return true;
  }

  // Check file extension for sensitive types
  const ext = extname(base).toLowerCase();
  if (sensitiveExtensions.has(ext)) {
    return true;
  }

  // Check path segments for sensitive directories
  const segments = filePath.split(sep);
  for (const segment of segments) {
    if (sensitivePathSegments.has(segment.toLowerCase())) {
      return true;
    }
  }

  return false;
};

/**
 * Check if a path traverses outside the base path (directory traversal attack).
 */
const isPathTraversal = (filePath: string, basePath: string): boolean => {
  const cleanFilePath = normalize(filePath);
  const cleanBasePath = normalize(basePath);

  // Path must start with basePath + separator, or be exactly basePath
  return (
    !cleanFilePath.startsWith(`${cleanBasePath}${sep}`) &&
    cleanFilePath !== cleanBasePath
  );
};

// ============================================================================
// Binary Detection
// ============================================================================

/**
 * Check if a line appears to be from a binary file.
 */
const isBinaryLine = (line: string): boolean => {
  if (line.length === 0) {
    return false;
  }

  // Check for null bytes (definite binary indicator)
  if (line.includes("\0")) {
    return true;
  }

  // Check ratio of non-printable characters
  let nonPrintable = 0;
  let total = 0;

  for (const char of line) {
    total++;
    const code = char.charCodeAt(0);
    // Non-printable chars except tab, newline, carriage return
    if (code < 32 && code !== 9 && code !== 10 && code !== 13) {
      nonPrintable++;
    }
  }

  // More than 10% non-printable is likely binary
  return total > 0 && nonPrintable / total > 0.1;
};

// ============================================================================
// UTF-8 Safe Truncation
// ============================================================================

/** Suffix added to truncated lines */
const TruncationSuffix = "...";

/**
 * Safely truncate a string to maxBytes without breaking UTF-8 sequences.
 * The suffix is NOT included in the returned string - caller must add it.
 */
const truncateUTF8 = (str: string, maxBytes: number): string => {
  if (Buffer.byteLength(str, "utf8") <= maxBytes) {
    return str;
  }

  // Convert to buffer, truncate, and find last valid UTF-8 boundary
  const buffer = Buffer.from(str, "utf8");
  let end = Math.min(maxBytes, buffer.length);

  // Walk back to find a valid UTF-8 start byte
  let byte = buffer[end];
  while (end > 0 && byte !== undefined && (byte & 0xc0) === 0x80) {
    end--;
    byte = buffer[end];
  }

  return buffer.subarray(0, end).toString("utf8");
};

// ============================================================================
// Snippet Extraction
// ============================================================================

/**
 * Extract a code snippet from a file with context lines around the error.
 *
 * Returns null if:
 * - filePath is empty
 * - line <= 0 (unknown location)
 * - file doesn't exist or can't be read
 * - file is larger than MaxFileSize
 * - file appears to be binary
 * - file is a symlink (security: prevents symlink attacks)
 * - file matches sensitive file patterns (security: prevents credential disclosure)
 */
export const extractSnippet = async (
  filePath: string,
  line: number,
  contextLines: number = DefaultContextLines
): Promise<CodeSnippet | null> => {
  // Validate inputs
  if (!filePath || line <= 0 || contextLines < 0) {
    return null;
  }

  // Security: Normalize the path
  const cleanPath = normalize(filePath);

  // Security: Check for sensitive file patterns
  if (isSensitiveFile(cleanPath)) {
    return null;
  }

  try {
    // Security: Use lstat first to detect symlinks
    const lstats = await lstat(cleanPath);

    // Security: Reject symlinks to prevent symlink attacks
    if (lstats.isSymbolicLink()) {
      return null;
    }

    // Reject directories
    if (lstats.isDirectory()) {
      return null;
    }

    // Reject files larger than MaxFileSize
    if (lstats.size > MaxFileSize) {
      return null;
    }

    // Open file
    const fileHandle = await open(cleanPath, "r");

    try {
      // Security: Verify the opened file matches what we stat'd
      const openedStats = await fileHandle.stat();

      // Check size hasn't changed (defense in depth)
      if (openedStats.size > MaxFileSize) {
        return null;
      }

      // Verify same file by checking inode and device
      if (lstats.ino !== openedStats.ino || lstats.dev !== openedStats.dev) {
        return null;
      }

      // Read lines around the error
      const startLine = Math.max(1, line - contextLines);
      const endLine = line + contextLines;

      const lines: string[] = [];
      let currentLine = 0;
      let totalSize = 0;

      const stream = fileHandle.createReadStream({ encoding: "utf8" });
      const rl = createInterface({
        input: stream,
        crlfDelay: Number.POSITIVE_INFINITY,
      });

      for await (const lineText of rl) {
        currentLine++;

        // Skip lines before our window
        if (currentLine < startLine) {
          continue;
        }

        // Stop if we've passed our window
        if (currentLine > endLine) {
          break;
        }

        // Check for binary content
        if (isBinaryLine(lineText)) {
          return null;
        }

        // Truncate long lines (account for suffix in byte limit)
        let processedLine = lineText;
        if (processedLine.length > MaxLineLength) {
          const suffixBytes = Buffer.byteLength(TruncationSuffix, "utf8");
          processedLine = `${truncateUTF8(processedLine, MaxLineLength - suffixBytes)}${TruncationSuffix}`;
        }

        // Check total size limit
        totalSize += processedLine.length + 1; // +1 for conceptual newline
        if (totalSize > MaxSnippetSize) {
          break;
        }

        lines.push(processedLine);
      }

      // Ensure we have at least one line
      if (lines.length === 0) {
        return null;
      }

      // Calculate error line position within snippet (1-indexed)
      let errorLineInSnippet = line - startLine + 1;
      if (errorLineInSnippet < 1) {
        errorLineInSnippet = 1;
      }
      if (errorLineInSnippet > lines.length) {
        errorLineInSnippet = lines.length;
      }

      return {
        lines,
        startLine,
        errorLine: errorLineInSnippet,
        language: detectLanguage(filePath),
      };
    } finally {
      await fileHandle.close();
    }
  } catch {
    // File doesn't exist or can't be read
    return null;
  }
};

// ============================================================================
// Batch Snippet Extraction
// ============================================================================

interface ErrorWithPath {
  err: ExtractedError;
  filePath: string;
  mutableErr: {
    codeSnippet?: CodeSnippet;
  };
}

/**
 * Extract snippets for multiple errors in a single file (batch read).
 * This is more efficient than reading the file multiple times.
 */
const extractSnippetsBatched = async (
  filePath: string,
  errorsInFile: ErrorWithPath[]
): Promise<{ succeeded: number; failed: number }> => {
  // Security: Normalize the path
  const cleanPath = normalize(filePath);

  // Security: Check for sensitive file patterns
  if (isSensitiveFile(cleanPath)) {
    return { succeeded: 0, failed: errorsInFile.length };
  }

  try {
    // Security: Use lstat first to detect symlinks
    const lstats = await lstat(cleanPath);

    // Security: Reject symlinks
    if (lstats.isSymbolicLink()) {
      return { succeeded: 0, failed: errorsInFile.length };
    }

    // Reject directories
    if (lstats.isDirectory()) {
      return { succeeded: 0, failed: errorsInFile.length };
    }

    // Reject files larger than MaxFileSize
    if (lstats.size > MaxFileSize) {
      return { succeeded: 0, failed: errorsInFile.length };
    }

    // Open file
    const fileHandle = await open(cleanPath, "r");

    try {
      // Security: Verify the opened file matches
      const openedStats = await fileHandle.stat();

      if (openedStats.size > MaxFileSize) {
        return { succeeded: 0, failed: errorsInFile.length };
      }

      if (lstats.ino !== openedStats.ino || lstats.dev !== openedStats.dev) {
        return { succeeded: 0, failed: errorsInFile.length };
      }

      // Sort errors by line number for efficient single-pass reading
      errorsInFile.sort((a, b) => (a.err.line ?? 0) - (b.err.line ?? 0));

      // Calculate the overall range we need to read
      const minLine = Math.max(
        1,
        (errorsInFile[0]?.err.line ?? 1) - DefaultContextLines
      );
      const maxLine =
        (errorsInFile.at(-1)?.err.line ?? 1) + DefaultContextLines;

      // Read all lines in the range into memory
      const lineCache = new Map<number, string>();
      let currentLine = 0;
      let isBinary = false;

      const stream = fileHandle.createReadStream({ encoding: "utf8" });
      const rl = createInterface({
        input: stream,
        crlfDelay: Number.POSITIVE_INFINITY,
      });

      for await (const lineText of rl) {
        currentLine++;

        // Skip lines before our window
        if (currentLine < minLine) {
          continue;
        }

        // Stop if we've passed our window
        if (currentLine > maxLine) {
          break;
        }

        // Check for binary content
        if (isBinaryLine(lineText)) {
          isBinary = true;
          break;
        }

        // Truncate long lines (account for suffix in byte limit)
        let processedLine = lineText;
        if (processedLine.length > MaxLineLength) {
          const suffixBytes = Buffer.byteLength(TruncationSuffix, "utf8");
          processedLine = `${truncateUTF8(processedLine, MaxLineLength - suffixBytes)}${TruncationSuffix}`;
        }

        lineCache.set(currentLine, processedLine);
      }

      if (isBinary) {
        return { succeeded: 0, failed: errorsInFile.length };
      }

      // Detect language once for the file
      const language = detectLanguage(filePath);

      // Extract snippets for each error using the cached lines
      let succeeded = 0;
      let failed = 0;

      for (const ewp of errorsInFile) {
        const errLine = ewp.err.line ?? 0;
        const startLine = Math.max(1, errLine - DefaultContextLines);
        const endLine = errLine + DefaultContextLines;

        // Collect lines for this snippet
        const lines: string[] = [];
        let totalSize = 0;

        for (let l = startLine; l <= endLine; l++) {
          const cachedLine = lineCache.get(l);
          if (cachedLine === undefined) {
            continue;
          }
          totalSize += cachedLine.length + 1;
          if (totalSize > MaxSnippetSize) {
            break;
          }
          lines.push(cachedLine);
        }

        if (lines.length === 0) {
          failed++;
          continue;
        }

        // Calculate error line position within snippet
        let errorLineInSnippet = errLine - startLine + 1;
        if (errorLineInSnippet < 1) {
          errorLineInSnippet = 1;
        }
        if (errorLineInSnippet > lines.length) {
          errorLineInSnippet = lines.length;
        }

        ewp.mutableErr.codeSnippet = {
          lines,
          startLine,
          errorLine: errorLineInSnippet,
          language,
        };
        succeeded++;
      }

      return { succeeded, failed };
    } finally {
      await fileHandle.close();
    }
  } catch {
    return { succeeded: 0, failed: errorsInFile.length };
  }
};

/**
 * Add code snippets to all errors that have valid file+line.
 * Returns counts of successes and failures for AIContext metrics.
 *
 * Security: When basePath is provided, paths are validated to prevent directory traversal attacks.
 * Performance: Batches file reads so each file is only read once, even with multiple errors.
 *
 * Note: This function returns new error objects with snippets attached rather than
 * mutating the input errors, preserving immutability.
 */
export const extractSnippetsForErrors = async (
  errors: readonly ExtractedError[],
  basePath?: string
): Promise<{
  errors: ExtractedError[];
  succeeded: number;
  failed: number;
}> => {
  // Security: Clean and resolve basePath once if provided
  let cleanBasePath: string | undefined;
  if (basePath) {
    try {
      // Ensure basePath is absolute
      if (!isAbsolute(basePath)) {
        return {
          errors: [...errors],
          succeeded: 0,
          failed: errors.length,
        };
      }

      // Security: Resolve symlinks in basePath to prevent symlink-based traversal
      // This ensures the canonical path is used for all comparisons
      cleanBasePath = await realpath(basePath);

      // Verify basePath exists and is a directory
      const baseStats = await stat(cleanBasePath);
      if (!baseStats.isDirectory()) {
        return {
          errors: [...errors],
          succeeded: 0,
          failed: errors.length,
        };
      }
    } catch {
      // If we can't resolve basePath, fail safely
      return {
        errors: [...errors],
        succeeded: 0,
        failed: errors.length,
      };
    }
  }

  // Group errors by resolved file path for batched reading
  const fileGroups = new Map<string, ErrorWithPath[]>();
  const resultErrors: ExtractedError[] = [];
  // Use Map for O(1) lookup instead of array.find() which is O(n)
  const mutableRefs = new Map<number, { codeSnippet?: CodeSnippet }>();

  for (let i = 0; i < errors.length; i++) {
    const err = errors[i];
    if (!err) {
      continue;
    }

    // Skip if no file or no valid line number
    if (!(err.file && err.line) || err.line <= 0) {
      resultErrors.push(err);
      continue;
    }

    // Resolve file path
    let filePath = err.file;
    if (cleanBasePath && !isAbsolute(filePath)) {
      // Security: Join paths and then verify the result is still under basePath
      filePath = join(cleanBasePath, filePath);

      // Security: Check for path traversal
      if (isPathTraversal(filePath, cleanBasePath)) {
        resultErrors.push(err);
        continue;
      }
      filePath = normalize(filePath);
    }

    // Create a mutable reference for this error
    const mutableErr: { codeSnippet?: CodeSnippet } = {};
    mutableRefs.set(i, mutableErr);

    const existing = fileGroups.get(filePath);
    if (existing) {
      existing.push({ err, filePath, mutableErr });
    } else {
      fileGroups.set(filePath, [{ err, filePath, mutableErr }]);
    }

    resultErrors.push(err);
  }

  let totalSucceeded = 0;
  let totalFailed = 0;

  // Process each file
  for (const [filePath, errorsInFile] of fileGroups) {
    // For single error, use the simple extraction path
    if (errorsInFile.length === 1) {
      const ewp = errorsInFile[0];
      if (ewp) {
        const snippet = await extractSnippet(filePath, ewp.err.line ?? 0);
        if (snippet) {
          ewp.mutableErr.codeSnippet = snippet;
          totalSucceeded++;
        } else {
          totalFailed++;
        }
      }
      continue;
    }

    // Multiple errors in same file - batch read
    const { succeeded, failed } = await extractSnippetsBatched(
      filePath,
      errorsInFile
    );
    totalSucceeded += succeeded;
    totalFailed += failed;
  }

  // Reconstruct errors with snippets attached (O(n) with Map lookup)
  const finalErrors = resultErrors.map((err, i) => {
    const ref = mutableRefs.get(i);
    if (ref?.codeSnippet) {
      return { ...err, codeSnippet: ref.codeSnippet };
    }
    return err;
  });

  return {
    errors: finalErrors,
    succeeded: totalSucceeded,
    failed: totalFailed,
  };
};
