/**
 * Parser utilities for error extraction.
 * Contains shared helpers used across parsers.
 */

// ============================================================================
// ANSI Escape Code Handling
// ============================================================================

/**
 * Pattern matching ANSI escape sequences for terminal output.
 * Covers:
 * - CSI sequences: ESC[ followed by parameters and a final byte (0x40-0x7E)
 *   Examples: \x1b[0m (reset), \x1b[31m (red), \x1b[2J (clear screen), \x1b[H (cursor home)
 * - OSC sequences: ESC] ... (BEL | ESC\)
 *   Examples: \x1b]0;title\x07 (set window title)
 * - Simple escape sequences: ESC followed by single char
 *   Examples: \x1b7 (save cursor), \x1b8 (restore cursor)
 *
 * ReDoS safety: Uses possessive-like character classes [^\x07\x1b]* with bounded alternatives
 * to prevent catastrophic backtracking. The OSC pattern matches non-terminator chars then the terminator.
 */
// biome-ignore lint/suspicious/noControlCharactersInRegex: intentional ANSI escape sequence matching
const ansiEscapePattern =
  /\x1b\[[0-9;:?]*[A-Za-z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?|\x1b[()][AB012]|\x1b[@-_]/g;

/**
 * Remove ANSI escape sequences from a string.
 * Used to clean up colored CLI output before parsing error patterns.
 * CI tools like golangci-lint, cargo, tsc, and eslint may output colored text.
 */
export const stripAnsi = (s: string): string =>
  s.replace(ansiEscapePattern, "");

// ============================================================================
// File Extension Handling
// ============================================================================

/**
 * Maps file extensions to parser IDs for fast-path lookup.
 * Enables O(1) parser selection when a line contains a file path with known extension.
 */
export const extensionToParserID: Readonly<Record<string, string>> = {
  // Go
  ".go": "go",

  // TypeScript / JavaScript
  ".ts": "typescript",
  ".tsx": "typescript",
  ".mts": "typescript", // ES module TypeScript
  ".cts": "typescript", // CommonJS TypeScript
  ".js": "eslint", // JS errors more likely from linter than tsc
  ".jsx": "eslint",
  ".mjs": "eslint",
  ".cjs": "eslint",

  // Framework-specific (use eslint as they're JS/TS based)
  ".vue": "eslint", // Vue single-file components
  ".svelte": "eslint", // Svelte components
  ".astro": "eslint", // Astro components

  // Rust
  ".rs": "rust",
  ".toml": "rust", // Cargo.toml errors

  // Python
  ".py": "python",
  ".pyi": "python", // Type stubs
  ".pyw": "python", // Windows Python
};

/** Set of known extensions for O(1) lookup */
const knownExtensions = new Set(Object.keys(extensionToParserID));

/**
 * Check if a character code is valid for a file extension (a-z, A-Z, 0-9).
 * Uses char codes to avoid string allocation overhead.
 */
const isExtCharCode = (code: number): boolean =>
  (code >= 97 && code <= 122) || // a-z
  (code >= 65 && code <= 90) || // A-Z
  (code >= 48 && code <= 57); // 0-9

/**
 * Extract a file extension from a line containing a file path.
 * Returns the extension (e.g., ".go", ".ts") or empty string if none found.
 * Looks for common error format patterns: file.ext:line:col, file.ext(line,col), etc.
 */
export const extractFileExtension = (line: string): string => {
  // Common patterns for file paths in error messages:
  // - path/file.go:10:5: message
  // - path/file.ts(10,5): message
  // - path/file.rs:10: message
  // - /absolute/path/file.go:10:5: message

  const len = line.length;

  // Find potential file path by looking for extension followed by : or (
  for (let i = 0; i < len; i++) {
    if (line.charCodeAt(i) === 46) {
      // '.' = 46
      // Found a dot, extract potential extension
      let extEnd = i + 1;
      while (extEnd < len && isExtCharCode(line.charCodeAt(extEnd))) {
        extEnd++;
      }
      // Check if followed by : or ( (common in error formats)
      const nextChar = line.charCodeAt(extEnd);
      if (nextChar === 58 || nextChar === 40) {
        // ':' = 58, '(' = 40
        const ext = line.slice(i, extEnd).toLowerCase();
        if (knownExtensions.has(ext)) {
          return ext;
        }
      }
    }
  }
  return "";
};

// ============================================================================
// Line/Column Extraction Helpers
// ============================================================================

/**
 * Safely parse an integer, returning undefined if not valid.
 * Validates the result is within JavaScript's safe integer range
 * and is non-negative (line/column numbers are always positive).
 */
export const safeParseInt = (s: string | undefined): number | undefined => {
  if (!s) {
    return undefined;
  }
  const n = Number.parseInt(s, 10);
  if (Number.isNaN(n) || n < 0 || n > Number.MAX_SAFE_INTEGER) {
    return undefined;
  }
  return n;
};

/**
 * Common error line patterns for extracting file:line:col.
 * These patterns match formats like:
 * - file.go:10:5: message (Go, golangci-lint)
 * - file.ts(10,5): message (TypeScript)
 * - file.py:10: message (Python)
 */
export const patterns = {
  /** Go-style: file.go:10:5: message */
  goStyle: /^(.+?):(\d+):(\d+):\s*(.*)$/,

  /** TypeScript-style: file.ts(10,5): error TS1234: message */
  tsStyle: /^(.+?)\((\d+),(\d+)\):\s*(.*)$/,

  /** Simple line-only: file.py:10: message */
  lineOnly: /^(.+?):(\d+):\s*(.*)$/,

  /** ESLint format: file:line:col warning/error rule: message */
  eslint: /^(.+?):(\d+):(\d+)\s+(warning|error)\s+(.*)$/i,

  /** Location patterns for parseLocation */
  colonLocation: /^(\d+):(\d+)$/,
  commaLocation: /^(\d+),(\d+)$/,
  lineOnlyLocation: /^(\d+)$/,
} as const;

/**
 * Parse a location string like "10:5" or "(10,5)" into line and column.
 * Returns [line, column] or [undefined, undefined] if not parseable.
 * Uses safeParseInt for overflow/negative number validation.
 */
export const parseLocation = (
  loc: string
): [number | undefined, number | undefined] => {
  // Early return for empty/whitespace-only input
  if (!loc || loc.trim().length === 0) {
    return [undefined, undefined];
  }

  // Try colon format: "10:5"
  const colonMatch = patterns.colonLocation.exec(loc);
  if (colonMatch?.[1] && colonMatch[2]) {
    const line = safeParseInt(colonMatch[1]);
    const col = safeParseInt(colonMatch[2]);
    if (line !== undefined && col !== undefined) {
      return [line, col];
    }
  }

  // Try comma format: "10,5"
  const commaMatch = patterns.commaLocation.exec(loc);
  if (commaMatch?.[1] && commaMatch[2]) {
    const line = safeParseInt(commaMatch[1]);
    const col = safeParseInt(commaMatch[2]);
    if (line !== undefined && col !== undefined) {
      return [line, col];
    }
  }

  // Try line-only: "10"
  const lineMatch = patterns.lineOnlyLocation.exec(loc);
  if (lineMatch?.[1]) {
    const line = safeParseInt(lineMatch[1]);
    if (line !== undefined) {
      return [line, undefined];
    }
  }

  return [undefined, undefined];
};

// ============================================================================
// Command Splitting
// ============================================================================

/**
 * Split a command line by common shell operators (&&, ||, ;, |).
 * Handles quoted strings and escape sequences properly.
 */
// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: migrated from Go, shell parsing inherently complex
export const splitCommands = (line: string): string[] => {
  const result: string[] = [];
  let current = "";
  let inQuote = false;
  let quoteChar = "";
  const len = line.length;

  for (let i = 0; i < len; i++) {
    const ch = line[i];

    // Handle escape sequences - skip next character
    if (ch === "\\" && i + 1 < len) {
      current += ch;
      current += line[i + 1];
      i++;
      continue;
    }

    // Handle quotes
    if (ch === '"' || ch === "'") {
      if (!inQuote) {
        inQuote = true;
        quoteChar = ch;
      } else if (ch === quoteChar) {
        inQuote = false;
      }
      current += ch;
      continue;
    }

    // Check for command separators only when not in quotes
    if (!inQuote) {
      // Check for && or ||
      if (
        i < len - 1 &&
        ((ch === "&" && line[i + 1] === "&") ||
          (ch === "|" && line[i + 1] === "|"))
      ) {
        if (current.length > 0) {
          result.push(current);
          current = "";
        }
        i++; // Skip next character
        continue;
      }
      // Check for ;
      if (ch === ";") {
        if (current.length > 0) {
          result.push(current);
          current = "";
        }
        continue;
      }
      // Check for | (pipe) - still a command separator
      if (ch === "|") {
        if (current.length > 0) {
          result.push(current);
          current = "";
        }
        continue;
      }
    }

    current += ch;
  }

  if (current.length > 0) {
    result.push(current);
  }

  return result;
};
