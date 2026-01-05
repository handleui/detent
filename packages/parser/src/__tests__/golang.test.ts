import { beforeEach, describe, expect, it } from "vitest";
import type { ParseContext } from "../parser-types.js";
import { createGolangParser } from "../parsers/golang.js";

const createContext = (
  overrides: Partial<ParseContext> = {}
): ParseContext => ({
  job: "",
  step: "",
  tool: "",
  lastFile: "",
  basePath: "",
  ...overrides,
});

describe("GolangParser", () => {
  let parser: ReturnType<typeof createGolangParser>;

  beforeEach(() => {
    parser = createGolangParser();
  });

  describe("Go compiler errors", () => {
    it("parses standard compiler error with line and column", () => {
      const ctx = createContext();
      const line = "./main.go:10:5: undefined: foo";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0.9);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("./main.go");
      expect(result?.line).toBe(10);
      expect(result?.column).toBe(5);
      expect(result?.message).toBe("undefined: foo");
      expect(result?.category).toBe("compile");
      expect(result?.source).toBe("go");
    });

    it("parses type mismatch error", () => {
      const ctx = createContext();
      const line = "main.go:10:5: cannot use x (type int) as type string";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("main.go");
      expect(result?.line).toBe(10);
      expect(result?.column).toBe(5);
      expect(result?.message).toBe("cannot use x (type int) as type string");
    });

    it("parses error without column number", () => {
      const ctx = createContext();
      const line = "main.go:25: missing return at end of function";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0.9);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("main.go");
      expect(result?.line).toBe(25);
      expect(result?.column).toBeUndefined();
      expect(result?.message).toBe("missing return at end of function");
    });

    it("parses import cycle error", () => {
      const ctx = createContext();
      const line = "pkg/a/a.go:5:2: import cycle not allowed";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toBe("import cycle not allowed");
      expect(result?.category).toBe("compile");
    });

    it("parses build constraint error", () => {
      const ctx = createContext();
      const line =
        "pkg/unix.go:1:1: build constraints exclude all Go files in pkg";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.category).toBe("compile");
    });
  });

  describe("golangci-lint errors", () => {
    it("parses staticcheck error with code", () => {
      const ctx = createContext({ step: "Run golangci-lint" });
      const line = "main.go:10:5: SA1000: regexp.MustCompile(staticcheck)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("main.go");
      expect(result?.line).toBe(10);
      expect(result?.column).toBe(5);
      expect(result?.message).toBe("regexp.MustCompile");
      expect(result?.ruleId).toBe("SA1000/staticcheck");
      expect(result?.category).toBe("lint");
      expect(result?.severity).toBe("error");
    });

    it("parses errcheck linter output", () => {
      const ctx = createContext({ step: "lint" });
      const line =
        "server.go:42:9: Error return value is not checked(errcheck)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toBe("Error return value is not checked");
      expect(result?.ruleId).toBe("errcheck");
      expect(result?.category).toBe("lint");
      expect(result?.severity).toBe("error");
    });

    it("parses gosimple warning", () => {
      const ctx = createContext({ step: "lint" });
      const line =
        "utils.go:15:3: S1000: should use for range instead of for { select {} } (gosimple)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("S1000/gosimple");
      expect(result?.severity).toBe("warning");
    });

    it("parses govet error", () => {
      const ctx = createContext({ step: "lint" });
      const line =
        "handler.go:88:2: printf: Printf format %s has arg of wrong type(govet)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toBe(
        "printf: Printf format %s has arg of wrong type"
      );
      expect(result?.ruleId).toBe("govet");
      expect(result?.severity).toBe("error");
    });

    it("parses ineffassign warning", () => {
      const ctx = createContext({ step: "lint" });
      const line = "main.go:50:2: ineffectual assignment to err (ineffassign)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("ineffassign");
      expect(result?.severity).toBe("error");
    });

    it("parses gocritic warning", () => {
      const ctx = createContext({ step: "lint" });
      const line =
        "service.go:100:5: hugeParam: data is heavy, consider passing by pointer(gocritic)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("gocritic");
      expect(result?.severity).toBe("warning");
    });

    it("parses gosec security error", () => {
      const ctx = createContext({ step: "lint" });
      const line =
        "crypto.go:20:10: G501: Blacklisted import crypto/md5 (gosec)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("G501/gosec");
      expect(result?.severity).toBe("error");
    });

    it("parses stylecheck warning", () => {
      const ctx = createContext({ step: "lint" });
      const line =
        "api.go:30:1: ST1000: at least one file in a package should have a package comment (stylecheck)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("ST1000/stylecheck");
      expect(result?.severity).toBe("warning");
    });

    it("parses revive warning", () => {
      const ctx = createContext({ step: "lint" });
      const line =
        "config.go:45:6: exported: exported type Config should have comment or be unexported (revive)";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("revive");
      expect(result?.severity).toBe("warning");
    });

    it("detects lint category from step context", () => {
      const ctx = createContext({ step: "Run golangci-lint" });
      const line = "main.go:10:5: undefined: foo";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.category).toBe("lint");
    });
  });

  describe("Go test failures", () => {
    it("parses simple test failure", () => {
      const ctx = createContext();
      const line = "--- FAIL: TestFoo (0.00s)";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0.9);

      const result = parser.parse(line, ctx);
      expect(result).toBeNull();

      const continued = parser.continueMultiLine(
        "    foo_test.go:15: expected true, got false",
        ctx
      );
      expect(continued).toBe(true);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.message).toBe("expected true, got false");
      expect(finished?.category).toBe("test");
      expect(finished?.source).toBe("go-test");
      expect(finished?.file).toBe("foo_test.go");
      expect(finished?.line).toBe(15);
    });

    it("parses subtest failure", () => {
      const ctx = createContext();
      parser.parse("--- FAIL: TestFoo/SubTest (0.00s)", ctx);
      parser.continueMultiLine("    foo_test.go:20: assertion failed", ctx);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.file).toBe("foo_test.go");
      expect(finished?.line).toBe(20);
    });

    it("parses test failure with multiple output lines", () => {
      const ctx = createContext();
      parser.parse("--- FAIL: TestComplex (0.05s)", ctx);
      parser.continueMultiLine("    complex_test.go:50: Error Trace:", ctx);
      parser.continueMultiLine("        complex_test.go:50", ctx);
      parser.continueMultiLine("    Error: Not equal:", ctx);
      parser.continueMultiLine("        expected: 1", ctx);
      parser.continueMultiLine("        actual: 2", ctx);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.file).toBe("complex_test.go");
      expect(finished?.stackTrace).toContain("expected: 1");
    });

    it("parses build failure in test package", () => {
      const ctx = createContext();
      const line = "pkg/foo/foo.go:10:5: undefined: bar";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.file).toBe("pkg/foo/foo.go");
      expect(result?.category).toBe("compile");
    });

    it("handles test failure without file reference", () => {
      const ctx = createContext();
      parser.parse("--- FAIL: TestNoFile (0.00s)", ctx);
      parser.continueMultiLine("    some output without file:line", ctx);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.message).toContain("FAIL: TestNoFile");
    });
  });

  describe("multi-line panic/stack traces", () => {
    it("parses simple panic with stack trace", () => {
      const ctx = createContext();
      const line = "panic: runtime error: index out of range [5] with length 3";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0.9);

      parser.parse(line, ctx);

      parser.continueMultiLine("", ctx);
      parser.continueMultiLine("goroutine 1 [running]:", ctx);
      parser.continueMultiLine("main.foo(0x1234)", ctx);
      parser.continueMultiLine("\t/home/user/project/main.go:25 +0x1a2", ctx);
      parser.continueMultiLine("main.main()", ctx);
      parser.continueMultiLine("\t/home/user/project/main.go:10 +0x45", ctx);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.message).toBe(
        "panic: runtime error: index out of range [5] with length 3"
      );
      expect(finished?.category).toBe("runtime");
      expect(finished?.source).toBe("go");
      expect(finished?.file).toBe("/home/user/project/main.go");
      expect(finished?.line).toBe(25);
      expect(finished?.stackTrace).toContain("goroutine 1 [running]:");
    });

    it("parses nil pointer dereference panic", () => {
      const ctx = createContext();
      parser.parse(
        "panic: runtime error: invalid memory address or nil pointer dereference",
        ctx
      );
      parser.continueMultiLine(
        "[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x12345]",
        ctx
      );
      parser.continueMultiLine("", ctx);
      parser.continueMultiLine("goroutine 1 [running]:", ctx);
      parser.continueMultiLine("main.process(0x0)", ctx);
      parser.continueMultiLine("\t/app/handler.go:45 +0x20", ctx);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.message).toContain("nil pointer dereference");
      expect(finished?.file).toBe("/app/handler.go");
      expect(finished?.line).toBe(45);
    });

    it("parses panic with multiple goroutines", () => {
      const ctx = createContext();
      parser.parse("panic: send on closed channel", ctx);
      parser.continueMultiLine("", ctx);
      parser.continueMultiLine("goroutine 18 [running]:", ctx);
      parser.continueMultiLine("main.worker(0xc000010080)", ctx);
      parser.continueMultiLine("\t/app/worker.go:30 +0x100", ctx);
      parser.continueMultiLine("created by main.startWorkers", ctx);
      parser.continueMultiLine("\t/app/main.go:50 +0x80", ctx);
      parser.continueMultiLine("", ctx);
      parser.continueMultiLine("goroutine 1 [chan receive]:", ctx);
      parser.continueMultiLine("main.main()", ctx);
      parser.continueMultiLine("\t/app/main.go:60 +0x200", ctx);

      const result = parser.continueMultiLine("exit status 2", ctx);
      expect(result).toBe(false);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.stackTrace).toContain("goroutine 18");
      expect(finished?.file).toBe("/app/worker.go");
      expect(finished?.line).toBe(30);
    });

    it("parses custom panic message", () => {
      const ctx = createContext();
      parser.parse("panic: custom error: something went wrong", ctx);
      parser.continueMultiLine("", ctx);
      parser.continueMultiLine("goroutine 1 [running]:", ctx);
      parser.continueMultiLine("main.doSomething()", ctx);
      parser.continueMultiLine("\t/app/main.go:15 +0x50", ctx);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.message).toBe(
        "panic: custom error: something went wrong"
      );
    });

    it("handles stack trace with runtime frames", () => {
      const ctx = createContext();
      parser.parse("panic: test panic", ctx);
      parser.continueMultiLine("", ctx);
      parser.continueMultiLine("goroutine 1 [running]:", ctx);
      parser.continueMultiLine("runtime/debug.Stack()", ctx);
      parser.continueMultiLine(
        "\t/usr/local/go/src/runtime/debug/stack.go:24 +0x65",
        ctx
      );
      parser.continueMultiLine("main.panicHandler()", ctx);
      parser.continueMultiLine("\t/app/handler.go:10 +0x20", ctx);

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.file).toBe("/usr/local/go/src/runtime/debug/stack.go");
    });
  });

  describe("Go module errors", () => {
    it("parses go mod error", () => {
      const ctx = createContext();
      const line =
        "go: example.com/foo@v1.2.3: reading example.com/foo/go.mod: 404 Not Found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0.8);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toBe(
        "example.com/foo@v1.2.3: reading example.com/foo/go.mod: 404 Not Found"
      );
      expect(result?.category).toBe("compile");
    });

    it("parses go.mod syntax error via module pattern", () => {
      const ctx = createContext();
      const line = "go.mod:5: unknown directive: foo";

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toBe("unknown directive: foo");
      expect(result?.category).toBe("compile");
    });
  });

  describe("noise detection", () => {
    it("detects go: downloading messages as noise", () => {
      expect(parser.isNoise("go: downloading github.com/foo/bar v1.2.3")).toBe(
        true
      );
      expect(parser.isNoise("go: extracting github.com/foo/bar v1.2.3")).toBe(
        true
      );
    });

    it("detects test run markers as noise", () => {
      expect(parser.isNoise("=== RUN   TestFoo")).toBe(true);
      expect(parser.isNoise("=== PAUSE TestFoo")).toBe(true);
      expect(parser.isNoise("=== CONT  TestFoo")).toBe(true);
      expect(parser.isNoise("=== NAME  TestFoo")).toBe(true);
    });

    it("detects test pass markers as noise", () => {
      expect(parser.isNoise("--- PASS: TestFoo (0.00s)")).toBe(true);
      expect(parser.isNoise("--- SKIP: TestBar (0.00s)")).toBe(true);
      expect(parser.isNoise("PASS")).toBe(true);
    });

    it("detects package pass markers as noise", () => {
      expect(parser.isNoise("ok  	github.com/foo/bar	0.123s")).toBe(true);
      expect(parser.isNoise("?   	github.com/foo/baz	[no test files]")).toBe(
        true
      );
    });

    it("detects build/test context as noise", () => {
      expect(parser.isNoise("# github.com/foo/bar")).toBe(true);
    });

    it("detects log level prefixes as noise", () => {
      expect(parser.isNoise('level=info msg="starting server"')).toBe(true);
      expect(parser.isNoise('level=debug msg="processing"')).toBe(true);
    });

    it("detects coverage messages as noise", () => {
      expect(parser.isNoise("coverage: 85.5% of statements")).toBe(true);
    });

    it("detects golangci-lint info as noise", () => {
      expect(
        parser.isNoise("Running [/usr/local/bin/golangci-lint run ./...]")
      ).toBe(true);
      expect(parser.isNoise("Issues: 0")).toBe(true);
    });

    it("detects nested test pass as noise", () => {
      expect(parser.isNoise("    --- PASS: TestFoo/SubTest (0.00s)")).toBe(
        true
      );
    });

    it("detects package failure summary as noise", () => {
      expect(parser.isNoise("FAIL github.com/foo/bar 0.123s")).toBe(true);
    });

    it("does not mark real errors as noise", () => {
      expect(parser.isNoise("main.go:10:5: undefined: foo")).toBe(false);
      expect(parser.isNoise("--- FAIL: TestFoo (0.00s)")).toBe(false);
      expect(parser.isNoise("panic: runtime error")).toBe(false);
    });
  });

  describe("edge cases", () => {
    describe("Windows paths", () => {
      it("parses error with backslash path", () => {
        const ctx = createContext();
        const line = "pkg\\handler\\main.go:10:5: undefined: foo";

        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.file).toBe("pkg\\handler\\main.go");
        expect(result?.line).toBe(10);
      });

      it("parses error with absolute Unix path", () => {
        const ctx = createContext();
        const line = "/Users/dev/project/main.go:15:2: missing return";

        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.file).toBe("/Users/dev/project/main.go");
      });
    });

    describe("module paths with version suffixes", () => {
      it("handles v2+ module paths", () => {
        const ctx = createContext();
        const line =
          "github.com/foo/bar/v2/pkg/handler.go:25:10: undefined: baz";

        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.file).toBe("github.com/foo/bar/v2/pkg/handler.go");
        expect(result?.line).toBe(25);
      });

      it("handles module path in error message", () => {
        const ctx = createContext();
        const line = "go: github.com/foo/bar/v3@v3.0.0: missing go.sum entry";

        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.message).toContain("v3@v3.0.0");
      });
    });

    describe("generic type errors (Go 1.18+)", () => {
      it("parses generic type constraint error", () => {
        const ctx = createContext();
        const line = "main.go:10:5: int does not satisfy comparable";

        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.message).toBe("int does not satisfy comparable");
      });

      it("parses generic instantiation error", () => {
        const ctx = createContext();
        const line =
          "main.go:15:2: cannot use generic type List[T any] without instantiation";

        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.message).toContain("generic type List[T any]");
      });

      it("parses type parameter error", () => {
        const ctx = createContext();
        const line = "generics.go:20:8: type parameter T is not used";

        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.message).toBe("type parameter T is not used");
      });
    });

    describe("ANSI escape codes", () => {
      it("strips ANSI codes from error line", () => {
        const ctx = createContext();
        const line = "\x1b[31mmain.go:10:5: undefined: foo\x1b[0m";

        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.file).toBe("main.go");
        expect(result?.message).toBe("undefined: foo");
      });
    });

    describe("very long lines", () => {
      it("rejects extremely long lines for security", () => {
        const ctx = createContext();
        const longLine = `main.go:10:5: ${"a".repeat(5000)}`;

        expect(parser.canParse(longLine, ctx)).toBe(0);
      });
    });

    describe("parser reset", () => {
      it("resets multi-line state correctly", () => {
        const ctx = createContext();
        parser.parse("panic: test", ctx);
        parser.continueMultiLine("goroutine 1 [running]:", ctx);

        parser.reset();

        const result = parser.finishMultiLine(ctx);
        expect(result).toBeNull();
      });

      it("can parse new error after reset", () => {
        const ctx = createContext();
        parser.parse("panic: first panic", ctx);
        parser.reset();

        const line = "main.go:10:5: new error";
        const result = parser.parse(line, ctx);
        expect(result).not.toBeNull();
        expect(result?.message).toBe("new error");
      });
    });

    describe("empty and whitespace lines", () => {
      it("handles empty lines in panic stack trace", () => {
        const ctx = createContext();
        parser.parse("panic: test", ctx);
        const continued = parser.continueMultiLine("", ctx);
        expect(continued).toBe(true);
      });

      it("handles whitespace-only lines in test output", () => {
        const ctx = createContext();
        parser.parse("--- FAIL: TestFoo (0.00s)", ctx);
        const continued = parser.continueMultiLine("    ", ctx);
        expect(continued).toBe(true);
      });
    });
  });

  describe("severity determination", () => {
    it("assigns error severity to staticcheck SA codes", () => {
      const ctx = createContext({ step: "lint" });
      const line = "main.go:10:5: SA1000: some issue (staticcheck)";

      const result = parser.parse(line, ctx);
      expect(result?.severity).toBe("error");
    });

    it("assigns warning severity to stylecheck ST codes", () => {
      const ctx = createContext({ step: "lint" });
      const line = "main.go:10:5: ST1000: some style issue (stylecheck)";

      const result = parser.parse(line, ctx);
      expect(result?.severity).toBe("warning");
    });

    it("assigns warning severity to gosimple S codes", () => {
      const ctx = createContext({ step: "lint" });
      const line = "main.go:10:5: S1000: can be simplified (gosimple)";

      const result = parser.parse(line, ctx);
      expect(result?.severity).toBe("warning");
    });

    it("assigns error severity to gosec G codes", () => {
      const ctx = createContext({ step: "lint" });
      const line = "main.go:10:5: G101: security issue (gosec)";

      const result = parser.parse(line, ctx);
      expect(result?.severity).toBe("error");
    });

    it("defaults to error for unknown linters", () => {
      const ctx = createContext({ step: "lint" });
      const line = "main.go:10:5: some issue (unknownlinter)";

      const result = parser.parse(line, ctx);
      expect(result?.severity).toBe("error");
    });
  });

  describe("lineKnown and columnKnown flags", () => {
    it("sets lineKnown true when line is present", () => {
      const ctx = createContext();
      const line = "main.go:10:5: error";

      const result = parser.parse(line, ctx);
      expect(result?.lineKnown).toBe(true);
    });

    it("sets columnKnown true when column is present", () => {
      const ctx = createContext();
      const line = "main.go:10:5: error";

      const result = parser.parse(line, ctx);
      expect(result?.columnKnown).toBe(true);
    });

    it("sets columnKnown false when column is absent", () => {
      const ctx = createContext();
      const line = "main.go:10: error";

      const result = parser.parse(line, ctx);
      expect(result?.columnKnown).toBe(false);
    });
  });

  describe("multi-line resource limits", () => {
    it("handles stack trace truncation", () => {
      const ctx = createContext();
      parser.parse("panic: test", ctx);
      parser.continueMultiLine("", ctx);
      parser.continueMultiLine("goroutine 1 [running]:", ctx);

      for (let i = 0; i < 600; i++) {
        parser.continueMultiLine(`frame${i}()`, ctx);
        parser.continueMultiLine(`\t/file${i}.go:${i} +0x0`, ctx);
      }

      const finished = parser.finishMultiLine(ctx);
      expect(finished).not.toBeNull();
      expect(finished?.stackTraceTruncated).toBe(true);
    });
  });

  describe("canParse confidence levels", () => {
    it("returns high confidence for exact error pattern", () => {
      const ctx = createContext();
      expect(
        parser.canParse("main.go:10:5: error", ctx)
      ).toBeGreaterThanOrEqual(0.95);
    });

    it("returns slightly lower confidence for no-column pattern", () => {
      const ctx = createContext();
      expect(parser.canParse("main.go:10: error", ctx)).toBeGreaterThanOrEqual(
        0.93
      );
    });

    it("returns high confidence for test fail pattern", () => {
      const ctx = createContext();
      expect(
        parser.canParse("--- FAIL: TestFoo (0.00s)", ctx)
      ).toBeGreaterThanOrEqual(0.95);
    });

    it("returns high confidence for panic pattern", () => {
      const ctx = createContext();
      expect(
        parser.canParse("panic: something bad", ctx)
      ).toBeGreaterThanOrEqual(0.95);
    });

    it("returns moderate confidence for module errors", () => {
      const ctx = createContext();
      expect(parser.canParse("go: module error", ctx)).toBeGreaterThanOrEqual(
        0.9
      );
    });

    it("returns zero for non-matching lines", () => {
      const ctx = createContext();
      expect(parser.canParse("just some random text", ctx)).toBe(0);
      expect(parser.canParse("error: something without file", ctx)).toBe(0);
    });
  });

  describe("parser properties", () => {
    it("has correct id", () => {
      expect(parser.id).toBe("go");
    });

    it("has correct priority", () => {
      expect(parser.priority).toBe(80);
    });

    it("supports multi-line parsing", () => {
      expect(parser.supportsMultiLine()).toBe(true);
    });
  });
});
