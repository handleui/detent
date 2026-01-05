/**
 * Parser Registry for managing tool-specific error parsers.
 * Migrated from packages/core/tools/registry.go
 */

import type {
  NoisePatternProvider,
  ParseContext,
  ToolParser,
} from "./parser-types.js";
import {
  extensionToParserID,
  extractFileExtension,
  splitCommands,
  stripAnsi,
} from "./utils.js";

/**
 * Shared empty context to avoid allocations in hot paths.
 * This is used when no context is provided to findParser.
 */
const emptyParseContext: ParseContext = Object.freeze({
  job: "",
  step: "",
  tool: "",
  lastFile: "",
  basePath: "",
});

// ============================================================================
// Noise Checker
// ============================================================================

/**
 * Optimized noise detection using consolidated patterns.
 * Instead of calling isNoise on every parser for every line, this consolidates
 * all noise patterns and applies fast checks before expensive regex operations.
 */
interface NoiseChecker {
  /** Fast prefixes that indicate noise (checked first) */
  readonly fastPrefixes: readonly string[];

  /** Fast substrings that indicate noise */
  readonly fastContains: readonly string[];

  /** Regex patterns for noise detection (checked last, most expensive) */
  readonly regexPatterns: readonly RegExp[];
}

/**
 * Create a noise checker by collecting patterns from all parsers.
 */
const createNoiseChecker = (parsers: readonly ToolParser[]): NoiseChecker => {
  const prefixSet = new Set<string>();
  const containsSet = new Set<string>();
  const regexSet = new Map<string, RegExp>();

  // Collect patterns from all parsers that implement NoisePatternProvider
  for (const p of parsers) {
    if (isNoisePatternProvider(p)) {
      const patterns = p.noisePatterns();

      // Collect fast prefixes (lowercase for case-insensitive matching)
      for (const prefix of patterns.fastPrefixes) {
        prefixSet.add(prefix.toLowerCase());
      }

      // Collect fast contains (lowercase for case-insensitive matching)
      for (const contains of patterns.fastContains) {
        containsSet.add(contains.toLowerCase());
      }

      // Collect regex patterns (deduplicate by string representation)
      for (const re of patterns.regex) {
        const patternStr = re.source;
        if (!regexSet.has(patternStr)) {
          regexSet.set(patternStr, re);
        }
      }
    }
  }

  return {
    fastPrefixes: [...prefixSet],
    fastContains: [...containsSet],
    regexPatterns: [...regexSet.values()],
  };
};

/**
 * Check if a parser implements NoisePatternProvider.
 */
const isNoisePatternProvider = (
  p: ToolParser
): p is ToolParser & NoisePatternProvider =>
  "noisePatterns" in p &&
  typeof (p as NoisePatternProvider).noisePatterns === "function";

/**
 * Check if a line is noise using the consolidated checker.
 */
const isNoiseWithChecker = (
  line: string,
  checker: NoiseChecker | undefined
): boolean => {
  if (!checker) {
    return false;
  }

  // Strip ANSI codes once for all checks
  const stripped = stripAnsi(line);

  // Fast path: empty or whitespace-only lines are noise
  const trimmed = stripped.trim();
  if (trimmed === "") {
    return true;
  }

  // Fast prefix checks (case-insensitive)
  const lowerTrimmed = trimmed.toLowerCase();
  for (const prefix of checker.fastPrefixes) {
    if (lowerTrimmed.startsWith(prefix)) {
      return true;
    }
  }

  // Fast substring checks (case-insensitive)
  const lowerStripped = stripped.toLowerCase();
  for (const substr of checker.fastContains) {
    if (lowerStripped.includes(substr)) {
      return true;
    }
  }

  // Regex patterns (most expensive, checked last)
  for (const pattern of checker.regexPatterns) {
    if (pattern.test(stripped)) {
      return true;
    }
  }

  return false;
};

// ============================================================================
// Tool Patterns for Detection
// ============================================================================

/**
 * Tool detection pattern with metadata.
 */
interface ToolPattern {
  readonly pattern: RegExp;
  readonly parserID: string;
  readonly displayName: string;
}

/**
 * Patterns for detecting tools from run commands.
 * Only patterns for tools with implemented parsers are included.
 */
const toolPatterns: readonly ToolPattern[] = [
  // Go tools
  {
    pattern: /(?:^|\s|\/)golangci-lint\s/,
    parserID: "go",
    displayName: "golangci-lint",
  },
  {
    pattern: /(?:^|\s)go\s+(test|build|vet|run|install|mod|fmt|generate)\b/,
    parserID: "go",
    displayName: "go",
  },
  { pattern: /(?:^|\s)go\s+tool\s/, parserID: "go", displayName: "go tool" },
  {
    pattern: /(?:^|\s|\/)staticcheck\b/,
    parserID: "go",
    displayName: "staticcheck",
  },
  {
    pattern: /(?:^|\s|\/)govulncheck\b/,
    parserID: "go",
    displayName: "govulncheck",
  },

  // TypeScript/JavaScript type checking
  { pattern: /(?:^|\s|\/)tsc\b/, parserID: "typescript", displayName: "tsc" },
  {
    pattern: /(?:^|\s)npx\s+tsc\b/,
    parserID: "typescript",
    displayName: "tsc",
  },
  {
    pattern: /(?:^|\s)bunx?\s+tsc\b/,
    parserID: "typescript",
    displayName: "tsc",
  },
  {
    // HACK: Use [^\n]* instead of .* to prevent ReDoS - limits backtracking
    pattern: /(?:^|\s)pnpm\s+[^\n]*\btsc\b/,
    parserID: "typescript",
    displayName: "tsc",
  },
  {
    // HACK: Use [^\n]* instead of .* to prevent ReDoS - limits backtracking
    pattern: /(?:^|\s)yarn\s+[^\n]*\btsc\b/,
    parserID: "typescript",
    displayName: "tsc",
  },

  // ESLint
  { pattern: /(?:^|\s|\/)eslint\b/, parserID: "eslint", displayName: "eslint" },
  {
    pattern: /(?:^|\s)npx\s+eslint\b/,
    parserID: "eslint",
    displayName: "eslint",
  },
  {
    pattern: /(?:^|\s)bunx?\s+eslint\b/,
    parserID: "eslint",
    displayName: "eslint",
  },
  {
    // HACK: Use [^\n]* instead of .* to prevent ReDoS - limits backtracking
    pattern: /(?:^|\s)pnpm\s+[^\n]*\beslint\b/,
    parserID: "eslint",
    displayName: "eslint",
  },
  {
    // HACK: Use [^\n]* instead of .* to prevent ReDoS - limits backtracking
    pattern: /(?:^|\s)yarn\s+[^\n]*\beslint\b/,
    parserID: "eslint",
    displayName: "eslint",
  },

  // Rust tools
  {
    pattern: /(?:^|\s)cargo\s+(test|build|check|clippy|run|fmt)\b/,
    parserID: "rust",
    displayName: "cargo",
  },
  { pattern: /(?:^|\s|\/)rustc\b/, parserID: "rust", displayName: "rustc" },
  {
    pattern: /(?:^|\s|\/)clippy-driver\b/,
    parserID: "rust",
    displayName: "clippy",
  },
  { pattern: /(?:^|\s|\/)rustfmt\b/, parserID: "rust", displayName: "rustfmt" },

  // Python tools
  {
    pattern: /(?:^|\s)python3?\s+-m\s+pytest\b/,
    parserID: "python",
    displayName: "pytest",
  },
  { pattern: /(?:^|\s|\/)pytest\b/, parserID: "python", displayName: "pytest" },
  {
    pattern: /(?:^|\s)python3?\s+-m\s+mypy\b/,
    parserID: "python",
    displayName: "mypy",
  },
  { pattern: /(?:^|\s|\/)mypy\b/, parserID: "python", displayName: "mypy" },
  {
    pattern: /(?:^|\s)python3?\s+-m\s+pylint\b/,
    parserID: "python",
    displayName: "pylint",
  },
  { pattern: /(?:^|\s|\/)pylint\b/, parserID: "python", displayName: "pylint" },
  {
    pattern: /(?:^|\s)python3?\s+-m\s+flake8\b/,
    parserID: "python",
    displayName: "flake8",
  },
  { pattern: /(?:^|\s|\/)flake8\b/, parserID: "python", displayName: "flake8" },
  {
    pattern: /(?:^|\s|\/)ruff\s+(check|format)\b/,
    parserID: "python",
    displayName: "ruff",
  },
  { pattern: /(?:^|\s|\/)ruff\b/, parserID: "python", displayName: "ruff" },
  { pattern: /(?:^|\s)python3?\s+/, parserID: "python", displayName: "python" },
  {
    pattern: /(?:^|\s)pip3?\s+install\b/,
    parserID: "python",
    displayName: "pip",
  },
  {
    pattern: /(?:^|\s|\/)uv\s+(run|pip|sync)\b/,
    parserID: "python",
    displayName: "uv",
  },
  {
    pattern: /(?:^|\s|\/)poetry\s+(install|run|build)\b/,
    parserID: "python",
    displayName: "poetry",
  },
];

// ============================================================================
// Detection Types
// ============================================================================

/**
 * Detected tool information.
 */
export interface DetectedTool {
  readonly id: string;
  readonly displayName: string;
  readonly supported: boolean;
}

/**
 * Options for tool detection.
 */
export interface DetectionOptions {
  /** Return only the first detected tool (default: false, returns all) */
  readonly firstOnly?: boolean;
  /** Check if tools are supported by the registry (default: false) */
  readonly checkSupport?: boolean;
}

/**
 * Result of tool detection.
 */
export interface DetectionResult {
  readonly tools: readonly DetectedTool[];
}

/**
 * Get the first detected tool, or undefined if none found.
 */
export const firstTool = (result: DetectionResult): DetectedTool | undefined =>
  result.tools[0];

/**
 * Get the ID of the first detected tool, or empty string if none found.
 */
export const firstToolID = (result: DetectionResult): string =>
  result.tools[0]?.id ?? "";

/**
 * Check if any tools were detected.
 */
export const hasTools = (result: DetectionResult): boolean =>
  result.tools.length > 0;

/**
 * Get only unsupported tools from the result.
 */
export const unsupportedTools = (
  result: DetectionResult
): readonly DetectedTool[] => result.tools.filter((t) => !t.supported);

/**
 * Check if all detected tools are supported.
 */
export const allSupported = (result: DetectionResult): boolean =>
  result.tools.every((t) => t.supported);

// ============================================================================
// Parser Registry
// ============================================================================

/**
 * ParserRegistry manages tool parsers and routes output to the appropriate parser.
 * It maintains parsers in priority order and supports tool-aware selection.
 */
export class ParserRegistry {
  private readonly parsers: ToolParser[] = [];
  private readonly byID: Map<string, ToolParser> = new Map();
  private readonly byExt: Map<string, ToolParser> = new Map();
  private noiseChecker: NoiseChecker | undefined;

  /**
   * Register a parser with the registry.
   * Parsers are automatically sorted by priority (highest first).
   */
  register(parser: ToolParser): void {
    this.parsers.push(parser);
    this.byID.set(parser.id, parser);

    // Populate extension index for this parser
    for (const [ext, parserID] of Object.entries(extensionToParserID)) {
      if (parserID === parser.id) {
        this.byExt.set(ext, parser);
      }
    }

    // Sort by priority descending (highest priority first)
    this.parsers.sort((a, b) => b.priority - a.priority);
  }

  /**
   * Get a parser by ID, or undefined if not found.
   */
  get(id: string): ToolParser | undefined {
    return this.byID.get(id);
  }

  /**
   * Find the best matching parser for a line.
   * If the context has a known tool, that parser is used directly.
   * Otherwise, tries extension-based lookup first, then falls back to
   * priority-ordered confidence scoring.
   */
  findParser(line: string, ctx?: ParseContext): ToolParser | undefined {
    // Fast path 1: if tool is known from step context, use that parser directly
    if (ctx?.tool) {
      const p = this.byID.get(ctx.tool);
      if (p) {
        return p;
      }
    }

    // Fast path 2: try extension-based lookup for lines with file paths
    const ext = extractFileExtension(line);
    if (ext) {
      const p = this.byExt.get(ext);
      if (p && p.canParse(line, ctx ?? emptyParseContext) > 0) {
        return p;
      }
    }

    // Slow path: find parser with highest confidence score
    let best: ToolParser | undefined;
    let bestScore = 0;

    const effectiveCtx = ctx ?? emptyParseContext;
    for (const p of this.parsers) {
      const score = p.canParse(line, effectiveCtx);
      if (score > bestScore) {
        bestScore = score;
        best = p;
      }
    }

    return best;
  }

  /**
   * Check if a line is noise using optimized consolidated patterns.
   */
  isNoise(line: string): boolean {
    return isNoiseWithChecker(line, this.noiseChecker);
  }

  /**
   * Initialize the noise checker by collecting patterns from all registered parsers.
   * Call this after all parsers are registered.
   */
  initNoiseChecker(): void {
    this.noiseChecker = createNoiseChecker(this.parsers);
  }

  /**
   * Reset the state of all registered parsers.
   * Should be called between parsing different outputs.
   */
  resetAll(): void {
    for (const p of this.parsers) {
      p.reset();
    }
  }

  /**
   * Get all registered parsers in priority order.
   * Returns the internal array directly (readonly prevents mutation).
   */
  allParsers(): readonly ToolParser[] {
    return this.parsers;
  }

  /**
   * Check if the registry has a non-generic parser for the given ID.
   */
  hasDedicatedParser(id: string): boolean {
    const p = this.byID.get(id);
    if (!p) {
      return false;
    }
    // Generic parser is a fallback, not a "dedicated" parser
    return p.id !== "generic";
  }

  /**
   * Get all registered parser IDs that have dedicated parsing (excludes generic).
   */
  supportedToolIDs(): string[] {
    return [...this.byID.keys()].filter((id) => id !== "generic").sort();
  }

  /**
   * Detect tools from a run command.
   */
  detectTools(run: string, opts: DetectionOptions = {}): DetectionResult {
    return detectToolsInternal(run, opts, this);
  }
}

/**
 * Internal tool detection logic.
 * Migrated from Go registry.go - shell command parsing is inherently complex.
 */
const detectToolsInternal = (
  run: string,
  opts: DetectionOptions,
  registry?: ParserRegistry
): DetectionResult => {
  const seen = new Set<string>();
  const detectedTools: DetectedTool[] = [];

  // Process each line of the command
  for (const rawLine of run.split("\n")) {
    const line = rawLine.trim();
    if (line === "" || line.startsWith("#")) {
      continue;
    }

    // Process each command segment
    for (const rawCmd of splitCommands(line)) {
      const cmd = rawCmd.trim();
      if (cmd === "") {
        continue;
      }

      const detected = detectSingleCommand(cmd, seen, opts, registry);
      if (detected) {
        detectedTools.push(detected);
        if (opts.firstOnly) {
          return { tools: detectedTools };
        }
      }
    }
  }

  return { tools: detectedTools };
};

/**
 * Detect a tool from a single command string.
 */
const detectSingleCommand = (
  cmd: string,
  seen: Set<string>,
  opts: DetectionOptions,
  registry?: ParserRegistry
): DetectedTool | undefined => {
  for (const tp of toolPatterns) {
    if (tp.pattern.test(cmd) && !seen.has(tp.parserID)) {
      seen.add(tp.parserID);
      return {
        id: tp.parserID,
        displayName: tp.displayName,
        supported:
          opts.checkSupport && registry
            ? registry.hasDedicatedParser(tp.parserID)
            : false,
      };
    }
  }
  return undefined;
};

/**
 * Detect tool from a run command (standalone function).
 * Returns empty string if no known tool is detected.
 */
export const detectToolFromRun = (run: string): string =>
  firstToolID(detectToolsInternal(run, { firstOnly: true }));

/**
 * Detect all tools from a run command (standalone function).
 */
export const detectAllToolsFromRun = (run: string): readonly DetectedTool[] =>
  detectToolsInternal(run, {}).tools;

// ============================================================================
// Warning Formatting
// ============================================================================

/**
 * Format a list of items as "a, b, and c" or "a and b".
 */
const formatList = (items: readonly string[]): string => {
  if (items.length === 0) {
    return "";
  }
  if (items.length === 1) {
    return items[0] ?? "";
  }
  if (items.length === 2) {
    return `${items[0]} and ${items[1]}`;
  }
  const last = items.at(-1);
  return `${items.slice(0, -1).join(", ")}, and ${last}`;
};

/**
 * Format a warning message for unsupported tools.
 */
export const formatUnsupportedToolsWarning = (
  unsupported: readonly DetectedTool[],
  supportedIDs: readonly string[]
): string => {
  if (unsupported.length === 0) {
    return "";
  }

  const toolNames = unsupported.map((t) => t.displayName);
  let msg =
    unsupported.length === 1
      ? `Tool "${toolNames[0]}" detected but not fully supported`
      : `Tools ${formatList(toolNames)} detected but not fully supported`;

  msg += ". Errors will be captured but may not be fully structured.";

  if (supportedIDs.length > 0) {
    msg += ` Fully supported tools: ${supportedIDs.join(", ")}.`;
  }

  return msg;
};

// ============================================================================
// Factory
// ============================================================================

/**
 * Create a new empty parser registry.
 */
export const createRegistry = (): ParserRegistry => new ParserRegistry();
