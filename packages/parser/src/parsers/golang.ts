/**
 * Go parser for compiler, test, and linter error extraction.
 * Migrated from packages/core/tools/golang/parser.go
 */

import {
  applyWorkflowContext,
  MultiLineParser,
  type ParseContext,
  type ParseResult,
} from "../parser-types.js";
import type {
  ErrorCategory,
  ErrorSource,
  MutableExtractedError,
} from "../types.js";
import { safeParseInt, stripAnsi } from "../utils.js";

// ============================================================================
// Constants
// ============================================================================

const MAX_STACK_TRACE_LINES = 500;
const MAX_STACK_TRACE_BYTES = 512 * 1024;
/** Maximum line length to process (prevents ReDoS on malicious input) */
const MAX_LINE_LENGTH = 4096;

// ============================================================================
// Go-specific regex patterns
// ============================================================================

/**
 * Go compiler and linter errors: file.go:123:45: message
 * SECURITY: Anchored, no nested quantifiers, bounded character classes
 */
const goErrorPattern = /^([^\s:]+\.go):(\d+):(\d+):\s*(.+)$/;

/**
 * Go compiler errors without column: file.go:123: message
 * SECURITY: Anchored, no nested quantifiers, bounded character classes
 */
const goErrorNoColPattern = /^([^\s:]+\.go):(\d+):\s*(.+)$/;

/**
 * Test failure markers: --- FAIL: TestName (0.00s)
 * SECURITY: Anchored, single quantifier on \s, bounded test name
 */
const goTestFailPattern = /^--- FAIL: (\S+)/;

/**
 * Start of a panic: panic: message
 * SECURITY: Anchored, no nested quantifiers
 */
const goPanicPattern = /^panic: (.+)$/;

/**
 * Goroutine headers in stack traces
 * SECURITY: Anchored, simple pattern
 */
const goGoroutinePattern = /^goroutine \d+ \[/;

/**
 * Stack frame function lines: main.foo(0x1234)
 * SECURITY: Anchored, bounded character classes, no nested quantifiers
 */
const goStackFunctionPattern = /^\S+\([^)]*\)$/;

/**
 * Stack frame file lines: /path/to/file.go:123 +0x1a2
 * SECURITY: Anchored, bounded character classes
 */
const goStackFileLinePattern = /^\t[^\s]+\.go:\d+/;

/**
 * Created by lines in stack traces: created by main.main
 * SECURITY: Anchored, simple pattern
 */
const goCreatedByPattern = /^created by /;

/** Extract file and line from stack trace file lines */
const goStackFilePattern = /^\t(\S+\.go):(\d+)/;

/**
 * Build constraint errors
 * SECURITY: Simple alternation with anchored subpatterns
 */
const goBuildConstraintPattern =
  /build constraints? exclude|no (?:buildable )?go (?:source )?files/i;

/** Import cycle errors */
const goImportCyclePattern = /import cycle not allowed/;

/**
 * Go module errors: go: message or go.mod:10: message
 * SECURITY: Anchored, bounded groups
 */
const goModuleErrorPattern = /^go(?:\.mod)?(?::\d+)?: (.+)$/;

/**
 * golangci-lint rule extraction: message (rulename)
 * Captures the linter name in parentheses at end of line
 * SECURITY: Use [^()]+ instead of .+? to avoid backtracking
 */
const golangciLintRulePattern = /^([^()]+)\(([a-zA-Z][\w-]*)\)$/;

/**
 * Static analysis codes: SA4006, G101, ST1000, etc.
 * SECURITY: Anchored, bounded character classes
 */
const golangciLintCodePattern = /^([A-Z]{1,3}\d+): (.+)$/;

/**
 * Indented test output (4+ spaces)
 * SECURITY: Simple bounded quantifier
 */
const testOutputPattern = /^[ \t]{4}/;

/**
 * Test output file:line references
 * SECURITY: Anchored, bounded character classes
 */
const testFileLinePattern = /^[ \t]+([^\s:]+\.go):(\d+): (.+)$/;

/**
 * Test assertion patterns for extracting assertion messages
 * Matches common testing framework patterns like testify
 */
const testAssertionPattern =
  /Error Trace:|Error:|Messages:|expected:|actual:|Not equal:/i;

/**
 * Fast prefix checks for noise detection (O(1) lookup)
 * These are checked before regex patterns for performance
 */
const noisePrefixes = new Set([
  "=== RUN",
  "=== PAUSE",
  "=== CONT",
  "=== NAME",
  "--- PASS:",
  "--- SKIP:",
  "PASS",
  "ok ",
  "? ",
  "# ",
  "go: ",
  "level=",
  "Running ",
  "Issues:",
  "coverage:",
]);

/**
 * Noise patterns to skip (only checked after fast prefix fails)
 * SECURITY: All patterns are anchored and use simple quantifiers
 */
const noisePatterns = [
  /^FAIL \S+ \d/, // Test package failure summary
  /^[ \t]+--- PASS:/, // Nested test pass
];

// ============================================================================
// Linter Severity Mappings
// ============================================================================

const knownLinters: Readonly<Record<string, string>> = {
  // Error-level linters
  gosec: "error",
  staticcheck: "error",
  govet: "error",
  errcheck: "error",
  ineffassign: "error",
  typecheck: "error",
  bodyclose: "error",
  nilerr: "error",
  nilnil: "error",
  sqlclosecheck: "error",
  rowserrcheck: "error",
  makezero: "error",
  durationcheck: "error",
  exportloopref: "error",
  noctx: "error",
  exhaustive: "error",
  asasalint: "error",
  bidichk: "error",
  contextcheck: "error",
  errchkjson: "error",
  execinquery: "error",
  gomoddirectives: "error",
  goprintffuncname: "error",
  musttag: "error",
  nosprintfhostport: "error",
  reassign: "error",
  vet: "error",
  unused: "error",
  deadcode: "error",
  structcheck: "error",
  varcheck: "error",
  copyloopvar: "error",
  intrange: "error",
  zerologlint: "error",
  spancheck: "error",
  protogetter: "error",
  perfsprint: "error",
  nilnesserr: "error",
  fatcontext: "error",
  sloglint: "error",
  recvcheck: "error",

  // Warning-level linters
  gocritic: "warning",
  gocyclo: "warning",
  gocognit: "warning",
  funlen: "warning",
  lll: "warning",
  nestif: "warning",
  godox: "warning",
  gofmt: "warning",
  goimports: "warning",
  misspell: "warning",
  whitespace: "warning",
  wsl: "warning",
  nlreturn: "warning",
  dogsled: "warning",
  dupl: "warning",
  golint: "warning",
  stylecheck: "warning",
  unconvert: "warning",
  unparam: "warning",
  nakedret: "warning",
  prealloc: "warning",
  goconst: "warning",
  gomnd: "warning",
  mnd: "warning",
  revive: "warning",
  forbidigo: "warning",
  depguard: "warning",
  godot: "warning",
  err113: "warning",
  goerr113: "warning",
  wrapcheck: "warning",
  errorlint: "warning",
  forcetypeassert: "warning",
  ifshort: "warning",
  varnamelen: "warning",
  ireturn: "warning",
  exhaustruct: "warning",
  nonamedreturns: "warning",
  maintidx: "warning",
  cyclop: "warning",
  gochecknoglobals: "warning",
  gochecknoinits: "warning",
  testpackage: "warning",
  paralleltest: "warning",
  tparallel: "warning",
  thelper: "warning",
  containedctx: "warning",
  usestdlibvars: "warning",
  loggercheck: "warning",
  logrlint: "warning",
  decorder: "warning",
  errname: "warning",
  grouper: "warning",
  importas: "warning",
  interfacebloat: "warning",
  nolintlint: "warning",
  nosnakecase: "warning",
  predeclared: "warning",
  promlinter: "warning",
  tagliatelle: "warning",
  tenv: "warning",
  testableexamples: "warning",
  wastedassign: "warning",
  ascicheck: "warning",
  asciicheck: "warning",
  canonicalheader: "warning",
  dupword: "warning",
  gci: "warning",
  ginkgolinter: "warning",
  gocheckcompilerdirectives: "warning",
  gochecksumtype: "warning",
  goheader: "warning",
  gomodguard: "warning",
  gosimple: "warning",
  gosmopolitan: "warning",
  inamedparam: "warning",
  interfacer: "warning",
  mirror: "warning",
  nargs: "warning",
  tagalign: "warning",
  testifylint: "warning",
};

const codePrefixSeverity: Readonly<Record<string, string>> = {
  SA: "error",
  S: "warning",
  ST: "warning",
  QF: "warning",
  G: "error",
};

// ============================================================================
// Multi-line State Types
// ============================================================================

type MultiLineState = "idle" | "panic" | "test-failure";

interface PanicState {
  message: string;
  file: string;
  line: number;
  stackTrace: string[];
  goroutineSeen: boolean;
}

interface TestFailureState {
  testName: string;
  file: string;
  line: number;
  message: string;
  stackTrace: string[];
}

// ============================================================================
// Helper Functions
// ============================================================================

const extractCodePrefix = (code: string): string => {
  for (let i = 0; i < code.length; i++) {
    const c = code[i];
    if (c && c >= "0" && c <= "9") {
      return code.slice(0, i);
    }
  }
  return code;
};

const determineLintSeverity = (
  linterName: string,
  codePrefix: string
): string => {
  if (codePrefix) {
    const sev = codePrefixSeverity[codePrefix];
    if (sev) {
      return sev;
    }
  }

  if (linterName) {
    const sev = knownLinters[linterName];
    if (sev) {
      return sev;
    }
  }

  return "error";
};

// ============================================================================
// GolangParser Class
// ============================================================================

export class GolangParser extends MultiLineParser {
  readonly id = "go";
  readonly priority = 80;

  private state: MultiLineState = "idle";
  private panicState: PanicState = {
    message: "",
    file: "",
    line: 0,
    stackTrace: [],
    goroutineSeen: false,
  };
  private testState: TestFailureState = {
    testName: "",
    file: "",
    line: 0,
    message: "",
    stackTrace: [],
  };

  canParse = (line: string, _ctx: ParseContext): number => {
    // SECURITY: Reject extremely long lines to prevent ReDoS
    if (line.length > MAX_LINE_LENGTH) {
      return 0;
    }

    const stripped = stripAnsi(line);

    // Check if we're in a multi-line state
    if (this.state !== "idle") {
      return 0.9;
    }

    // Check for exact pattern matches (high confidence)
    if (goErrorPattern.test(stripped)) {
      return 0.95;
    }

    // Error without column number
    if (goErrorNoColPattern.test(stripped)) {
      return 0.93;
    }

    if (goTestFailPattern.test(stripped)) {
      return 0.95;
    }

    if (goPanicPattern.test(stripped)) {
      return 0.95;
    }

    // Go module errors
    if (goModuleErrorPattern.test(stripped)) {
      return 0.9;
    }

    // Lower confidence for stack trace continuation lines
    if (this.isStackTraceLine(stripped)) {
      return 0.8;
    }

    return 0;
  };

  /**
   * Check if a line is part of a Go stack trace
   * Separates patterns to avoid alternation-based ReDoS
   */
  private readonly isStackTraceLine = (stripped: string): boolean =>
    goGoroutinePattern.test(stripped) ||
    goStackFunctionPattern.test(stripped) ||
    goStackFileLinePattern.test(stripped) ||
    goCreatedByPattern.test(stripped);

  parse = (line: string, ctx: ParseContext): ParseResult => {
    const stripped = stripAnsi(line);

    // Handle panic start
    const panicMatches = goPanicPattern.exec(stripped);
    if (panicMatches) {
      this.startPanic(panicMatches[1] ?? "", line);
      return null;
    }

    // Handle test failure start
    const testFailMatches = goTestFailPattern.exec(stripped);
    if (testFailMatches) {
      this.startTestFailure(testFailMatches[1] ?? "");
      return null;
    }

    // Handle standard Go error (compiler, linter) with column
    const errorMatches = goErrorPattern.exec(stripped);
    if (errorMatches) {
      const lineNum = safeParseInt(errorMatches[2]) ?? 0;
      const col = safeParseInt(errorMatches[3]) ?? 0;
      return this.parseGoError(
        errorMatches[1] ?? "",
        lineNum,
        col,
        errorMatches[4] ?? "",
        line,
        ctx
      );
    }

    // Handle Go error without column
    const noColMatches = goErrorNoColPattern.exec(stripped);
    if (noColMatches) {
      const lineNum = safeParseInt(noColMatches[2]) ?? 0;
      return this.parseGoError(
        noColMatches[1] ?? "",
        lineNum,
        0,
        noColMatches[3] ?? "",
        line,
        ctx
      );
    }

    // Handle Go module errors
    const moduleMatches = goModuleErrorPattern.exec(stripped);
    if (moduleMatches) {
      return this.parseModuleError(moduleMatches[1] ?? "", line, ctx);
    }

    return null;
  };

  isNoise = (line: string): boolean => {
    const stripped = stripAnsi(line);

    // Fast path: check prefixes first (O(1) for each prefix length)
    for (const prefix of noisePrefixes) {
      if (stripped.startsWith(prefix)) {
        return true;
      }
    }

    // Slow path: regex patterns (only if fast path fails)
    for (const pattern of noisePatterns) {
      if (pattern.test(stripped)) {
        return true;
      }
    }
    return false;
  };

  continueMultiLine = (line: string, _ctx: ParseContext): boolean => {
    if (this.state === "panic") {
      return this.continuePanic(line);
    }

    if (this.state === "test-failure") {
      return this.continueTestFailure(line);
    }

    return false;
  };

  finishMultiLine = (ctx: ParseContext): ParseResult => {
    if (this.state === "panic") {
      return this.finishPanic(ctx);
    }

    if (this.state === "test-failure") {
      return this.finishTestFailure(ctx);
    }

    return null;
  };

  reset = (): void => {
    this.state = "idle";
    this.panicState = {
      message: "",
      file: "",
      line: 0,
      stackTrace: [],
      goroutineSeen: false,
    };
    this.testState = {
      testName: "",
      file: "",
      line: 0,
      message: "",
      stackTrace: [],
    };
  };

  // ============================================================================
  // Private Methods
  // ============================================================================

  private readonly parseGoError = (
    file: string,
    lineNum: number,
    col: number,
    message: string,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult => {
    const source: ErrorSource = "go";
    let category: ErrorCategory = "compile";
    let parsedMessage = message;

    // Check for specific error types
    if (goImportCyclePattern.test(parsedMessage)) {
      category = "compile";
    } else if (goBuildConstraintPattern.test(parsedMessage)) {
      category = "compile";
    }

    // Check for lint tool indicators in context
    if (ctx.step.toLowerCase().includes("lint")) {
      category = "lint";
    }

    // Extract rule ID from golangci-lint format
    let ruleId = "";
    let linterName = "";
    const ruleMatches = golangciLintRulePattern.exec(parsedMessage);
    if (ruleMatches) {
      parsedMessage = ruleMatches[1] ?? "";
      ruleId = ruleMatches[2] ?? "";
      linterName = ruleMatches[2] ?? "";
      category = "lint";
    }

    // Check for static analysis codes
    let codePrefix = "";
    const codeMatches = golangciLintCodePattern.exec(parsedMessage);
    if (codeMatches) {
      const code = codeMatches[1] ?? "";
      if (ruleId === "") {
        ruleId = code;
      } else {
        ruleId = `${code}/${ruleId}`;
      }
      parsedMessage = codeMatches[2] ?? "";
      category = "lint";
      codePrefix = extractCodePrefix(code);
    }

    const severity = determineLintSeverity(linterName, codePrefix);

    const err: MutableExtractedError = {
      message: parsedMessage,
      file,
      line: lineNum,
      column: col > 0 ? col : undefined,
      severity: severity === "warning" ? "warning" : "error",
      raw: rawLine,
      category,
      source,
      ruleId: ruleId || undefined,
      lineKnown: lineNum > 0,
      columnKnown: col > 0,
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly parseModuleError = (
    message: string,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult => {
    const err: MutableExtractedError = {
      message,
      severity: "error",
      raw: rawLine,
      category: "compile",
      source: "go",
      lineKnown: false,
      columnKnown: false,
    };

    applyWorkflowContext(err, ctx);
    return err;
  };

  private readonly startPanic = (message: string, rawLine: string): void => {
    this.state = "panic";
    this.panicState = {
      message,
      file: "",
      line: 0,
      stackTrace: [rawLine],
      goroutineSeen: false,
    };
  };

  private readonly startTestFailure = (testName: string): void => {
    this.state = "test-failure";
    this.testState = {
      testName,
      file: "",
      line: 0,
      message: "",
      stackTrace: [],
    };
  };

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: Go panic stack trace parsing requires handling multiple interleaved formats (goroutine headers, function frames, file references, resource limits)
  private readonly continuePanic = (line: string): boolean => {
    // SECURITY: Check resource limits BEFORE accumulating
    const currentBytes = this.panicState.stackTrace.reduce(
      (sum, l) => sum + l.length + 1,
      0
    );
    const atLimit =
      this.panicState.stackTrace.length >= MAX_STACK_TRACE_LINES ||
      currentBytes >= MAX_STACK_TRACE_BYTES;

    // Empty line might signal end of stack trace
    if (line.trim() === "") {
      if (this.panicState.goroutineSeen) {
        return false;
      }
      if (!atLimit) {
        this.panicState.stackTrace.push(line);
      }
      return true;
    }

    // Goroutine header
    if (goGoroutinePattern.test(line)) {
      this.panicState.goroutineSeen = true;
      if (!atLimit) {
        this.panicState.stackTrace.push(line);
      }
      return true;
    }

    // Stack frame (function line or file line)
    if (this.isStackTraceLine(line)) {
      if (!atLimit) {
        this.panicState.stackTrace.push(line);
      }

      // Extract first file location as the error location
      if (this.panicState.file === "") {
        const fileMatches = goStackFilePattern.exec(line);
        if (fileMatches) {
          this.panicState.file = fileMatches[1] ?? "";
          this.panicState.line = safeParseInt(fileMatches[2]) ?? 0;
        }
      }
      return true;
    }

    // If we've seen a goroutine but this line doesn't match stack patterns, end
    if (this.panicState.goroutineSeen) {
      return false;
    }

    // Before goroutine, accumulate everything (up to limit)
    if (!atLimit) {
      this.panicState.stackTrace.push(line);
    }
    return true;
  };

  private readonly continueTestFailure = (line: string): boolean => {
    // SECURITY: Check resource limits BEFORE accumulating
    const currentBytes = this.testState.stackTrace.reduce(
      (sum, l) => sum + l.length + 1,
      0
    );
    const atLimit =
      this.testState.stackTrace.length >= MAX_STACK_TRACE_LINES ||
      currentBytes >= MAX_STACK_TRACE_BYTES;

    // Non-indented, non-empty line ends test failure
    if (line.trim() !== "" && !testOutputPattern.test(line)) {
      return false;
    }

    // Empty line continues the test output context
    if (line.trim() === "") {
      return true;
    }

    // Check for test output with file:line reference
    const fileLineMatches = testFileLinePattern.exec(line);
    if (fileLineMatches) {
      if (this.testState.file === "") {
        this.testState.file = fileLineMatches[1] ?? "";
        this.testState.line = safeParseInt(fileLineMatches[2]) ?? 0;
        this.testState.message = fileLineMatches[3] ?? "";
      }
      if (!atLimit) {
        this.testState.stackTrace.push(line);
      }
      return true;
    }

    // Extract assertion messages from test framework output
    if (testAssertionPattern.test(line) && this.testState.message === "") {
      // Try to extract a meaningful message from assertion output
      const trimmed = line.trim();
      if (trimmed.startsWith("Error:") || trimmed.startsWith("Messages:")) {
        this.testState.message = trimmed;
      }
    }

    // Indented continuation lines
    if (!atLimit) {
      this.testState.stackTrace.push(line);
    }
    return true;
  };

  private readonly finishPanic = (ctx: ParseContext): ParseResult => {
    if (this.state !== "panic") {
      return null;
    }

    const stackTraceStr = this.panicState.stackTrace.join("\n");
    const truncated =
      this.panicState.stackTrace.length >= MAX_STACK_TRACE_LINES ||
      stackTraceStr.length >= MAX_STACK_TRACE_BYTES;

    const err: MutableExtractedError = {
      message: `panic: ${this.panicState.message}`,
      file: this.panicState.file || undefined,
      line: this.panicState.line > 0 ? this.panicState.line : undefined,
      severity: "error",
      raw: stackTraceStr,
      stackTrace: stackTraceStr,
      category: "runtime",
      source: "go",
      lineKnown: this.panicState.file !== "" && this.panicState.line > 0,
      columnKnown: false,
      stackTraceTruncated: truncated,
    };

    applyWorkflowContext(err, ctx);
    this.reset();
    return err;
  };

  private readonly finishTestFailure = (ctx: ParseContext): ParseResult => {
    if (this.state !== "test-failure") {
      return null;
    }

    let message = `FAIL: ${this.testState.testName}`;
    if (this.testState.message) {
      message = this.testState.message;
    }

    const stackTraceStr = this.testState.stackTrace.join("\n");
    const truncated =
      this.testState.stackTrace.length >= MAX_STACK_TRACE_LINES ||
      stackTraceStr.length >= MAX_STACK_TRACE_BYTES;

    const err: MutableExtractedError = {
      message,
      file: this.testState.file || undefined,
      line: this.testState.line > 0 ? this.testState.line : undefined,
      severity: "error",
      raw: `--- FAIL: ${this.testState.testName}`,
      stackTrace: stackTraceStr || undefined,
      category: "test",
      source: "go-test",
      lineKnown: this.testState.file !== "" && this.testState.line > 0,
      columnKnown: false,
      stackTraceTruncated: truncated,
    };

    applyWorkflowContext(err, ctx);
    this.reset();
    return err;
  };
}

// ============================================================================
// Factory Function
// ============================================================================

export const createGolangParser = (): GolangParser => new GolangParser();
