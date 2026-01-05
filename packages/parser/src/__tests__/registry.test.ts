/**
 * Comprehensive tests for the ParserRegistry class.
 */

import { beforeEach, describe, expect, it } from "vitest";
import type { NoisePatternProvider, NoisePatterns } from "../parser-types.js";
import {
  BaseParser,
  type ParseContext,
  type ParseResult,
} from "../parser-types.js";
import { createGenericParser } from "../parsers/generic.js";
import { createGolangParser } from "../parsers/golang.js";
import { createPythonParser } from "../parsers/python.js";
import { createRustParser } from "../parsers/rust.js";
import { createTypeScriptParser } from "../parsers/typescript.js";
import {
  allSupported,
  createRegistry,
  type DetectedTool,
  type DetectionResult,
  detectAllToolsFromRun,
  detectToolFromRun,
  firstTool,
  firstToolID,
  formatUnsupportedToolsWarning,
  hasTools,
  ParserRegistry,
  unsupportedTools,
} from "../registry.js";

// ============================================================================
// Test Fixtures - Mock Parsers
// ============================================================================

class MockParser extends BaseParser {
  readonly id: string;
  readonly priority: number;
  private readonly confidence: number;
  private readonly parses: boolean;

  constructor(id: string, priority: number, confidence = 0.5, parses = true) {
    super();
    this.id = id;
    this.priority = priority;
    this.confidence = confidence;
    this.parses = parses;
  }

  canParse = (_line: string, _ctx: ParseContext): number => this.confidence;

  parse = (_line: string, _ctx: ParseContext): ParseResult => {
    if (!this.parses) {
      return null;
    }
    return {
      message: `Parsed by ${this.id}`,
      source: "generic",
    };
  };

  isNoise = (_line: string): boolean => false;
}

/** Regex for noise pattern testing - defined at module level for performance */
const noiseLineRegex = /^NOISE_LINE$/;

class MockParserWithNoise extends MockParser implements NoisePatternProvider {
  private readonly noisePrefixes: string[];
  private readonly noiseContains: string[];
  private readonly noiseRegex: RegExp[];

  constructor(
    id: string,
    priority: number,
    noisePrefixes: string[] = [],
    noiseContains: string[] = [],
    noiseRegex: RegExp[] = []
  ) {
    super(id, priority);
    this.noisePrefixes = noisePrefixes;
    this.noiseContains = noiseContains;
    this.noiseRegex = noiseRegex;
  }

  noisePatterns = (): NoisePatterns => ({
    fastPrefixes: this.noisePrefixes,
    fastContains: this.noiseContains,
    regex: this.noiseRegex,
  });
}

// ============================================================================
// Test Helpers
// ============================================================================

const createTestRegistry = (): ParserRegistry => {
  const registry = createRegistry();
  registry.register(createGolangParser());
  registry.register(createTypeScriptParser());
  registry.register(createPythonParser());
  registry.register(createRustParser());
  registry.register(createGenericParser());
  registry.initNoiseChecker();
  return registry;
};

// ============================================================================
// Test Suites
// ============================================================================

describe("ParserRegistry", () => {
  let registry: ParserRegistry;

  beforeEach(() => {
    registry = createRegistry();
  });

  describe("Parser registration", () => {
    it("registers a parser and retrieves it by ID", () => {
      const parser = new MockParser("test", 50);
      registry.register(parser);

      expect(registry.get("test")).toBe(parser);
    });

    it("returns undefined for unknown parser ID", () => {
      expect(registry.get("unknown")).toBeUndefined();
    });

    it("maintains parsers in priority order (highest first)", () => {
      registry.register(new MockParser("low", 10));
      registry.register(new MockParser("high", 90));
      registry.register(new MockParser("medium", 50));

      const parsers = registry.allParsers();
      expect(parsers[0]?.id).toBe("high");
      expect(parsers[1]?.id).toBe("medium");
      expect(parsers[2]?.id).toBe("low");
    });

    it("returns all parsers with allParsers()", () => {
      registry.register(new MockParser("a", 10));
      registry.register(new MockParser("b", 20));

      expect(registry.allParsers()).toHaveLength(2);
    });
  });

  describe("Parser selection (findParser)", () => {
    it("returns parser specified in context.tool (fast path)", () => {
      const parser = new MockParser("specific", 50);
      registry.register(parser);
      registry.register(new MockParser("other", 90, 1.0)); // Higher priority, higher confidence

      const ctx: ParseContext = {
        job: "",
        step: "",
        tool: "specific",
        lastFile: "",
        basePath: "",
      };

      const found = registry.findParser("any line", ctx);
      expect(found?.id).toBe("specific");
    });

    it("uses extension-based fast path for file paths", () => {
      registry.register(createGolangParser());
      registry.register(createTypeScriptParser());

      const goParser = registry.findParser("main.go:10:5: error");
      expect(goParser?.id).toBe("go");

      const tsParser = registry.findParser("src/app.ts(10,5): error TS123");
      expect(tsParser?.id).toBe("typescript");
    });

    it("falls back to priority-based selection", () => {
      registry.register(new MockParser("low", 10, 0.5));
      registry.register(new MockParser("high", 90, 0.8));
      registry.register(new MockParser("medium", 50, 0.6));

      // All parsers can parse, but "high" has highest confidence
      const parser = registry.findParser("some error line");
      expect(parser?.id).toBe("high");
    });

    it("returns parser with highest confidence score", () => {
      registry.register(new MockParser("low-confidence", 90, 0.3));
      registry.register(new MockParser("high-confidence", 10, 0.9));

      const parser = registry.findParser("some error line");
      expect(parser?.id).toBe("high-confidence");
    });

    it("returns undefined when no parser can handle the line", () => {
      registry.register(new MockParser("test", 50, 0)); // Returns 0 confidence

      const parser = registry.findParser("some line");
      expect(parser).toBeUndefined();
    });

    it("works without context parameter", () => {
      registry.register(createGolangParser());

      const parser = registry.findParser("main.go:10:5: undefined: foo");
      expect(parser?.id).toBe("go");
    });
  });

  describe("Dedicated parser detection", () => {
    it("hasDedicatedParser returns true for registered non-generic parser", () => {
      registry.register(createGolangParser());
      registry.register(createGenericParser());

      expect(registry.hasDedicatedParser("go")).toBe(true);
    });

    it("hasDedicatedParser returns false for generic parser", () => {
      registry.register(createGenericParser());

      expect(registry.hasDedicatedParser("generic")).toBe(false);
    });

    it("hasDedicatedParser returns false for unknown parser", () => {
      expect(registry.hasDedicatedParser("unknown")).toBe(false);
    });

    it("supportedToolIDs returns all non-generic parser IDs", () => {
      registry.register(createGolangParser());
      registry.register(createTypeScriptParser());
      registry.register(createGenericParser());

      const supported = registry.supportedToolIDs();
      expect(supported).toContain("go");
      expect(supported).toContain("typescript");
      expect(supported).not.toContain("generic");
    });
  });

  describe("Noise detection", () => {
    it("detects noise using consolidated patterns from parsers", () => {
      const parserWithNoise = new MockParserWithNoise(
        "noisy",
        50,
        ["skip this:"], // fastPrefixes
        ["ignore me"], // fastContains
        [noiseLineRegex] // regex
      );
      registry.register(parserWithNoise);
      registry.initNoiseChecker();

      expect(registry.isNoise("skip this: some content")).toBe(true);
      expect(registry.isNoise("prefix ignore me suffix")).toBe(true);
      expect(registry.isNoise("NOISE_LINE")).toBe(true);
      expect(registry.isNoise("valid content")).toBe(false);
    });

    it("treats empty/whitespace lines as noise", () => {
      registry.initNoiseChecker();

      expect(registry.isNoise("")).toBe(true);
      expect(registry.isNoise("   ")).toBe(true);
      expect(registry.isNoise("\t\n")).toBe(true);
    });

    it("handles ANSI escape codes in noise detection", () => {
      const parserWithNoise = new MockParserWithNoise("noisy", 50, ["skip:"]);
      registry.register(parserWithNoise);
      registry.initNoiseChecker();

      // Line with ANSI codes should still match
      expect(registry.isNoise("\x1b[31mskip: red text\x1b[0m")).toBe(true);
    });

    it("returns false when no noise checker is initialized", () => {
      // Don't call initNoiseChecker
      expect(registry.isNoise("any line")).toBe(false);
    });
  });

  describe("Reset functionality", () => {
    it("resetAll resets all registered parsers", () => {
      registry.register(createGolangParser());
      registry.register(createPythonParser());

      // Should not throw
      expect(() => registry.resetAll()).not.toThrow();
    });
  });
});

describe("Tool detection", () => {
  describe("detectToolFromRun", () => {
    it("detects Go tools", () => {
      expect(detectToolFromRun("go test ./...")).toBe("go");
      expect(detectToolFromRun("go build -o app .")).toBe("go");
      expect(detectToolFromRun("golangci-lint run ./...")).toBe("go");
      expect(detectToolFromRun("staticcheck ./...")).toBe("go");
      expect(detectToolFromRun("govulncheck ./...")).toBe("go");
    });

    it("detects TypeScript tools", () => {
      expect(detectToolFromRun("tsc --noEmit")).toBe("typescript");
      expect(detectToolFromRun("npx tsc --build")).toBe("typescript");
      expect(detectToolFromRun("bunx tsc")).toBe("typescript");
      expect(detectToolFromRun("pnpm run tsc")).toBe("typescript");
      expect(detectToolFromRun("yarn tsc --project tsconfig.json")).toBe(
        "typescript"
      );
    });

    it("detects ESLint", () => {
      expect(detectToolFromRun("eslint src/")).toBe("eslint");
      expect(detectToolFromRun("npx eslint .")).toBe("eslint");
      expect(detectToolFromRun("bunx eslint --fix")).toBe("eslint");
      expect(detectToolFromRun("pnpm eslint src/")).toBe("eslint");
      expect(detectToolFromRun("yarn eslint .")).toBe("eslint");
    });

    it("detects Rust/Cargo tools", () => {
      expect(detectToolFromRun("cargo test")).toBe("rust");
      expect(detectToolFromRun("cargo build --release")).toBe("rust");
      expect(detectToolFromRun("cargo clippy -- -D warnings")).toBe("rust");
      expect(detectToolFromRun("rustc main.rs")).toBe("rust");
      expect(detectToolFromRun("rustfmt src/")).toBe("rust");
    });

    it("detects Python tools", () => {
      expect(detectToolFromRun("pytest tests/")).toBe("python");
      expect(detectToolFromRun("python -m pytest")).toBe("python");
      expect(detectToolFromRun("python3 -m mypy src/")).toBe("python");
      expect(detectToolFromRun("mypy src/")).toBe("python");
      expect(detectToolFromRun("pylint src/")).toBe("python");
      expect(detectToolFromRun("flake8 src/")).toBe("python");
      expect(detectToolFromRun("ruff check src/")).toBe("python");
      expect(detectToolFromRun("uv run pytest")).toBe("python");
      expect(detectToolFromRun("poetry run pytest")).toBe("python");
    });

    it("returns empty string for unknown commands", () => {
      expect(detectToolFromRun("unknown-tool --arg")).toBe("");
      expect(detectToolFromRun("make build")).toBe("");
      expect(detectToolFromRun("docker build .")).toBe("");
    });

    it("handles commands with paths", () => {
      expect(detectToolFromRun("/usr/local/bin/golangci-lint run")).toBe("go");
      expect(detectToolFromRun("./node_modules/.bin/eslint src/")).toBe(
        "eslint"
      );
    });

    it("ignores comment lines", () => {
      expect(detectToolFromRun("# go test")).toBe("");
    });

    it("handles empty input", () => {
      expect(detectToolFromRun("")).toBe("");
    });
  });

  describe("detectAllToolsFromRun", () => {
    it("detects multiple tools in compound commands", () => {
      const tools = detectAllToolsFromRun("go test ./... && golangci-lint run");

      // Should detect "go" only once (deduplicated)
      const ids = tools.map((t) => t.id);
      expect(ids).toContain("go");
    });

    it("detects different tools in pipeline", () => {
      const tools = detectAllToolsFromRun("tsc --noEmit && eslint src/");

      const ids = tools.map((t) => t.id);
      expect(ids).toContain("typescript");
      expect(ids).toContain("eslint");
    });

    it("handles semicolon-separated commands", () => {
      const tools = detectAllToolsFromRun("cargo build; cargo test");

      const ids = tools.map((t) => t.id);
      expect(ids).toContain("rust");
    });

    it("handles OR-separated commands", () => {
      const tools = detectAllToolsFromRun("pytest || python -m pytest");

      const ids = tools.map((t) => t.id);
      expect(ids).toContain("python");
    });

    it("handles multi-line scripts", () => {
      const script = `go build ./...
go test ./...
golangci-lint run`;

      const tools = detectAllToolsFromRun(script);
      const ids = tools.map((t) => t.id);
      expect(ids).toContain("go");
    });

    it("returns empty array for no tools detected", () => {
      expect(detectAllToolsFromRun("echo hello")).toEqual([]);
    });
  });

  describe("Detection with registry", () => {
    let registry: ParserRegistry;

    beforeEach(() => {
      registry = createTestRegistry();
    });

    it("detectTools checks support status when requested", () => {
      const result = registry.detectTools("go test ./...", {
        checkSupport: true,
      });

      expect(result.tools).toHaveLength(1);
      expect(result.tools[0]?.id).toBe("go");
      expect(result.tools[0]?.supported).toBe(true);
    });

    it("detectTools returns firstOnly when requested", () => {
      const result = registry.detectTools("go test && cargo build", {
        firstOnly: true,
      });

      expect(result.tools).toHaveLength(1);
    });

    it("detectTools returns all tools without firstOnly", () => {
      const result = registry.detectTools("go test && cargo build");

      expect(result.tools.length).toBeGreaterThanOrEqual(2);
    });
  });
});

describe("Detection result helpers", () => {
  const mockResult: DetectionResult = {
    tools: [
      { id: "go", displayName: "go", supported: true },
      { id: "rust", displayName: "cargo", supported: false },
      { id: "python", displayName: "pytest", supported: true },
    ],
  };

  const emptyResult: DetectionResult = { tools: [] };

  describe("firstTool", () => {
    it("returns first tool from result", () => {
      const tool = firstTool(mockResult);
      expect(tool?.id).toBe("go");
    });

    it("returns undefined for empty result", () => {
      expect(firstTool(emptyResult)).toBeUndefined();
    });
  });

  describe("firstToolID", () => {
    it("returns first tool ID", () => {
      expect(firstToolID(mockResult)).toBe("go");
    });

    it("returns empty string for empty result", () => {
      expect(firstToolID(emptyResult)).toBe("");
    });
  });

  describe("hasTools", () => {
    it("returns true when tools detected", () => {
      expect(hasTools(mockResult)).toBe(true);
    });

    it("returns false for empty result", () => {
      expect(hasTools(emptyResult)).toBe(false);
    });
  });

  describe("unsupportedTools", () => {
    it("returns only unsupported tools", () => {
      const unsupported = unsupportedTools(mockResult);
      expect(unsupported).toHaveLength(1);
      expect(unsupported[0]?.id).toBe("rust");
    });

    it("returns empty array when all supported", () => {
      const allSupport: DetectionResult = {
        tools: [{ id: "go", displayName: "go", supported: true }],
      };
      expect(unsupportedTools(allSupport)).toHaveLength(0);
    });
  });

  describe("allSupported", () => {
    it("returns false when some tools unsupported", () => {
      expect(allSupported(mockResult)).toBe(false);
    });

    it("returns true when all tools supported", () => {
      const allSupport: DetectionResult = {
        tools: [
          { id: "go", displayName: "go", supported: true },
          { id: "python", displayName: "pytest", supported: true },
        ],
      };
      expect(allSupported(allSupport)).toBe(true);
    });

    it("returns true for empty result", () => {
      expect(allSupported(emptyResult)).toBe(true);
    });
  });
});

describe("formatUnsupportedToolsWarning", () => {
  it("formats warning for single unsupported tool", () => {
    const unsupported: DetectedTool[] = [
      { id: "make", displayName: "make", supported: false },
    ];

    const warning = formatUnsupportedToolsWarning(unsupported, ["go", "rust"]);

    expect(warning).toContain('Tool "make"');
    expect(warning).toContain("not fully supported");
    expect(warning).toContain("go, rust");
  });

  it("formats warning for multiple unsupported tools", () => {
    const unsupported: DetectedTool[] = [
      { id: "make", displayName: "make", supported: false },
      { id: "cmake", displayName: "cmake", supported: false },
    ];

    const warning = formatUnsupportedToolsWarning(unsupported, ["go"]);

    expect(warning).toContain("make and cmake");
    expect(warning).toContain("not fully supported");
  });

  it("formats warning for three+ unsupported tools with Oxford comma", () => {
    const unsupported: DetectedTool[] = [
      { id: "a", displayName: "tool-a", supported: false },
      { id: "b", displayName: "tool-b", supported: false },
      { id: "c", displayName: "tool-c", supported: false },
    ];

    const warning = formatUnsupportedToolsWarning(unsupported, []);

    expect(warning).toContain("tool-a, tool-b, and tool-c");
  });

  it("returns empty string when no unsupported tools", () => {
    expect(formatUnsupportedToolsWarning([], ["go"])).toBe("");
  });

  it("handles empty supported tools list", () => {
    const unsupported: DetectedTool[] = [
      { id: "make", displayName: "make", supported: false },
    ];

    const warning = formatUnsupportedToolsWarning(unsupported, []);

    expect(warning).not.toContain("Fully supported tools:");
    expect(warning).toContain("not fully supported");
  });
});

describe("Factory function", () => {
  it("createRegistry returns a new ParserRegistry", () => {
    const registry = createRegistry();
    expect(registry).toBeInstanceOf(ParserRegistry);
    expect(registry.allParsers()).toHaveLength(0);
  });
});
