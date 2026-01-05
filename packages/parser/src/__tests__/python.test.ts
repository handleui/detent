import { beforeEach, describe, expect, it } from "vitest";
import { createParseContext, type ParseContext } from "../parser-types.js";
import { createPythonParser } from "../parsers/python.js";
import type { ExtractedError } from "../types.js";

describe("PythonParser", () => {
  let parser: ReturnType<typeof createPythonParser>;
  let ctx: ParseContext;

  beforeEach(() => {
    parser = createPythonParser();
    ctx = createParseContext();
  });

  describe("pytest errors", () => {
    describe("FAILED markers", () => {
      it("parses pytest FAILED with AssertionError", () => {
        const line =
          "FAILED tests/test_math.py::test_addition - AssertionError: assert 1 == 2";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toBe(
          "Test failed: test_addition - AssertionError: assert 1 == 2"
        );
        expect(result.file).toBe("tests/test_math.py");
        expect(result.severity).toBe("error");
        expect(result.category).toBe("test");
        expect(result.source).toBe("python");
        expect(result.ruleId).toBe("test_addition");
      });

      it("parses pytest FAILED with TypeError", () => {
        const line =
          "FAILED tests/test_api.py::TestClient::test_post - TypeError: missing required argument";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toContain("Test failed: TestClient::test_post");
        expect(result.file).toBe("tests/test_api.py");
        expect(result.category).toBe("test");
      });

      it("parses pytest FAILED with parameterized test", () => {
        const line =
          "FAILED tests/test_calc.py::test_divide[10-0] - ZeroDivisionError: division by zero";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toContain("test_divide[10-0]");
        expect(result.file).toBe("tests/test_calc.py");
      });
    });

    describe("ERROR markers (collection errors)", () => {
      it("parses pytest ERROR with ModuleNotFoundError", () => {
        const line =
          "ERROR tests/test_imports.py - ModuleNotFoundError: No module named 'nonexistent'";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toBe(
          "Collection error: ModuleNotFoundError: No module named 'nonexistent'"
        );
        expect(result.file).toBe("tests/test_imports.py");
        expect(result.severity).toBe("error");
        expect(result.category).toBe("test");
      });

      it("parses pytest ERROR with ImportError", () => {
        const line =
          "ERROR tests/test_db.py - ImportError: cannot import name 'DB' from 'database'";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toContain("Collection error:");
        expect(result.file).toBe("tests/test_db.py");
      });
    });
  });

  describe("mypy errors", () => {
    it("parses mypy error with type-arg code", () => {
      const line =
        'app/models.py:42: error: Missing type parameters for generic type "Dict" [type-arg]';
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toBe(
        'Missing type parameters for generic type "Dict"'
      );
      expect(result.file).toBe("app/models.py");
      expect(result.line).toBe(42);
      expect(result.severity).toBe("error");
      expect(result.category).toBe("type-check");
      expect(result.source).toBe("python");
      expect(result.ruleId).toBe("type-arg");
    });

    it("parses mypy error with assignment code", () => {
      const line =
        'config.py:15: error: Incompatible types in assignment (expression has type "str", variable has type "int") [assignment]';
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toContain("Incompatible types in assignment");
      expect(result.file).toBe("config.py");
      expect(result.line).toBe(15);
      expect(result.ruleId).toBe("assignment");
    });

    it("parses mypy error with return-value code", () => {
      const line =
        'utils/helpers.py:88: error: Incompatible return value type (got "None", expected "str") [return-value]';
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.ruleId).toBe("return-value");
      expect(result.severity).toBe("error");
    });

    it("parses mypy warning", () => {
      const line =
        "deprecated.py:5: warning: Function is deprecated [deprecated]";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.severity).toBe("warning");
    });

    it("parses mypy note as warning", () => {
      const line = 'hints.py:10: note: Revealed type is "builtins.str"';
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.severity).toBe("warning");
    });

    it("parses mypy error without error code", () => {
      const line = 'simple.py:1: error: Name "undefined_var" is not defined';
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toBe('Name "undefined_var" is not defined');
      expect(result.ruleId).toBeUndefined();
    });

    it("parses mypy error for .pyi stub file", () => {
      const line =
        "stubs/mymodule.pyi:20: error: Missing return statement [return]";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.file).toBe("stubs/mymodule.pyi");
      expect(result.ruleId).toBe("return");
    });
  });

  describe("Ruff/Flake8 errors", () => {
    describe("Ruff format (with column)", () => {
      it("parses E501 Line too long", () => {
        const line =
          "app/views.py:42:120: E501 Line too long (150 > 120 characters)";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toBe("Line too long (150 > 120 characters)");
        expect(result.file).toBe("app/views.py");
        expect(result.line).toBe(42);
        expect(result.column).toBe(120);
        expect(result.severity).toBe("warning");
        expect(result.category).toBe("lint");
        expect(result.source).toBe("python");
        expect(result.ruleId).toBe("E501");
      });

      it("parses F401 unused import as error", () => {
        const line =
          "main.py:1:1: F401 `os` imported but unused; consider removing";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.ruleId).toBe("F401");
        expect(result.severity).toBe("error");
      });

      it("parses F821 undefined name as error", () => {
        const line = "script.py:10:5: F821 Undefined name `undefined_func`";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.ruleId).toBe("F821");
        expect(result.severity).toBe("error");
      });

      it("parses W293 whitespace warning", () => {
        const line = "format.py:5:1: W293 blank line contains whitespace";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.severity).toBe("warning");
        expect(result.ruleId).toBe("W293");
      });

      it("parses B006 mutable default argument", () => {
        const line =
          "functions.py:15:20: B006 Do not use mutable data structures for argument defaults";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.ruleId).toBe("B006");
        expect(result.severity).toBe("warning");
      });

      it("parses I001 isort issue", () => {
        const line =
          "imports.py:1:1: I001 Import block is un-sorted or un-formatted";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.ruleId).toBe("I001");
        expect(result.severity).toBe("warning");
      });

      it("parses S101 bandit assert warning", () => {
        const line = "test_utils.py:50:5: S101 Use of assert detected";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.ruleId).toBe("S101");
        expect(result.severity).toBe("warning");
      });
    });

    describe("Flake8 format (with column)", () => {
      it("parses flake8 E302 two blank lines", () => {
        const line = "module.py:10:1: E302 expected 2 blank lines, found 1";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toBe("expected 2 blank lines, found 1");
        expect(result.ruleId).toBe("E302");
      });

      it("parses flake8 E999 syntax error", () => {
        const line = "broken.py:5:10: E999 SyntaxError: invalid syntax";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.ruleId).toBe("E999");
        expect(result.severity).toBe("error");
      });
    });

    describe("Ruff/Flake8 without column", () => {
      it("parses error without column", () => {
        const line = "legacy.py:25: E501 Line too long";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.file).toBe("legacy.py");
        expect(result.line).toBe(25);
        expect(result.column).toBeUndefined();
        expect(result.columnKnown).toBe(false);
        expect(result.ruleId).toBe("E501");
      });
    });
  });

  describe("Pylint errors", () => {
    it("parses C0114 missing module docstring", () => {
      const line =
        "mymodule.py:1:0: C0114: Missing module docstring (missing-module-docstring)";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toBe("Missing module docstring");
      expect(result.file).toBe("mymodule.py");
      expect(result.line).toBe(1);
      expect(result.column).toBe(0);
      expect(result.severity).toBe("warning");
      expect(result.category).toBe("lint");
      expect(result.source).toBe("python");
      expect(result.ruleId).toBe("missing-module-docstring");
    });

    it("parses W0611 unused import warning", () => {
      const line = "utils.py:5:0: W0611: Unused import os (unused-import)";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.severity).toBe("warning");
      expect(result.ruleId).toBe("unused-import");
    });

    it("parses E0001 syntax error", () => {
      const line =
        "bad_syntax.py:10:5: E0001: Parsing failed: 'invalid syntax' (syntax-error)";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.severity).toBe("error");
      expect(result.ruleId).toBe("syntax-error");
    });

    it("parses F0001 fatal error", () => {
      const line = "corrupted.py:1:0: F0001: Unable to parse file (fatal)";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.severity).toBe("error");
      expect(result.ruleId).toBe("fatal");
    });

    it("parses R0801 duplicate code refactor suggestion", () => {
      const line =
        "duplicate.py:50:0: R0801: Similar lines in 2 files (duplicate-code)";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.severity).toBe("warning");
      expect(result.ruleId).toBe("duplicate-code");
    });
  });

  describe("Full tracebacks (multi-line)", () => {
    it("parses simple ValueError traceback", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "/app/main.py", line 42, in run',
        "    result = process(data)",
        '  File "/app/processor.py", line 15, in process',
        "    raise ValueError('Invalid input')",
        "ValueError: Invalid input",
      ];

      expect(parser.canParse(lines[0]!, ctx)).toBeGreaterThan(0.9);
      parser.parse(lines[0]!, ctx);

      for (let i = 1; i < lines.length - 1; i++) {
        expect(parser.continueMultiLine(lines[i]!, ctx)).toBe(true);
      }
      expect(parser.continueMultiLine(lines.at(-1)!, ctx)).toBe(false);

      const result = parser.finishMultiLine(ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toBe("ValueError: Invalid input");
      expect(result.file).toBe("/app/processor.py");
      expect(result.line).toBe(15);
      expect(result.severity).toBe("error");
      expect(result.category).toBe("runtime");
      expect(result.source).toBe("python");
      expect(result.stackTrace).toContain("Traceback (most recent call last):");
      expect(result.stackTrace).toContain("ValueError: Invalid input");
    });

    it("parses KeyError traceback with string key", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "app.py", line 10, in get_config',
        "    return config['missing_key']",
        "KeyError: 'missing_key'",
      ];

      parser.parse(lines[0]!, ctx);
      for (let i = 1; i < lines.length - 1; i++) {
        parser.continueMultiLine(lines[i]!, ctx);
      }
      parser.continueMultiLine(lines.at(-1)!, ctx);
      const result = parser.finishMultiLine(ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toBe("KeyError: 'missing_key'");
      expect(result.file).toBe("app.py");
      expect(result.line).toBe(10);
    });

    it("parses AttributeError traceback", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "script.py", line 5, in main',
        "    obj.nonexistent_method()",
        "AttributeError: 'NoneType' object has no attribute 'nonexistent_method'",
      ];

      parser.parse(lines[0]!, ctx);
      for (let i = 1; i < lines.length - 1; i++) {
        parser.continueMultiLine(lines[i]!, ctx);
      }
      parser.continueMultiLine(lines.at(-1)!, ctx);
      const result = parser.finishMultiLine(ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toContain("AttributeError");
      expect(result.message).toContain("nonexistent_method");
    });

    it("parses deep traceback and extracts deepest frame", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "a.py", line 1, in outer',
        "    b()",
        '  File "b.py", line 2, in middle',
        "    c()",
        '  File "c.py", line 3, in inner',
        "    d()",
        '  File "d.py", line 4, in deepest',
        "    raise RuntimeError('deep')",
        "RuntimeError: deep",
      ];

      parser.parse(lines[0]!, ctx);
      for (let i = 1; i < lines.length - 1; i++) {
        parser.continueMultiLine(lines[i]!, ctx);
      }
      parser.continueMultiLine(lines.at(-1)!, ctx);
      const result = parser.finishMultiLine(ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.file).toBe("d.py");
      expect(result.line).toBe(4);
      expect(result.message).toBe("RuntimeError: deep");
    });

    it("parses chained exception traceback", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "handler.py", line 10, in handle',
        "    parse(data)",
        "ValueError: bad data",
        "",
        "During handling of the above exception, another exception occurred:",
        "",
        "Traceback (most recent call last):",
        '  File "main.py", line 5, in run',
        "    handle(input)",
        "RuntimeError: Handler failed",
      ];

      parser.parse(lines[0]!, ctx);
      for (let i = 1; i < lines.length - 1; i++) {
        parser.continueMultiLine(lines[i]!, ctx);
      }
      parser.continueMultiLine(lines.at(-1)!, ctx);
      const result = parser.finishMultiLine(ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.stackTrace).toContain(
        "During handling of the above exception"
      );
      expect(result.message).toBe("RuntimeError: Handler failed");
    });
  });

  describe("SyntaxError handling", () => {
    it("parses SyntaxError traceback with caret", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "broken.py", line 5',
        "    print('hello'",
        "         ^",
        "SyntaxError: unexpected EOF while parsing",
      ];

      parser.parse(lines[0]!, ctx);
      for (let i = 1; i < lines.length - 1; i++) {
        parser.continueMultiLine(lines[i]!, ctx);
      }
      parser.continueMultiLine(lines.at(-1)!, ctx);
      const result = parser.finishMultiLine(ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.file).toBe("broken.py");
      expect(result.line).toBe(5);
      expect(result.column).toBe(10);
      expect(result.message).toContain("SyntaxError");
    });

    it("parses SyntaxError traceback with nested frames", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "script.py", line 10, in parse_input',
        "    exec(code)",
        '  File "<string>", line 1',
        "    x =",
        "      ^",
        "SyntaxError: invalid syntax",
      ];

      parser.parse(lines[0]!, ctx);
      for (let i = 1; i < lines.length - 1; i++) {
        parser.continueMultiLine(lines[i]!, ctx);
      }
      parser.continueMultiLine(lines.at(-1)!, ctx);
      const result = parser.finishMultiLine(ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toContain("SyntaxError");
      expect(result.file).toBe("<string>");
      expect(result.line).toBe(1);
    });

    it("parses IndentationError traceback", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "indent.py", line 3',
        "    def foo():",
        "    ^",
        "IndentationError: expected an indented block",
      ];

      parser.parse(lines[0]!, ctx);
      for (let i = 1; i < lines.length - 1; i++) {
        parser.continueMultiLine(lines[i]!, ctx);
      }
      parser.continueMultiLine(lines.at(-1)!, ctx);
      const result = parser.finishMultiLine(ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.file).toBe("indent.py");
      expect(result.line).toBe(3);
      expect(result.message).toContain("IndentationError");
    });

    it("parses standalone SyntaxError", () => {
      const line = "SyntaxError: invalid syntax";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toBe("SyntaxError: invalid syntax");
      expect(result.category).toBe("compile");
      expect(result.lineKnown).toBe(false);
    });

    it("parses TabError", () => {
      const line =
        "TabError: inconsistent use of tabs and spaces in indentation";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toContain("TabError");
      expect(result.category).toBe("compile");
    });
  });

  describe("Standalone exceptions", () => {
    it("parses standalone RuntimeError", () => {
      const line = "RuntimeError: maximum recursion depth exceeded";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toBe(
        "RuntimeError: maximum recursion depth exceeded"
      );
      expect(result.severity).toBe("error");
      expect(result.category).toBe("runtime");
    });

    it("parses standalone MemoryError", () => {
      const line = "MemoryError: Unable to allocate array";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toContain("MemoryError");
    });

    it("parses standalone TimeoutError", () => {
      const line = "TimeoutError: Operation timed out after 30 seconds";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toContain("TimeoutError");
    });

    it("parses custom exception with Warning suffix", () => {
      const line = "DeprecationWarning: This function is deprecated";
      const result = parser.parse(line, ctx) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.message).toContain("DeprecationWarning");
    });
  });

  describe("Edge cases", () => {
    describe("Virtual environment paths", () => {
      it("parses error with venv path", () => {
        const lines = [
          "Traceback (most recent call last):",
          '  File "/home/user/project/.venv/lib/python3.11/site-packages/requests/api.py", line 59, in request',
          "    return session.request(method=method, url=url, **kwargs)",
          '  File "/home/user/project/app.py", line 20, in fetch',
          "    response.raise_for_status()",
          "requests.exceptions.HTTPError: 404 Client Error",
        ];

        parser.parse(lines[0]!, ctx);
        for (let i = 1; i < lines.length - 1; i++) {
          parser.continueMultiLine(lines[i]!, ctx);
        }
        parser.continueMultiLine(lines.at(-1)!, ctx);
        const result = parser.finishMultiLine(ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.file).toBe("/home/user/project/app.py");
        expect(result.line).toBe(20);
      });

      it("parses mypy error with venv-like path", () => {
        const line =
          ".venv/lib/python3.11/site-packages/pkg/module.py:10: error: Argument 1 has incompatible type [arg-type]";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.file).toContain("site-packages");
      });
    });

    describe("Site-packages paths (noise filtering)", () => {
      it("identifies frozen module file refs as noise", () => {
        expect(parser.isNoise("<frozen importlib._bootstrap>")).toBe(true);
        expect(parser.isNoise("<frozen importlib._bootstrap_external>")).toBe(
          true
        );
      });

      it("identifies internal file refs as noise", () => {
        expect(parser.isNoise('  File "<stdin>", line 1, in <module>')).toBe(
          true
        );
        expect(parser.isNoise('  File "<string>", line 1, in <module>')).toBe(
          true
        );
      });
    });

    describe("Unicode in messages", () => {
      it("parses error message with unicode characters", () => {
        const line =
          "ValueError: Invalid character: '\\u2019' (right single quotation mark)";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toContain("Invalid character");
      });

      it("parses file path with unicode", () => {
        const line = "tests/test_unicod\u00e9.py:10:1: E501 Line too long";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.file).toContain("unicod");
      });

      it("parses traceback with unicode in message", () => {
        const lines = [
          "Traceback (most recent call last):",
          '  File "app.py", line 5, in main',
          "    raise ValueError('Invalid: \u2603')",
          "ValueError: Invalid: \u2603",
        ];

        parser.parse(lines[0]!, ctx);
        for (let i = 1; i < lines.length - 1; i++) {
          parser.continueMultiLine(lines[i]!, ctx);
        }
        parser.continueMultiLine(lines.at(-1)!, ctx);
        const result = parser.finishMultiLine(ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toContain("\u2603");
      });
    });

    describe("Very long tracebacks (bounded)", () => {
      it("handles traceback with many frames (resource limits)", () => {
        const lines: string[] = ["Traceback (most recent call last):"];

        for (let i = 0; i < 150; i++) {
          lines.push(`  File "module_${i}.py", line ${i + 1}, in func_${i}`);
          lines.push(`    func_${i + 1}()`);
        }
        lines.push("RecursionError: maximum recursion depth exceeded");

        parser.parse(lines[0]!, ctx);
        for (let i = 1; i < lines.length; i++) {
          parser.continueMultiLine(lines[i]!, ctx);
        }
        const result = parser.finishMultiLine(ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.stackTraceTruncated).toBe(true);
        expect(result.file).toBe("module_99.py");
        expect(result.line).toBe(100);
      });

      it("truncates very long error messages", () => {
        const longMessage = "x".repeat(3000);
        const line = `ValueError: ${longMessage}`;
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message.length).toBeLessThanOrEqual(2012);
      });
    });

    describe("ANSI escape codes", () => {
      it("strips ANSI codes from error line", () => {
        const line = "\x1b[31mValueError: \x1b[0mInvalid input";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.message).toBe("ValueError: Invalid input");
      });

      it("strips ANSI codes from file path", () => {
        const line =
          "\x1b[1m\x1b[36mapp/main.py\x1b[0m:10:5: E501 Line too long";
        const result = parser.parse(line, ctx) as ExtractedError;

        expect(result).not.toBeNull();
        expect(result.file).toBe("app/main.py");
      });
    });
  });

  describe("Noise detection", () => {
    it("identifies pytest progress dots as noise", () => {
      expect(parser.isNoise("....")).toBe(true);
      expect(parser.isNoise("..........")).toBe(true);
    });

    it("identifies pytest summary lines as noise", () => {
      expect(parser.isNoise("5 passed in 0.42s")).toBe(true);
      expect(parser.isNoise("3 failed, 10 passed")).toBe(true);
      expect(parser.isNoise("2 errors")).toBe(true);
      expect(parser.isNoise("1 warning")).toBe(true);
      expect(parser.isNoise("10 skipped")).toBe(true);
    });

    it("identifies pytest headers as noise", () => {
      expect(parser.isNoise("=== test session starts ===")).toBe(true);
      expect(parser.isNoise("=== short test summary info ===")).toBe(true);
      expect(parser.isNoise("=== warnings summary ===")).toBe(true);
      expect(parser.isNoise("=== FAILURES ===")).toBe(true);
      expect(parser.isNoise("=== ERRORS ===")).toBe(true);
    });

    it("identifies pytest platform info as noise", () => {
      expect(
        parser.isNoise(
          "platform linux -- Python 3.11.0, pytest-7.2.0, pluggy-1.0.0"
        )
      ).toBe(true);
      expect(
        parser.isNoise(
          "platform darwin -- Python 3.10.5, pytest-7.1.2, pluggy-0.13.1"
        )
      ).toBe(true);
      expect(
        parser.isNoise("platform win32 -- Python 3.9.7, pytest-6.2.5")
      ).toBe(true);
    });

    it("identifies pytest verbose passed/skipped as noise", () => {
      expect(parser.isNoise("tests/test_foo.py::test_bar PASSED")).toBe(true);
      expect(parser.isNoise("tests/test_foo.py::test_baz SKIPPED")).toBe(true);
    });

    it("identifies pytest assertion details as noise", () => {
      expect(parser.isNoise("E       assert 1 == 2")).toBe(true);
      expect(parser.isNoise("E       +  where 1 = func()")).toBe(true);
      expect(parser.isNoise("E       -  and 2 = other()")).toBe(true);
    });

    it("identifies mypy/pylint summary as noise", () => {
      expect(parser.isNoise("Success: no issues found in 5 source files")).toBe(
        true
      );
      expect(parser.isNoise("Found 3 errors in 2 files")).toBe(true);
      expect(
        parser.isNoise(
          "Your code has been rated at 9.50/10 (previous run: 9.00/10)"
        )
      ).toBe(true);
    });

    it("identifies ruff/flake8 summary as noise", () => {
      expect(parser.isNoise("All checks passed!")).toBe(true);
      expect(parser.isNoise("10 files checked")).toBe(true);
      expect(parser.isNoise("5 files scanned")).toBe(true);
    });

    it("identifies coverage output as noise", () => {
      expect(parser.isNoise("Coverage report:")).toBe(true);
      expect(
        parser.isNoise("Name                      Stmts   Miss  Cover")
      ).toBe(true);
      expect(
        parser.isNoise("TOTAL                       500     50    90%")
      ).toBe(true);
    });

    it("identifies empty/whitespace lines as noise", () => {
      expect(parser.isNoise("")).toBe(true);
      expect(parser.isNoise("   ")).toBe(true);
      expect(parser.isNoise("\t")).toBe(true);
    });

    it("does not identify actual errors as noise", () => {
      expect(parser.isNoise("ValueError: Invalid input")).toBe(false);
      expect(
        parser.isNoise("FAILED tests/test_foo.py::test_bar - AssertionError")
      ).toBe(false);
      expect(parser.isNoise("app.py:10: error: Type mismatch")).toBe(false);
    });
  });

  describe("canParse confidence scores", () => {
    it("returns high confidence for traceback start", () => {
      expect(parser.canParse("Traceback (most recent call last):", ctx)).toBe(
        0.95
      );
    });

    it("returns high confidence for pytest FAILED", () => {
      expect(
        parser.canParse(
          "FAILED tests/test.py::test_foo - AssertionError: msg",
          ctx
        )
      ).toBe(0.95);
    });

    it("returns high confidence for exception lines", () => {
      expect(parser.canParse("ValueError: Invalid input", ctx)).toBe(0.95);
    });

    it("returns good confidence for mypy errors", () => {
      expect(
        parser.canParse("app.py:10: error: Type mismatch [type-arg]", ctx)
      ).toBe(0.93);
    });

    it("returns good confidence for ruff/flake8 with column", () => {
      expect(parser.canParse("app.py:10:5: E501 Line too long", ctx)).toBe(
        0.93
      );
    });

    it("returns zero for non-matching lines", () => {
      expect(parser.canParse("Just some regular text", ctx)).toBe(0);
      expect(parser.canParse("", ctx)).toBe(0);
      expect(parser.canParse("npm install", ctx)).toBe(0);
    });

    it("returns higher confidence when in traceback state", () => {
      parser.parse("Traceback (most recent call last):", ctx);
      expect(parser.canParse('  File "app.py", line 10, in main', ctx)).toBe(
        0.9
      );
    });
  });

  describe("Parser reset", () => {
    it("resets state between parses", () => {
      const lines = [
        "Traceback (most recent call last):",
        '  File "app.py", line 10, in main',
        "    raise ValueError('test')",
      ];

      parser.parse(lines[0]!, ctx);
      parser.continueMultiLine(lines[1]!, ctx);

      parser.reset();

      expect(parser.canParse("Traceback (most recent call last):", ctx)).toBe(
        0.95
      );
      expect(parser.canParse("some other line", ctx)).toBe(0);
    });
  });

  describe("noisePatterns method", () => {
    it("returns noise patterns for registry optimization", () => {
      const patterns = parser.noisePatterns();

      expect(patterns.fastPrefixes.length).toBeGreaterThan(0);
      expect(patterns.fastContains.length).toBeGreaterThan(0);
      expect(patterns.regex.length).toBeGreaterThan(0);
    });
  });

  describe("Workflow context", () => {
    it("applies workflow context to parsed errors", () => {
      const ctxWithWorkflow = createParseContext({
        job: "test-job",
        step: "Run pytest",
      });

      const line = "ValueError: Invalid input";
      const result = parser.parse(line, ctxWithWorkflow) as ExtractedError;

      expect(result).not.toBeNull();
      expect(result.workflowContext?.job).toBe("test-job");
      expect(result.workflowContext?.step).toBe("Run pytest");
    });
  });
});
