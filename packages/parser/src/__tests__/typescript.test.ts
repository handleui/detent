import { beforeEach, describe, expect, it } from "vitest";
import { createParseContext, type ParseContext } from "../parser-types.js";
import {
  createTypeScriptParser,
  TypeScriptParser,
} from "../parsers/typescript.js";

describe("TypeScriptParser", () => {
  let parser: TypeScriptParser;
  let ctx: ParseContext;

  beforeEach(() => {
    parser = createTypeScriptParser();
    ctx = createParseContext();
  });

  describe("factory function", () => {
    it("creates a parser instance", () => {
      const p = createTypeScriptParser();
      expect(p).toBeInstanceOf(TypeScriptParser);
      expect(p.id).toBe("typescript");
      expect(p.priority).toBe(80);
    });

    it("supports multi-line parsing", () => {
      expect(parser.supportsMultiLine()).toBe(true);
    });
  });

  describe("basic TSC errors - parenthesized format", () => {
    it("parses file.ts(1,2): error TS2304: Cannot find name 'foo'", () => {
      const line = "file.ts(1,2): error TS2304: Cannot find name 'foo'";
      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("file.ts");
      expect(result?.line).toBe(1);
      expect(result?.column).toBe(2);
      expect(result?.ruleId).toBe("TS2304");
      expect(result?.message).toBe("Cannot find name 'foo'");
      expect(result?.severity).toBe("error");
      expect(result?.source).toBe("typescript");
    });

    it("parses path/to/component.tsx(10,15): error TS2339: Property 'x' does not exist", () => {
      const line =
        "path/to/component.tsx(10,15): error TS2339: Property 'x' does not exist on type 'Props'";
      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("path/to/component.tsx");
      expect(result?.line).toBe(10);
      expect(result?.column).toBe(15);
      expect(result?.ruleId).toBe("TS2339");
      expect(result?.message).toBe(
        "Property 'x' does not exist on type 'Props'"
      );
    });

    it("parses warning severity", () => {
      const line =
        "src/app.ts(5,10): warning TS6133: 'x' is declared but its value is never read.";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.severity).toBe("warning");
      expect(result?.ruleId).toBe("TS6133");
    });

    it("parses without error code", () => {
      const line = "src/app.ts(1,1): Some error message without code";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("src/app.ts");
      expect(result?.line).toBe(1);
      expect(result?.column).toBe(1);
      expect(result?.message).toBe("Some error message without code");
      expect(result?.ruleId).toBeUndefined();
    });
  });

  describe("basic TSC errors - colon-separated format (pretty mode)", () => {
    it("parses file.ts:1:2 - error TS2304: Cannot find name 'foo'", () => {
      const line = "file.ts:1:2 - error TS2304: Cannot find name 'foo'";
      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      parser.reset();
      parser.parse(line, ctx);
      const result = parser.finishMultiLine(ctx);

      expect(result).not.toBeNull();
      expect(result?.file).toBe("file.ts");
      expect(result?.line).toBe(1);
      expect(result?.column).toBe(2);
      expect(result?.ruleId).toBe("TS2304");
      expect(result?.message).toBe("Cannot find name 'foo'");
      expect(result?.severity).toBe("error");
    });

    it("parses colon-separated warning", () => {
      const line =
        "src/utils.ts:100:5 - warning TS6133: 'unused' is declared but never used.";

      parser.reset();
      parser.parse(line, ctx);
      const result = parser.finishMultiLine(ctx);

      expect(result).not.toBeNull();
      expect(result?.file).toBe("src/utils.ts");
      expect(result?.line).toBe(100);
      expect(result?.column).toBe(5);
      expect(result?.severity).toBe("warning");
    });

    it("parses without error code in colon format", () => {
      const line = "app.tsx:42:8 - error Some message";

      parser.reset();
      parser.parse(line, ctx);
      const result = parser.finishMultiLine(ctx);

      expect(result).not.toBeNull();
      expect(result?.file).toBe("app.tsx");
      expect(result?.line).toBe(42);
      expect(result?.column).toBe(8);
      expect(result?.ruleId).toBeUndefined();
    });
  });

  describe("global errors (no file location)", () => {
    it("parses error TS5023: Unknown compiler option 'foo'", () => {
      const line = "error TS5023: Unknown compiler option 'foo'.";
      expect(parser.canParse(line, ctx)).toBe(0.95);
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBeUndefined();
      expect(result?.line).toBeUndefined();
      expect(result?.column).toBeUndefined();
      expect(result?.ruleId).toBe("TS5023");
      expect(result?.message).toBe("Unknown compiler option 'foo'.");
      expect(result?.severity).toBe("error");
      expect(result?.category).toBe("config");
    });

    it("parses warning global error", () => {
      const line =
        "warning TS6059: File 'path/file.ts' is not under 'rootDir'.";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.severity).toBe("warning");
      expect(result?.ruleId).toBe("TS6059");
    });
  });

  describe("various TS error codes", () => {
    const errorCodes: Array<{
      code: string;
      message: string;
      expectedCategory: string;
    }> = [
      {
        code: "TS2304",
        message: "Cannot find name 'foo'",
        expectedCategory: "type-check",
      },
      {
        code: "TS2339",
        message: "Property 'x' does not exist on type 'Y'",
        expectedCategory: "type-check",
      },
      {
        code: "TS2345",
        message:
          "Argument of type 'A' is not assignable to parameter of type 'B'",
        expectedCategory: "type-check",
      },
      {
        code: "TS7006",
        message: "Parameter 'x' implicitly has an 'any' type",
        expectedCategory: "type-check",
      },
      {
        code: "TS2322",
        message: "Type 'string' is not assignable to type 'number'",
        expectedCategory: "type-check",
      },
      {
        code: "TS2551",
        message: "Property 'x' does not exist. Did you mean 'y'?",
        expectedCategory: "type-check",
      },
      {
        code: "TS2349",
        message: "Cannot invoke expression",
        expectedCategory: "type-check",
      },
      {
        code: "TS2307",
        message: "Cannot find module 'unknown-module'",
        expectedCategory: "type-check",
      },
    ];

    for (const { code, message, expectedCategory } of errorCodes) {
      it(`parses ${code} with correct category`, () => {
        const line = `src/file.ts(1,1): error ${code}: ${message}`;
        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.ruleId).toBe(code);
        expect(result?.category).toBe(expectedCategory);
      });
    }
  });

  describe("edge cases - Windows paths", () => {
    it("parses Windows path with backslashes (parenthesized format)", () => {
      const line =
        "C:\\project\\src\\file.ts(10,5): error TS2304: Cannot find name 'x'";
      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("C:\\project\\src\\file.ts");
      expect(result?.line).toBe(10);
      expect(result?.column).toBe(5);
    });

    it("does not parse Windows path in colon format (ambiguous with line:col)", () => {
      // Windows paths with drive letters are ambiguous in colon format
      // because D:\path:42:8 has multiple colons that conflict with line:col parsing
      const line =
        "D:\\Users\\dev\\app.tsx:42:8 - error TS2339: Property 'x' does not exist";
      // The pattern [^\s:] excludes colons, so Windows paths won't match colon format
      expect(parser.canParse(line, ctx)).toBe(0);
    });

    it("parses Windows UNC path (parenthesized format)", () => {
      const line =
        "\\\\server\\share\\project\\file.ts(1,1): error TS2304: Cannot find name";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("\\\\server\\share\\project\\file.ts");
    });
  });

  describe("edge cases - paths with spaces", () => {
    it("does not parse paths with spaces (security - prevents ReDoS)", () => {
      // Paths with spaces are intentionally not supported
      // The pattern [^\s(] excludes spaces for security and performance
      const line =
        "path/with spaces/my file.ts(5,10): error TS2304: Cannot find name 'foo'";
      expect(parser.canParse(line, ctx)).toBe(0);
      expect(parser.parse(line, ctx)).toBeNull();
    });

    it("parses paths with dashes and underscores", () => {
      const line =
        "path/with-dashes/my_file.ts(5,10): error TS2304: Cannot find name 'foo'";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("path/with-dashes/my_file.ts");
    });
  });

  describe("edge cases - very long file paths", () => {
    it("parses very long file path", () => {
      const longPath = `${"a/".repeat(100)}file.ts`;
      const line = `${longPath}(1,1): error TS2304: Cannot find name 'x'`;
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe(longPath);
    });

    it("rejects overly long lines (security - ReDoS prevention)", () => {
      const veryLongLine = `${"a".repeat(5000)}.ts(1,1): error TS2304: message`;
      expect(parser.canParse(veryLongLine, ctx)).toBe(0);
      expect(parser.parse(veryLongLine, ctx)).toBeNull();
    });
  });

  describe("edge cases - unicode in paths and messages", () => {
    it("parses path with unicode characters", () => {
      const line =
        "src/componentes/BotaoAcao.tsx(10,5): error TS2304: Cannot find name 'foo'";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("src/componentes/BotaoAcao.tsx");
    });

    it("parses message with unicode characters", () => {
      const line = "file.ts(1,1): error TS2304: Cannot find name 'usuario'";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toBe("Cannot find name 'usuario'");
    });

    it("parses message with emoji", () => {
      const line = "file.ts(1,1): error TS2304: Missing property in type";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
    });
  });

  describe("edge cases - multiple errors per file", () => {
    it("parses multiple consecutive errors from same file", () => {
      const lines = [
        "src/app.ts(1,1): error TS2304: Cannot find name 'a'",
        "src/app.ts(2,1): error TS2304: Cannot find name 'b'",
        "src/app.ts(3,1): error TS2304: Cannot find name 'c'",
      ];

      const results: ReturnType<typeof parser.parse>[] = [];
      for (const line of lines) {
        parser.reset();
        const result = parser.parse(line, ctx);
        if (result) {
          results.push(result);
        }
      }

      expect(results).toHaveLength(3);
      expect(results[0]?.line).toBe(1);
      expect(results[0]?.message).toContain("'a'");
      expect(results[1]?.line).toBe(2);
      expect(results[1]?.message).toContain("'b'");
      expect(results[2]?.line).toBe(3);
      expect(results[2]?.message).toContain("'c'");
    });
  });

  describe("edge cases - TypeScript file extensions", () => {
    const extensions = [
      ".ts",
      ".tsx",
      ".mts",
      ".cts",
      ".d.ts",
      ".d.tsx",
      ".d.mts",
      ".d.cts",
    ];

    for (const ext of extensions) {
      it(`parses ${ext} file extension`, () => {
        const line = `src/types${ext}(1,1): error TS2304: Cannot find name 'x'`;
        expect(parser.canParse(line, ctx)).toBeGreaterThan(0);
        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.file).toBe(`src/types${ext}`);
      });
    }
  });

  describe("noise detection", () => {
    it("detects empty lines as noise", () => {
      expect(parser.isNoise("")).toBe(true);
      expect(parser.isNoise("   ")).toBe(true);
      expect(parser.isNoise("\t")).toBe(true);
    });

    it("detects TSC version output as noise", () => {
      expect(parser.isNoise("Version 5.0.4")).toBe(true);
      expect(parser.isNoise("version 4.9.5")).toBe(true);
    });

    it("detects watch mode messages as noise", () => {
      expect(parser.isNoise("Starting compilation in watch mode...")).toBe(
        true
      );
      expect(
        parser.isNoise(
          "File change detected. Starting incremental compilation..."
        )
      ).toBe(true);
      expect(parser.isNoise("Watching for file changes.")).toBe(true);
      expect(parser.isNoise("[12:34:56 PM] Starting compilation...")).toBe(
        true
      );
      expect(parser.isNoise("[1:23:45 AM] File change detected")).toBe(true);
    });

    it("detects 'Found X errors' messages as noise", () => {
      expect(parser.isNoise("Found 5 errors.")).toBe(true);
      expect(parser.isNoise("Found 1 error.")).toBe(true);
      expect(parser.isNoise("Found 10 errors in 3 files.")).toBe(true);
    });

    it("detects build mode messages as noise", () => {
      expect(parser.isNoise("Building project 'tsconfig.json'...")).toBe(true);
      expect(parser.isNoise("Project 'src/tsconfig.json' is up to date")).toBe(
        true
      );
      expect(parser.isNoise("Projects in this build: 5")).toBe(true);
      expect(parser.isNoise("* path/to/tsconfig.json")).toBe(true);
    });

    it("detects info messages as noise", () => {
      expect(parser.isNoise("message TS6194: Found 0 errors.")).toBe(true);
      expect(parser.isNoise("info TS6194: Found 0 errors.")).toBe(true);
    });

    it("does not mark actual errors as noise", () => {
      expect(
        parser.isNoise("file.ts(1,1): error TS2304: Cannot find name 'x'")
      ).toBe(false);
      expect(parser.isNoise("error TS5023: Unknown compiler option")).toBe(
        false
      );
    });
  });

  describe("category inference", () => {
    it("maps TS1xxx (syntax errors) to compile", () => {
      const line = "file.ts(1,1): error TS1005: ';' expected.";
      const result = parser.parse(line, ctx);
      expect(result?.category).toBe("compile");
    });

    it("maps TS2xxx (semantic errors) to type-check", () => {
      const line = "file.ts(1,1): error TS2304: Cannot find name 'x'";
      const result = parser.parse(line, ctx);
      expect(result?.category).toBe("type-check");
    });

    it("maps TS5xxx (compiler options) to config", () => {
      const line = "error TS5023: Unknown compiler option 'strictNullChecks'.";
      const result = parser.parse(line, ctx);
      expect(result?.category).toBe("config");
    });

    it("maps TS6xxx (message catalog) to metadata", () => {
      const line =
        "file.ts(1,1): error TS6133: 'x' is declared but its value is never read.";
      const result = parser.parse(line, ctx);
      expect(result?.category).toBe("metadata");
    });

    it("maps TS7xxx (strict null checks) to type-check", () => {
      const line =
        "file.ts(1,1): error TS7006: Parameter 'x' implicitly has an 'any' type.";
      const result = parser.parse(line, ctx);
      expect(result?.category).toBe("type-check");
    });

    it("maps TS3xxx (declaration emit) to compile", () => {
      const line =
        "file.ts(1,1): error TS3001: Cannot export a declaration file.";
      const result = parser.parse(line, ctx);
      expect(result?.category).toBe("compile");
    });

    it("maps unknown error codes to type-check", () => {
      const line = "file.ts(1,1): error TS9999: Unknown error.";
      const result = parser.parse(line, ctx);
      expect(result?.category).toBe("type-check");
    });

    it("maps errors without code to type-check", () => {
      const line = "file.ts(1,1): Some error message";
      const result = parser.parse(line, ctx);
      expect(result?.category).toBe("type-check");
    });
  });

  describe("suggestions extraction", () => {
    it("extracts Did you mean suggestion", () => {
      const line =
        "file.ts(1,1): error TS2551: Property 'naem' does not exist on type 'User'. Did you mean 'name'?";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.suggestions).toContain("name");
    });

    it("extracts suggestion with double quotes", () => {
      const line =
        "file.ts(1,1): error TS2551: Property 'x' does not exist. Did you mean \"y\"?";
      const result = parser.parse(line, ctx);
      expect(result?.suggestions).toContain("y");
    });
  });

  describe("multi-line error handling", () => {
    it("accumulates context lines for pretty output", () => {
      const errorLine =
        "src/app.ts:10:5 - error TS2339: Property 'foo' does not exist on type 'Bar'.";

      parser.reset();
      parser.parse(errorLine, ctx);

      expect(parser.continueMultiLine("10   const x = obj.foo;", ctx)).toBe(
        true
      );
      expect(parser.continueMultiLine("              ~~~", ctx)).toBe(true);

      const result = parser.finishMultiLine(ctx);
      expect(result).not.toBeNull();
      expect(result?.stackTrace).toContain("const x = obj.foo");
    });

    it("detects new error as end of multi-line", () => {
      const errorLine1 = "src/app.ts:10:5 - error TS2339: First error";
      const errorLine2 = "src/app.ts:20:5 - error TS2339: Second error";

      parser.reset();
      parser.parse(errorLine1, ctx);

      expect(parser.continueMultiLine(errorLine2, ctx)).toBe(false);
    });

    it("finalizes error when input ends", () => {
      const line = "src/app.ts:10:5 - error TS2339: Property does not exist";

      parser.reset();
      parser.parse(line, ctx);

      const result = parser.finishMultiLine(ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toBe("Property does not exist");
    });
  });

  describe("reset behavior", () => {
    it("clears state after reset", () => {
      const line = "src/app.ts:10:5 - error TS2339: Property does not exist";
      parser.parse(line, ctx);
      parser.reset();

      const result = parser.finishMultiLine(ctx);
      expect(result).toBeNull();
    });
  });

  describe("canParse confidence levels", () => {
    it("returns 0.95 for global errors (highest specificity)", () => {
      const line = "error TS5023: Unknown compiler option 'foo'.";
      expect(parser.canParse(line, ctx)).toBe(0.95);
    });

    it("returns 0.92 for colon-separated format", () => {
      const line = "file.ts:1:2 - error TS2304: Cannot find name 'foo'";
      expect(parser.canParse(line, ctx)).toBe(0.92);
    });

    it("returns 0.9 for parenthesized format", () => {
      const line = "file.ts(1,2): error TS2304: Cannot find name 'foo'";
      expect(parser.canParse(line, ctx)).toBe(0.9);
    });

    it("returns 0 for non-TypeScript lines", () => {
      expect(parser.canParse("Some random text", ctx)).toBe(0);
      expect(parser.canParse("go: file.go:10:5: undefined: foo", ctx)).toBe(0);
    });

    it("returns confidence for context lines when in multi-line error", () => {
      const errorLine =
        "src/app.ts:10:5 - error TS2339: Property does not exist";
      parser.parse(errorLine, ctx);

      expect(parser.canParse("10   const x = foo;", ctx)).toBe(0.85);
      expect(parser.canParse("         ~~~", ctx)).toBe(0.85);
    });
  });

  describe("ANSI color code handling", () => {
    it("strips ANSI codes from error lines", () => {
      const line =
        "\x1b[31mfile.ts(1,1): error TS2304: Cannot find name 'x'\x1b[0m";
      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toBe("Cannot find name 'x'");
    });

    it("handles colored error codes", () => {
      const line =
        "file.ts(1,1): \x1b[1;31merror\x1b[0m \x1b[36mTS2304\x1b[0m: Cannot find name";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("TS2304");
    });
  });

  describe("noisePatterns provider", () => {
    it("provides noise patterns for registry optimization", () => {
      const patterns = parser.noisePatterns();

      expect(patterns.fastPrefixes).toContain("version ");
      expect(patterns.fastPrefixes).toContain("found ");
      expect(patterns.fastPrefixes).toContain("starting compilation");

      expect(patterns.fastContains).toContain("up to date");

      expect(patterns.regex.length).toBeGreaterThan(0);
    });
  });

  describe("lineKnown and columnKnown flags", () => {
    it("sets lineKnown true when line is present", () => {
      const line = "file.ts(10,5): error TS2304: Cannot find name 'x'";
      const result = parser.parse(line, ctx);
      expect(result?.lineKnown).toBe(true);
      expect(result?.columnKnown).toBe(true);
    });

    it("sets lineKnown false for global errors", () => {
      const line = "error TS5023: Unknown compiler option.";
      const result = parser.parse(line, ctx);
      expect(result?.lineKnown).toBe(false);
      expect(result?.columnKnown).toBe(false);
    });
  });

  describe("message truncation", () => {
    it("truncates very long messages", () => {
      const longMessage = "x".repeat(2000);
      const line = `file.ts(1,1): error TS2304: ${longMessage}`;
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message.length).toBeLessThanOrEqual(1024);
      expect(result?.messageTruncated).toBe(true);
    });

    it("does not truncate normal messages", () => {
      const line = "file.ts(1,1): error TS2304: Cannot find name 'x'";
      const result = parser.parse(line, ctx);
      expect(result?.messageTruncated).toBeUndefined();
    });
  });

  describe("workflow context", () => {
    it("applies workflow context when provided", () => {
      const ctxWithWorkflow = createParseContext({
        job: "test-job",
        step: "type-check",
      });

      const line = "file.ts(1,1): error TS2304: Cannot find name 'x'";
      const result = parser.parse(line, ctxWithWorkflow);

      expect(result?.workflowContext?.job).toBe("test-job");
      expect(result?.workflowContext?.step).toBe("type-check");
    });
  });

  describe("sequential error parsing (finalize pending error)", () => {
    it("finalizes pending colon error when new parenthesized error arrives", () => {
      const line1 = "src/a.ts:1:1 - error TS2304: First error";
      const line2 = "src/b.ts(2,2): error TS2304: Second error";

      parser.reset();
      parser.parse(line1, ctx);

      const firstResult = parser.parse(line2, ctx);
      expect(firstResult).not.toBeNull();
      expect(firstResult?.file).toBe("src/a.ts");
      expect(firstResult?.message).toBe("First error");

      const secondResult = parser.finishMultiLine(ctx);
      expect(secondResult).not.toBeNull();
      expect(secondResult?.file).toBe("src/b.ts");
    });

    it("returns immediately for parenthesized errors (no multi-line)", () => {
      // Parenthesized format doesn't wait for multi-line context
      // It returns immediately unlike colon format
      const line1 = "src/a.ts(1,1): error TS2304: First error";
      const line2 = "src/b.ts:2:2 - error TS2304: Second error";

      parser.reset();
      const firstResult = parser.parse(line1, ctx);
      expect(firstResult).not.toBeNull();
      expect(firstResult?.file).toBe("src/a.ts");

      // Parse second line starts new multi-line context
      parser.parse(line2, ctx);
      const secondResult = parser.finishMultiLine(ctx);
      expect(secondResult).not.toBeNull();
      expect(secondResult?.file).toBe("src/b.ts");
    });

    it("finalizes pending error when global error arrives", () => {
      const line1 = "src/a.ts:1:1 - error TS2304: First error";
      const line2 = "error TS5023: Unknown compiler option.";

      parser.reset();
      parser.parse(line1, ctx);

      const firstResult = parser.parse(line2, ctx);
      expect(firstResult).not.toBeNull();
      expect(firstResult?.file).toBe("src/a.ts");
    });
  });

  describe("edge cases - large line and column numbers", () => {
    it("parses very large line numbers", () => {
      const line = "file.ts(99999,1): error TS2304: Cannot find name 'x'";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.line).toBe(99_999);
    });

    it("parses very large column numbers", () => {
      const line = "file.ts(1,99999): error TS2304: Cannot find name 'x'";
      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.column).toBe(99_999);
    });
  });

  describe("JSON output detection as noise", () => {
    it("detects JSON diagnostic output as noise", () => {
      expect(
        parser.isNoise('{"code": 2304, "message": "Cannot find name"}')
      ).toBe(true);
      expect(parser.isNoise('[{"code": 2304}]')).toBe(true);
    });
  });
});
