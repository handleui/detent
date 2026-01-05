/**
 * Python error parser for tracebacks, pytest, mypy, ruff, flake8, and pylint.
 * Migrated from packages/core/tools/python/parser.go
 */

import type { NoisePatternProvider, NoisePatterns } from "../parser-types.js";
import {
  applyWorkflowContext,
  MultiLineParser,
  type ParseContext,
  type ParseResult,
} from "../parser-types.js";
import type { MutableExtractedError } from "../types.js";
import { stripAnsi } from "../utils.js";

// ============================================================================
// Constants
// ============================================================================

const MAX_TRACEBACK_FRAMES = 100;
const MAX_TRACEBACK_BYTES = 256 * 1024; // 256KB
const MAX_MESSAGE_LENGTH = 2000;

// ============================================================================
// Patterns
// ============================================================================

/** Traceback start: "Traceback (most recent call last):" */
const tracebackStartPattern = /^Traceback \(most recent call last\):$/;

/** File location in traceback: '  File "/path/to/file.py", line 42, in function_name' */
const tracebackFilePattern = /^\s+File "([^"]+)", line (\d+)(?:, in (.+))?$/;

/** Code line in traceback (4+ space indented) */
const tracebackCodePattern = /^\s{4,}\S.*$/;

/** Exception line: "ValueError: message here" */
const exceptionPattern =
  /^([A-Z][a-zA-Z0-9]*(?:Error|Exception|Warning)): (.+)$/;

/** Chained exception headers */
const chainedExceptionPattern =
  /^(?:During handling of the above exception|The above exception was the direct cause)/;

/** SyntaxError file line (without function): '  File "script.py", line 5' */
const syntaxErrorFilePattern = /^\s+File "([^"]+)", line (\d+)\s*$/;

/** Caret line for SyntaxError column detection */
const syntaxErrorCaretPattern = /^\s*\^+\s*$/;

/** SyntaxError and related compile errors */
const syntaxErrorPattern = /^(SyntaxError|IndentationError|TabError): (.+)$/;

/** pytest FAILED: "FAILED tests/test_foo.py::test_bar - AssertionError: assert 1 == 2" */
const pytestFailedPattern = /^FAILED\s+([^\s:]+)::(\S+)\s+-\s+(.+)$/;

/** pytest ERROR collection: "ERROR tests/test_foo.py - ModuleNotFoundError: No module named 'foo'" */
const pytestErrorPattern = /^ERROR\s+(\S+)\s+-\s+(.+)$/;

/** mypy output: "app/main.py:42: error: message [rule-id]" */
const mypyPattern = /^([^\s:]+\.pyi?):(\d+): (error|warning|note): (.+)$/;

/** ruff/flake8 with column: "app/main.py:42:10: E501 Line too long" */
const ruffFlake8Pattern = /^([^\s:]+\.pyi?):(\d+):(\d+): ([A-Z]\d+) (.+)$/;

/** ruff/flake8 without column: "app/main.py:42: E501 Line too long" */
const ruffFlake8NoColPattern = /^([^\s:]+\.pyi?):(\d+): ([A-Z]\d+) (.+)$/;

/** pylint: "app/main.py:42:0: C0114: Missing module docstring (missing-module-docstring)" */
const pylintPattern =
  /^([^\s:]+\.pyi?):(\d+):(\d+): ([RCWEF]\d+): (.+) \(([^)]+)\)$/;

// ============================================================================
// Noise Patterns
// ============================================================================

const noisePatterns: readonly RegExp[] = [
  /^\s*$/, // Empty/whitespace lines
  /^\.{4,}$/, // pytest progress dots (4+ to avoid matching ...)
  /^\d+ passed/, // pytest summary
  /^\d+ failed,/, // pytest summary (with comma to be specific)
  /^\d+ errors?$/, // pytest summary (at end of line)
  /^\d+ warnings?$/, // pytest summary (at end of line)
  /^\d+ skipped/, // pytest summary
  /^test session starts/, // pytest header
  /^short test summary info/, // pytest header
  /^warnings summary/, // pytest header
  /^PASSED$/, // pytest passed indicator (exact match)
  /^SKIPPED$/, // pytest skipped indicator (exact match)
  /^platform (linux|darwin|win)/, // pytest platform info
  /^cachedir:/, // pytest cache info
  /^rootdir:/, // pytest root dir
  /^configfile:/, // pytest config
  /^plugins:/, // pytest plugins
  /^collecting/, // pytest collection
  /^collected\s+\d+/, // pytest collected tests
  /^ok\s+\(/, // unittest ok
  /^Ran\s+\d+\s+test/, // unittest summary
  /^OK$/, // unittest pass (exact match)
  /^Success:/, // mypy success
  /^Found\s+\d+\s+errors? in/, // mypy summary
  /^Your code has been rated/, // pylint rating
  /^All checks passed!/, // ruff success
  /^\d+ files? (checked|scanned)/, // ruff/flake8 summary
  /^Coverage/, // coverage output
  /^Name\s+Stmts\s+Miss/, // coverage header
  /^TOTAL\s+/, // coverage total
  /^self = </, // pytest self reference
  /^During handling of the above/, // chained exception header
  /^The above exception was/, // chained exception header
  /^.*\.py::.*PASSED/, // pytest verbose passed
  /^.*\.py::.*SKIPPED/, // pytest verbose skipped
  /^\s+@pytest/, // pytest decorators
  /^\s+@fixture/, // pytest fixtures
  /^\s+@mark/, // pytest marks
  /^=+ warnings summary =+/, // pytest warnings header
  /^=+ short test summary info =+/, // pytest summary header
  /^=+ FAILURES =+/, // pytest failures header
  /^=+ ERRORS =+/, // pytest errors header
  /^_+ .+ _+$/, // pytest test name separators
  /^in \d+\.\d+s$/, // pytest timing (exact end)
  /^Rerun/, // pytest-rerunfailures
  /^E\s+assert\s+/, // pytest assertion detail
  /^E\s+\+/, // pytest diff plus
  /^E\s+-/, // pytest diff minus
  /^E\s+where/, // pytest where clause
  /^E\s+and/, // pytest and clause
  /^\s+\.{3}$/, // traceback continuation
  /^<frozen /, // frozen module paths
  /^\s+File "</, // internal file refs like <stdin>
];

const noiseFastPrefixes: readonly string[] = [
  "collecting",
  "collected ",
  "platform linux",
  "platform darwin",
  "platform win",
  "cachedir:",
  "rootdir:",
  "configfile:",
  "plugins:",
  "ok (",
  "your code has been rated",
  "all checks passed",
];

const noiseFastContains: readonly string[] = [
  "test session starts",
  "short test summary",
  "warnings summary",
  "files checked",
  "files scanned",
  " passed in ",
  " passed,",
];

// ============================================================================
// Severity Mappings
// ============================================================================

type Severity = "error" | "warning";

/**
 * Ruff/flake8 severity by code prefix.
 * Two-character prefixes are checked first for more specific matches.
 * Covers: pyflakes (F), pycodestyle (E/W), flake8-bugbear (B), isort (I),
 * pep8-naming (N), pydocstyle (D), bandit (S), flake8-quotes (Q),
 * flake8-builtins (A), flake8-pytest-style (PT), ruff-specific (RUF),
 * pyupgrade (UP), pylint (PL*), tryceratops (TRY), etc.
 */
const ruffFlake8SeverityCodes: Readonly<Record<string, Severity>> = {
  // Two-char prefixes (checked first for specificity)
  E9: "error", // E9xx: Runtime/syntax errors
  F4: "error", // F4xx: Import-related errors (e.g., F401 unused import)
  F5: "error", // F5xx: Format string errors
  F6: "error", // F6xx: Invalid annotations
  F7: "error", // F7xx: Syntax errors
  F8: "error", // F8xx: Undefined names
  PL: "warning", // PLxx: pylint rules via ruff
  // One-char prefixes (fallback)
  F: "error", // General pyflakes errors
  E: "warning", // E1xx-E5xx: Style issues (pycodestyle)
  W: "warning", // Wxx: Warnings (pycodestyle)
  C: "warning", // Cxx: Complexity/convention (mccabe, flake8-comprehensions)
  N: "warning", // Nxx: Naming conventions (pep8-naming)
  D: "warning", // Dxx: Docstring issues (pydocstyle)
  B: "warning", // Bxx: Bugbear (flake8-bugbear)
  I: "warning", // Ixx: isort
  S: "warning", // Sxx: Security (bandit)
  T: "warning", // Txx: flake8-debugger, flake8-print
  Q: "warning", // Qxx: quotes (flake8-quotes)
  A: "warning", // Axx: builtins (flake8-builtins)
  P: "warning", // Pxx: pytest style
  U: "warning", // UPxxx: pyupgrade
  R: "warning", // RUFxxx: ruff-specific rules
  Y: "warning", // Yxx: flake8-pyi (type stub linting)
};

/**
 * Pylint severity by message type prefix.
 * C=Convention, R=Refactor, W=Warning, E=Error, F=Fatal
 */
const pylintSeverityCodes: Readonly<Record<string, Severity>> = {
  C: "warning", // Convention
  R: "warning", // Refactor
  W: "warning", // Warning
  E: "error", // Error
  F: "error", // Fatal
};

const getRuffFlake8Severity = (code: string): Severity => {
  if (code.length < 1) {
    return "error";
  }

  // Check 2-character prefix first (more specific)
  if (code.length >= 2) {
    const twoChar = ruffFlake8SeverityCodes[code.slice(0, 2)];
    if (twoChar !== undefined) {
      return twoChar;
    }
  }

  // Check 1-character prefix
  const firstChar = code[0];
  if (firstChar !== undefined) {
    const oneChar = ruffFlake8SeverityCodes[firstChar];
    if (oneChar !== undefined) {
      return oneChar;
    }
  }

  return "error";
};

const getPylintSeverity = (code: string): Severity => {
  const firstChar = code[0];
  if (firstChar === undefined) {
    return "error";
  }

  return pylintSeverityCodes[firstChar] ?? "error";
};

// ============================================================================
// Utility Functions
// ============================================================================

const truncateMessage = (msg: string): string => {
  if (msg.length <= MAX_MESSAGE_LENGTH) {
    return msg;
  }
  return msg.slice(0, MAX_MESSAGE_LENGTH);
};

// ============================================================================
// Traceback State
// ============================================================================

interface TracebackState {
  inTraceback: boolean;
  file: string;
  line: number;
  function: string;
  stackTrace: string[];
  /** Running byte count to avoid O(n) join on every line */
  byteCount: number;
  frameCount: number;
  isSyntaxError: boolean;
  column: number;
  codeContext: string;
}

const createTracebackState = (): TracebackState => ({
  inTraceback: false,
  file: "",
  line: 0,
  function: "",
  stackTrace: [],
  byteCount: 0,
  frameCount: 0,
  isSyntaxError: false,
  column: 0,
  codeContext: "",
});

const resetTracebackState = (state: TracebackState): void => {
  state.inTraceback = false;
  state.file = "";
  state.line = 0;
  state.function = "";
  state.stackTrace = [];
  state.byteCount = 0;
  state.frameCount = 0;
  state.isSyntaxError = false;
  state.column = 0;
  state.codeContext = "";
};

// ============================================================================
// Python Parser
// ============================================================================

/**
 * PythonParser handles Python tracebacks, pytest, mypy, ruff, flake8, and pylint output.
 * Implements multi-line parsing for Python tracebacks with exception chains.
 */
export class PythonParser
  extends MultiLineParser
  implements NoisePatternProvider
{
  readonly id = "python";
  readonly priority = 80;

  private readonly traceback: TracebackState = createTracebackState();

  canParse(line: string, _ctx: ParseContext): number {
    const stripped = stripAnsi(line);

    // Check if we're in a multi-line state (fast path)
    if (this.traceback.inTraceback) {
      return 0.9;
    }

    // Fast path: skip for empty lines
    if (stripped === "") {
      return 0;
    }

    // Fast path for traceback start (must begin with "Traceback")
    if (
      stripped.startsWith("Traceback") &&
      tracebackStartPattern.test(stripped)
    ) {
      return 0.95;
    }

    // Fast path for pytest (must begin with "FAILED" or "ERROR")
    if (stripped.startsWith("FAILED") && pytestFailedPattern.test(stripped)) {
      return 0.95;
    }
    if (stripped.startsWith("ERROR") && pytestErrorPattern.test(stripped)) {
      return 0.95;
    }

    // Fast path for exception lines (must contain ": " and start with uppercase)
    const firstChar = stripped[0];
    if (
      firstChar &&
      firstChar >= "A" &&
      firstChar <= "Z" &&
      stripped.includes(": ")
    ) {
      // Check SyntaxError first (subset of exception pattern)
      if (
        (stripped.startsWith("SyntaxError:") ||
          stripped.startsWith("IndentationError:") ||
          stripped.startsWith("TabError:")) &&
        syntaxErrorPattern.test(stripped)
      ) {
        return 0.9;
      }
      // Check general exception pattern
      if (exceptionPattern.test(stripped)) {
        return 0.95;
      }
    }

    // Fast path for .py files (mypy, ruff, flake8, pylint all require .py)
    if (stripped.includes(".py:")) {
      if (mypyPattern.test(stripped)) {
        return 0.93;
      }
      if (ruffFlake8Pattern.test(stripped)) {
        return 0.93;
      }
      if (ruffFlake8NoColPattern.test(stripped)) {
        return 0.91;
      }
      if (pylintPattern.test(stripped)) {
        return 0.93;
      }
    }

    // Fast path for File lines (traceback continuation, must start with whitespace + "File")
    if (stripped.startsWith("  File ") && tracebackFilePattern.test(stripped)) {
      return 0.8;
    }

    return 0;
  }

  parse(line: string, ctx: ParseContext): ParseResult {
    const stripped = stripAnsi(line);

    // Handle traceback start
    if (tracebackStartPattern.test(stripped)) {
      this.startTraceback(line);
      return null; // Wait for traceback to complete
    }

    // Handle pytest FAILED
    const pytestFailedMatch = pytestFailedPattern.exec(stripped);
    if (pytestFailedMatch) {
      return this.parsePytestFailed(pytestFailedMatch, line, ctx);
    }

    // Handle pytest ERROR
    const pytestErrorMatch = pytestErrorPattern.exec(stripped);
    if (pytestErrorMatch) {
      return this.parsePytestError(pytestErrorMatch, line, ctx);
    }

    // Handle mypy output
    const mypyMatch = mypyPattern.exec(stripped);
    if (mypyMatch) {
      return this.parseMypy(mypyMatch, line, ctx);
    }

    // Handle ruff/flake8 with column
    const ruffFlake8Match = ruffFlake8Pattern.exec(stripped);
    if (ruffFlake8Match) {
      return this.parseRuffFlake8(ruffFlake8Match, line, ctx);
    }

    // Handle ruff/flake8 without column
    const ruffFlake8NoColMatch = ruffFlake8NoColPattern.exec(stripped);
    if (ruffFlake8NoColMatch) {
      return this.parseRuffFlake8NoCol(ruffFlake8NoColMatch, line, ctx);
    }

    // Handle pylint
    const pylintMatch = pylintPattern.exec(stripped);
    if (pylintMatch) {
      return this.parsePylint(pylintMatch, line, ctx);
    }

    // Handle standalone SyntaxError BEFORE general exceptions
    const syntaxErrorMatch = syntaxErrorPattern.exec(stripped);
    if (syntaxErrorMatch) {
      return this.parseStandaloneSyntaxError(syntaxErrorMatch, line, ctx);
    }

    // Handle standalone exception (outside traceback)
    const exceptionMatch = exceptionPattern.exec(stripped);
    if (exceptionMatch) {
      return this.parseStandaloneException(exceptionMatch, line, ctx);
    }

    return null;
  }

  isNoise(line: string): boolean {
    const stripped = stripAnsi(line);
    const lowerStripped = stripped.toLowerCase();

    // Fast prefix checks
    for (const prefix of noiseFastPrefixes) {
      if (lowerStripped.startsWith(prefix)) {
        return true;
      }
    }

    // Fast contains checks
    for (const substr of noiseFastContains) {
      if (lowerStripped.includes(substr)) {
        return true;
      }
    }

    // Regex patterns
    for (const pattern of noisePatterns) {
      if (pattern.test(stripped)) {
        return true;
      }
    }

    return false;
  }

  continueMultiLine(line: string, _ctx: ParseContext): boolean {
    if (!this.traceback.inTraceback) {
      return false;
    }
    return this.continueTraceback(line);
  }

  finishMultiLine(ctx: ParseContext): ParseResult {
    if (!this.traceback.inTraceback) {
      return null;
    }
    return this.finishTraceback(ctx);
  }

  reset(): void {
    resetTracebackState(this.traceback);
  }

  noisePatterns(): NoisePatterns {
    return {
      fastPrefixes: noiseFastPrefixes,
      fastContains: noiseFastContains,
      regex: noisePatterns,
    };
  }

  // ============================================================================
  // Private Methods - Traceback Handling
  // ============================================================================

  /** Push a line to stack trace and update byte count (O(1) operation) */
  private pushTracebackLine(line: string): void {
    this.traceback.stackTrace.push(line);
    // +1 for newline character when joining
    this.traceback.byteCount += line.length + 1;
  }

  private startTraceback(rawLine: string): void {
    this.traceback.inTraceback = true;
    this.traceback.stackTrace = [rawLine];
    this.traceback.byteCount = rawLine.length;
    this.traceback.frameCount = 0;
    this.traceback.file = "";
    this.traceback.line = 0;
    this.traceback.function = "";
    this.traceback.isSyntaxError = false;
    this.traceback.column = 0;
    this.traceback.codeContext = "";
  }

  /**
   * State machine for Python traceback accumulation.
   *
   * Python tracebacks follow this structure:
   *   Traceback (most recent call last):
   *     File "a.py", line 10, in function_a    <- Frame 1 (oldest)
   *       code_line_1()
   *     File "b.py", line 20, in function_b    <- Frame 2
   *       code_line_2()
   *     File "c.py", line 30, in function_c    <- Frame N (deepest/most recent)
   *       code_line_n()
   *   ExceptionType: message                    <- End marker
   *
   * State transitions:
   *   - File lines: Extract location, increment frame count (keep LAST frame as deepest)
   *   - Code lines: Accumulate context (4+ space indented)
   *   - Caret lines: Extract column for SyntaxError (^ markers)
   *   - Chained headers: Continue accumulating (exception chains)
   *   - Exception/SyntaxError lines: Signal end, return false
   *   - Empty lines: Continue accumulating
   *   - Other lines: Signal end, return false
   */
  private continueTraceback(line: string): boolean {
    const stripped = stripAnsi(line);

    // Check resource limits using incremental byte count (O(1) instead of O(n) join)
    if (
      this.traceback.frameCount >= MAX_TRACEBACK_FRAMES ||
      this.traceback.byteCount >= MAX_TRACEBACK_BYTES
    ) {
      // Stop accumulating but look for exception line to finish
      if (
        exceptionPattern.test(stripped) ||
        syntaxErrorPattern.test(stripped)
      ) {
        return false;
      }
      return true;
    }

    // Handle chained exception headers - continue accumulating
    if (chainedExceptionPattern.test(stripped)) {
      this.pushTracebackLine(line);
      return true;
    }

    // Handle exception line - signals end of traceback
    if (exceptionPattern.test(stripped)) {
      this.pushTracebackLine(line);
      return false; // End of traceback
    }

    // Handle SyntaxError line - signals end of traceback
    if (syntaxErrorPattern.test(stripped)) {
      this.traceback.isSyntaxError = true;
      this.pushTracebackLine(line);
      return false; // End of traceback
    }

    // Handle File line - extract location (we want the LAST/deepest one)
    const fileMatch = tracebackFilePattern.exec(stripped);
    if (fileMatch) {
      const [, filePath, lineNum, funcName] = fileMatch;
      if (filePath && lineNum) {
        this.traceback.file = filePath;
        this.traceback.line = Number.parseInt(lineNum, 10);
        if (funcName) {
          this.traceback.function = funcName;
        }
        this.traceback.frameCount++;
        this.pushTracebackLine(line);
        return true;
      }
    }

    // Handle SyntaxError-specific File line (without function)
    const syntaxFileMatch = syntaxErrorFilePattern.exec(stripped);
    if (syntaxFileMatch) {
      const [, filePath, lineNum] = syntaxFileMatch;
      if (filePath && lineNum) {
        this.traceback.file = filePath;
        this.traceback.line = Number.parseInt(lineNum, 10);
        this.traceback.isSyntaxError = true;
        this.pushTracebackLine(line);
        return true;
      }
    }

    // Handle caret line for SyntaxError column detection
    if (syntaxErrorCaretPattern.test(stripped)) {
      const caretPos = stripped.indexOf("^");
      if (caretPos >= 0) {
        this.traceback.column = caretPos + 1; // 1-indexed
      }
      this.pushTracebackLine(line);
      return true;
    }

    // Handle code line in traceback
    if (tracebackCodePattern.test(stripped)) {
      if (this.traceback.isSyntaxError) {
        this.traceback.codeContext = stripped.trim();
      }
      this.pushTracebackLine(line);
      return true;
    }

    // Empty lines continue the traceback
    if (stripped.trim() === "") {
      this.pushTracebackLine(line);
      return true;
    }

    // Any other non-matching line signals end
    return false;
  }

  private finishTraceback(ctx: ParseContext): ParseResult {
    const stackTrace = this.traceback.stackTrace.join("\n");
    const lines = this.traceback.stackTrace;

    // Extract the exception message from the last line
    let message = "";
    for (let i = lines.length - 1; i >= 0; i--) {
      const lineItem = lines[i];
      if (!lineItem) {
        continue;
      }
      const line = stripAnsi(lineItem).trim();
      const exMatch = exceptionPattern.exec(line);
      if (exMatch) {
        const [, errType, errMsg] = exMatch;
        if (errType && errMsg) {
          message = `${errType}: ${errMsg}`;
          break;
        }
      }
      const syntaxMatch = syntaxErrorPattern.exec(line);
      if (syntaxMatch) {
        const [, errType, errMsg] = syntaxMatch;
        if (errType && errMsg) {
          message = `${errType}: ${errMsg}`;
          break;
        }
      }
    }

    // If no exception message found, use a generic one
    if (message === "") {
      message = "Python exception";
    }

    // Check if message will be truncated
    const messageTruncated = message.length > MAX_MESSAGE_LENGTH;
    message = truncateMessage(message);

    // Determine category
    const category = this.traceback.isSyntaxError ? "compile" : "runtime";

    // Check if stack trace was truncated due to resource limits
    const stackTraceTruncated =
      this.traceback.frameCount >= MAX_TRACEBACK_FRAMES ||
      this.traceback.byteCount >= MAX_TRACEBACK_BYTES;

    const err: MutableExtractedError = {
      message,
      file: this.traceback.file || undefined,
      line: this.traceback.line || undefined,
      column: this.traceback.column || undefined,
      severity: "error",
      raw: stackTrace,
      stackTrace,
      category,
      source: "python",
      lineKnown: this.traceback.file !== "" && this.traceback.line > 0,
      columnKnown: this.traceback.column > 0,
      stackTraceTruncated,
      messageTruncated,
    };

    applyWorkflowContext(err, ctx);

    this.reset();
    return err;
  }

  // ============================================================================
  // Private Methods - Single-line Parsers
  // ============================================================================

  private parsePytestFailed(
    matches: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const file = matches[1];
    const testName = matches[2];
    const message = matches[3];

    const fullMessage = truncateMessage(
      `Test failed: ${testName} - ${message}`
    );

    const err: MutableExtractedError = {
      message: fullMessage,
      file,
      severity: "error",
      raw: rawLine,
      category: "test",
      source: "python",
      ruleId: testName,
      lineKnown: false,
      columnKnown: false,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parsePytestError(
    matches: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const [, file, message] = matches;
    if (!(file && message)) {
      return null;
    }

    const err: MutableExtractedError = {
      message: `Collection error: ${message}`,
      file,
      severity: "error",
      raw: rawLine,
      category: "test",
      source: "python",
      lineKnown: false,
      columnKnown: false,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parseMypy(
    matches: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const [, file, lineStr, severity, msgRaw] = matches;
    if (!(file && lineStr && severity && msgRaw)) {
      return null;
    }

    const lineNum = Number.parseInt(lineStr, 10);
    let message = msgRaw;

    // Extract rule ID if present (in brackets at end)
    let ruleId: string | undefined;
    const bracketIdx = message.lastIndexOf(" [");
    if (bracketIdx !== -1 && message.endsWith("]")) {
      ruleId = message.slice(bracketIdx + 2, -1);
      message = message.slice(0, bracketIdx).trim();
    }

    // Map mypy severity
    const mappedSeverity: "error" | "warning" =
      severity === "warning" || severity === "note" ? "warning" : "error";

    message = truncateMessage(message);

    const err: MutableExtractedError = {
      message,
      file,
      line: lineNum,
      severity: mappedSeverity,
      raw: rawLine,
      category: "type-check",
      source: "python",
      ruleId,
      lineKnown: lineNum > 0,
      columnKnown: false,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parseRuffFlake8(
    matches: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const [, file, lineStr, colStr, code, msgRaw] = matches;
    if (!(file && lineStr && colStr && code && msgRaw)) {
      return null;
    }

    const lineNum = Number.parseInt(lineStr, 10);
    const col = Number.parseInt(colStr, 10);
    const severity = getRuffFlake8Severity(code);
    const message = truncateMessage(msgRaw);

    const err: MutableExtractedError = {
      message,
      file,
      line: lineNum,
      column: col,
      severity,
      raw: rawLine,
      category: "lint",
      source: "python",
      ruleId: code,
      lineKnown: lineNum > 0,
      columnKnown: col > 0,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parseRuffFlake8NoCol(
    matches: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const [, file, lineStr, code, msgRaw] = matches;
    if (!(file && lineStr && code && msgRaw)) {
      return null;
    }

    const lineNum = Number.parseInt(lineStr, 10);
    const severity = getRuffFlake8Severity(code);
    const message = truncateMessage(msgRaw);

    const err: MutableExtractedError = {
      message,
      file,
      line: lineNum,
      severity,
      raw: rawLine,
      category: "lint",
      source: "python",
      ruleId: code,
      lineKnown: lineNum > 0,
      columnKnown: false,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parsePylint(
    matches: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const [, file, lineStr, colStr, code, msgRaw, ruleId] = matches;
    if (!(file && lineStr && colStr && code && msgRaw)) {
      return null;
    }

    const lineNum = Number.parseInt(lineStr, 10);
    const col = Number.parseInt(colStr, 10);
    const severity = getPylintSeverity(code);
    const message = truncateMessage(msgRaw);

    const err: MutableExtractedError = {
      message,
      file,
      line: lineNum,
      column: col,
      severity,
      raw: rawLine,
      category: "lint",
      source: "python",
      ruleId,
      lineKnown: lineNum > 0,
      columnKnown: col > 0,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parseStandaloneException(
    matches: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const [, exceptionType, msgRaw] = matches;
    if (!(exceptionType && msgRaw)) {
      return null;
    }

    const message = truncateMessage(msgRaw);

    const err: MutableExtractedError = {
      message: `${exceptionType}: ${message}`,
      severity: "error",
      raw: rawLine,
      category: "runtime",
      source: "python",
      lineKnown: false,
      columnKnown: false,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parseStandaloneSyntaxError(
    matches: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const [, errorType, msgRaw] = matches;
    if (!(errorType && msgRaw)) {
      return null;
    }

    const message = truncateMessage(msgRaw);

    const err: MutableExtractedError = {
      message: `${errorType}: ${message}`,
      severity: "error",
      raw: rawLine,
      category: "compile",
      source: "python",
      lineKnown: false,
      columnKnown: false,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }
}

// ============================================================================
// Factory Function
// ============================================================================

export const createPythonParser = (): PythonParser => new PythonParser();
