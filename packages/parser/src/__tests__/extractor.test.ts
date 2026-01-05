/**
 * Comprehensive tests for the Extractor class.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { passthroughParser } from "../ci-types.js";
import {
  createExtractor,
  Extractor,
  getUnknownPatternReporter,
  maxDeduplicationSize,
  maxLineLength,
  reportUnknownPatterns,
  setUnknownPatternReporter,
} from "../extractor.js";
import { createGenericParser } from "../parsers/generic.js";
import { createGolangParser } from "../parsers/golang.js";
import { createPythonParser } from "../parsers/python.js";
import { createRustParser } from "../parsers/rust.js";
import { createTypeScriptParser } from "../parsers/typescript.js";
import { createRegistry, type ParserRegistry } from "../registry.js";
import type { ExtractedError } from "../types.js";

// ============================================================================
// Test Fixtures
// ============================================================================

const goCompilerError = "main.go:10:5: undefined: someFunc";

const goMultipleErrors = `main.go:10:5: undefined: someFunc
utils.go:20:10: cannot use x (type int) as type string`;

const mixedToolOutput = `src/main.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.
main.go:15:3: cannot use x (type int) as type string
src/lib.rs:4:7: error: mismatched types`;

const pythonTraceback = `Traceback (most recent call last):
  File "/app/main.py", line 42, in process
    result = compute(data)
  File "/app/utils.py", line 18, in compute
    return data / 0
ZeroDivisionError: division by zero`;

const goPanicStack = `panic: runtime error: index out of range [5] with length 3

goroutine 1 [running]:
main.processData(0xc0000b4000, 0x3, 0x8)
	/app/main.go:25 +0x1a2
main.main()
	/app/main.go:10 +0x85`;

const rustMultiLineError = `error[E0308]: mismatched types
 --> src/main.rs:4:7
  |
4 |     let x: i32 = "hello";
  |            ---   ^^^^^^^ expected \`i32\`, found \`&str\`
  |            |
  |            expected due to this
  = note: expected type \`i32\`
             found type \`&str\`
  = help: consider using \`.parse()\``;

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

/**
 * Create a registry without the generic parser.
 * The generic parser's noise patterns can interfere with multi-line parsing
 * because they mark traceback lines as noise at the registry level.
 */
const createMultiLineTestRegistry = (): ParserRegistry => {
  const registry = createRegistry();
  registry.register(createGolangParser());
  registry.register(createPythonParser());
  registry.register(createRustParser());
  registry.initNoiseChecker();
  return registry;
};

// ============================================================================
// Test Suites
// ============================================================================

describe("Extractor", () => {
  let registry: ParserRegistry;
  let extractor: Extractor;

  beforeEach(() => {
    registry = createTestRegistry();
    extractor = createExtractor(registry);
  });

  describe("Basic extraction", () => {
    it("extracts a single error from Go compiler output", () => {
      const errors = extractor.extract(goCompilerError, passthroughParser);

      expect(errors).toHaveLength(1);
      expect(errors[0]).toMatchObject({
        file: "main.go",
        line: 10,
        column: 5,
        message: "undefined: someFunc",
        source: "go",
      });
    });

    it("extracts multiple errors from the same tool", () => {
      const errors = extractor.extract(goMultipleErrors, passthroughParser);

      expect(errors).toHaveLength(2);
      expect(errors[0]).toMatchObject({
        file: "main.go",
        line: 10,
        message: "undefined: someFunc",
      });
      expect(errors[1]).toMatchObject({
        file: "utils.go",
        line: 20,
        message: "cannot use x (type int) as type string",
      });
    });

    it("extracts errors from mixed tool output", () => {
      const errors = extractor.extract(mixedToolOutput, passthroughParser);

      expect(errors.length).toBeGreaterThanOrEqual(2);

      // TypeScript error
      const tsError = errors.find((e) => e.source === "typescript");
      expect(tsError).toBeDefined();
      expect(tsError?.file).toBe("src/main.ts");

      // Go error
      const goError = errors.find((e) => e.source === "go");
      expect(goError).toBeDefined();
      expect(goError?.file).toBe("main.go");
    });

    it("handles empty input", () => {
      const errors = extractor.extract("", passthroughParser);
      expect(errors).toHaveLength(0);
    });

    it("handles input with no errors", () => {
      const noiseOutput = `Building project...
Compiling main.go...
Build succeeded.
All tests passed.`;

      const errors = extractor.extract(noiseOutput, passthroughParser);
      expect(errors).toHaveLength(0);
    });
  });

  describe("Deduplication", () => {
    it("removes exact duplicates", () => {
      const duplicatedOutput = `main.go:10:5: undefined: someFunc
main.go:10:5: undefined: someFunc
main.go:10:5: undefined: someFunc`;

      const errors = extractor.extract(duplicatedOutput, passthroughParser);

      expect(errors).toHaveLength(1);
    });

    it("keeps errors with different messages at the same location", () => {
      const differentMessages = `main.go:10:5: undefined: someFunc
main.go:10:5: cannot use x as type string`;

      const errors = extractor.extract(differentMessages, passthroughParser);

      expect(errors).toHaveLength(2);
    });

    it("keeps errors at different locations with the same message", () => {
      const sameMessageDifferentLines = `main.go:10:5: undefined: x
main.go:20:5: undefined: x
utils.go:10:5: undefined: x`;

      const errors = extractor.extract(
        sameMessageDifferentLines,
        passthroughParser
      );

      expect(errors).toHaveLength(3);
    });

    it("respects deduplication limit (maxDeduplicationSize)", () => {
      // Generate more unique errors than the deduplication limit
      const uniqueErrors: string[] = [];
      for (let i = 0; i < maxDeduplicationSize + 100; i++) {
        uniqueErrors.push(`main.go:${i}:1: error ${i}`);
      }

      const errors = extractor.extract(
        uniqueErrors.join("\n"),
        passthroughParser
      );

      // Should stop at the deduplication limit
      expect(errors).toHaveLength(maxDeduplicationSize);
    });
  });

  describe("Multi-line handling", () => {
    // Multi-line tests use a registry without the generic parser
    // because its noise patterns can interfere with traceback parsing
    let multiLineExtractor: Extractor;

    beforeEach(() => {
      const mlRegistry = createMultiLineTestRegistry();
      multiLineExtractor = createExtractor(mlRegistry);
    });

    it("handles Python tracebacks", () => {
      const errors = multiLineExtractor.extract(
        pythonTraceback,
        passthroughParser
      );

      expect(errors).toHaveLength(1);
      const error = errors[0];
      expect(error).toBeDefined();
      expect(error?.source).toBe("python");
      expect(error?.message).toContain("ZeroDivisionError");
      expect(error?.file).toBe("/app/utils.py");
      expect(error?.line).toBe(18);
      expect(error?.stackTrace).toBeDefined();
      expect(error?.stackTrace).toContain("Traceback");
    });

    it("handles Go panic stacks", () => {
      const errors = multiLineExtractor.extract(
        goPanicStack,
        passthroughParser
      );

      expect(errors).toHaveLength(1);
      const error = errors[0];
      expect(error).toBeDefined();
      expect(error?.source).toBe("go");
      expect(error?.message).toContain("panic");
      expect(error?.message).toContain("index out of range");
      expect(error?.stackTrace).toBeDefined();
      expect(error?.stackTrace).toContain("goroutine");
    });

    it("handles Rust multi-line errors with suggestions", () => {
      const errors = multiLineExtractor.extract(
        rustMultiLineError,
        passthroughParser
      );

      expect(errors).toHaveLength(1);
      const error = errors[0];
      expect(error).toBeDefined();
      expect(error?.source).toBe("rust");
      expect(error?.message).toBe("mismatched types");
      expect(error?.ruleId).toContain("E0308");
      // Suggestions are extracted from notes/help lines
      expect(error?.suggestions).toBeDefined();
      expect(error?.suggestions?.length).toBeGreaterThan(0);
      // Stack trace should contain the full multi-line context
      expect(error?.stackTrace).toBeDefined();
      expect(error?.stackTrace).toContain("error[E0308]");
    });

    it("finalizes pending multi-line error at end of input", () => {
      // Rust error without a terminating blank line
      const incompleteRustError = `error[E0308]: mismatched types
 --> src/main.rs:4:7
  |
4 |     let x: i32 = "hello";
  |                  ^^^^^^^`;

      const errors = multiLineExtractor.extract(
        incompleteRustError,
        passthroughParser
      );

      expect(errors).toHaveLength(1);
      expect(errors[0]?.source).toBe("rust");
    });

    it("handles chained Python exceptions", () => {
      const chainedTraceback = `Traceback (most recent call last):
  File "/app/main.py", line 10, in wrapper
    inner()
  File "/app/main.py", line 5, in inner
    raise ValueError("inner error")
ValueError: inner error

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "/app/main.py", line 15, in main
    wrapper()
  File "/app/main.py", line 12, in wrapper
    raise RuntimeError("wrapper error")
RuntimeError: wrapper error`;

      const errors = multiLineExtractor.extract(
        chainedTraceback,
        passthroughParser
      );

      // Should extract both tracebacks
      expect(errors.length).toBeGreaterThanOrEqual(1);
      const messages = errors.map((e) => e.message);
      expect(messages.some((m) => m.includes("RuntimeError"))).toBe(true);
    });
  });

  describe("Line limits", () => {
    it("skips very long lines (maxLineLength)", () => {
      // Create a line that exceeds maxLineLength
      const longLine = "x".repeat(maxLineLength + 100);
      const outputWithLongLine = `${longLine}
main.go:10:5: valid error
${"y".repeat(maxLineLength + 50)}`;

      const errors = extractor.extract(outputWithLongLine, passthroughParser);

      // Only the valid error should be extracted
      expect(errors).toHaveLength(1);
      expect(errors[0]?.message).toBe("valid error");
    });

    it("handles input with many lines", () => {
      const manyErrors: string[] = [];
      for (let i = 1; i <= 1000; i++) {
        manyErrors.push(`file${i}.go:${i}:1: error number ${i}`);
      }

      const errors = extractor.extract(
        manyErrors.join("\n"),
        passthroughParser
      );

      expect(errors.length).toBe(1000);
    });
  });

  describe("Reset functionality", () => {
    it("clears workflow context on reset", () => {
      // First extraction with context
      extractor.extract("main.go:10:5: error", passthroughParser);

      // Reset
      extractor.reset();

      // Workflow context should be cleared
      expect(extractor.getWorkflowContext()).toBeUndefined();
    });

    it("allows fresh extraction after reset", () => {
      // First extraction
      const errors1 = extractor.extract(
        "main.go:10:5: first error",
        passthroughParser
      );
      expect(errors1).toHaveLength(1);

      extractor.reset();

      // Second extraction should work independently
      const errors2 = extractor.extract(
        "utils.go:20:3: second error",
        passthroughParser
      );
      expect(errors2).toHaveLength(1);
      expect(errors2[0]?.file).toBe("utils.go");
    });
  });

  describe("Factory function", () => {
    it("creates an extractor with createExtractor", () => {
      const newExtractor = createExtractor(registry);
      expect(newExtractor).toBeInstanceOf(Extractor);

      const errors = newExtractor.extract(goCompilerError, passthroughParser);
      expect(errors).toHaveLength(1);
    });
  });
});

describe("Unknown pattern reporting", () => {
  beforeEach(() => {
    // Clear the reporter before each test
    setUnknownPatternReporter(undefined);
  });

  it("calls the reporter callback with unknown patterns", () => {
    const mockReporter = vi.fn();
    setUnknownPatternReporter(mockReporter);

    const errors: ExtractedError[] = [
      {
        message: "Some unknown error pattern",
        unknownPattern: true,
        raw: "Some unknown error pattern",
      },
    ];

    reportUnknownPatterns(errors);

    expect(mockReporter).toHaveBeenCalledTimes(1);
    expect(mockReporter).toHaveBeenCalledWith(
      expect.arrayContaining([expect.any(String)])
    );
  });

  it("does not call reporter when no unknown patterns exist", () => {
    const mockReporter = vi.fn();
    setUnknownPatternReporter(mockReporter);

    const errors: ExtractedError[] = [
      {
        message: "Known error pattern",
        unknownPattern: false,
        source: "go",
      },
    ];

    reportUnknownPatterns(errors);

    expect(mockReporter).not.toHaveBeenCalled();
  });

  it("does not call reporter when no reporter is set", () => {
    // No reporter set (default)
    const errors: ExtractedError[] = [
      {
        message: "Unknown pattern",
        unknownPattern: true,
      },
    ];

    // Should not throw
    expect(() => reportUnknownPatterns(errors)).not.toThrow();
  });

  it("limits the number of patterns reported", () => {
    const mockReporter = vi.fn();
    setUnknownPatternReporter(mockReporter);

    // Create more than the limit (10) of unknown patterns
    const errors: ExtractedError[] = [];
    for (let i = 0; i < 20; i++) {
      errors.push({
        message: `Unknown pattern ${i}`,
        unknownPattern: true,
        raw: `Unknown pattern ${i}`,
      });
    }

    reportUnknownPatterns(errors);

    expect(mockReporter).toHaveBeenCalledTimes(1);
    const reportedPatterns = mockReporter.mock.calls[0][0];
    expect(reportedPatterns.length).toBeLessThanOrEqual(10);
  });

  it("truncates long pattern lines", () => {
    const mockReporter = vi.fn();
    setUnknownPatternReporter(mockReporter);

    const longPattern = "x".repeat(1000);
    const errors: ExtractedError[] = [
      {
        message: longPattern,
        unknownPattern: true,
        raw: longPattern,
      },
    ];

    reportUnknownPatterns(errors);

    expect(mockReporter).toHaveBeenCalledTimes(1);
    const reportedPatterns = mockReporter.mock.calls[0][0];
    expect(reportedPatterns[0].length).toBeLessThan(1000);
    expect(reportedPatterns[0]).toContain("...");
  });

  it("getUnknownPatternReporter returns the current reporter", () => {
    const mockReporter = vi.fn();
    setUnknownPatternReporter(mockReporter);

    expect(getUnknownPatternReporter()).toBe(mockReporter);

    setUnknownPatternReporter(undefined);
    expect(getUnknownPatternReporter()).toBeUndefined();
  });
});
