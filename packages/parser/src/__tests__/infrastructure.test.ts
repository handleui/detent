import { beforeEach, describe, expect, it } from "vitest";
import type { ParseContext } from "../parser-types.js";
import { createInfrastructureParser } from "../parsers/infrastructure.js";

/**
 * Tests for the infrastructure error parser.
 * Infrastructure errors are CI/CD configuration issues, not code problems.
 *
 * Supported patterns:
 * - Package manager script failures: `error: script "X" exited with code N`
 * - Command/tool failures: `"X" exited with code N`
 * - Context cancellation: `Error: context canceled`
 * - Shell command not found: `bash: X: command not found`
 * - Permission denied: `./script.sh: Permission denied`
 * - npm errors: `npm ERR! code ELIFECYCLE`
 * - Docker errors: `docker: Error response from daemon: ...`
 * - Git fatal errors: `fatal: ...`
 */

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

describe("Infrastructure Error Parser", () => {
  let parser: ReturnType<typeof createInfrastructureParser>;

  beforeEach(() => {
    parser = createInfrastructureParser();
  });

  describe("Package Manager Script Failures", () => {
    it('parses: error: script "lint" exited with code 1', () => {
      const ctx = createContext();
      const line = 'error: script "lint" exited with code 1';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Script "lint" failed');
      expect(result?.message).toContain("exit code 1");
      expect(result?.category).toBe("infrastructure");
      expect(result?.ruleId).toBe("exit-1");
    });

    it('parses: error: script "lint:go" exited with code 127', () => {
      const ctx = createContext();
      const line = 'error: script "lint:go" exited with code 127';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Script "lint:go" failed');
      expect(result?.message).toContain("exit code 127");
      expect(result?.ruleId).toBe("exit-127");
    });

    it('parses: error: script "build" exited with code 2', () => {
      const ctx = createContext();
      const line = 'error: script "build" exited with code 2';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Script "build" failed');
    });
  });

  describe("Command Exit Code Failures (Turbo style)", () => {
    it('parses: "turbo" exited with code 1', () => {
      const ctx = createContext();
      const line = '"turbo" exited with code 1';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Command "turbo" failed');
      expect(result?.message).toContain("exit code 1");
      expect(result?.category).toBe("infrastructure");
    });

    it('parses: "eslint" exited with code 2', () => {
      const ctx = createContext();
      const line = '"eslint" exited with code 2';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Command "eslint" failed');
    });

    it('parses: "tsc" exited with code 1', () => {
      const ctx = createContext();
      const line = '"tsc" exited with code 1';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Command "tsc" failed');
    });
  });

  describe("Context Cancellation Errors", () => {
    it("parses: Error: context canceled", () => {
      const ctx = createContext();
      const line = "Error: context canceled";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("context canceled");
      expect(result?.category).toBe("metadata");
    });

    it("handles case-insensitive matching: error: context canceled", () => {
      const ctx = createContext();
      const line = "error: context canceled";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
    });
  });

  describe("Shell Command Not Found", () => {
    it("parses: bash: npm: command not found", () => {
      const ctx = createContext();
      const line = "bash: npm: command not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Command not found");
      expect(result?.message).toContain("npm");
      expect(result?.category).toBe("infrastructure");
      expect(result?.ruleId).toBe("exit-127");
    });

    it("parses: sh: golangci-lint: command not found", () => {
      const ctx = createContext();
      const line = "sh: golangci-lint: command not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Command not found");
      expect(result?.message).toContain("golangci-lint");
    });

    it("parses: zsh: node: command not found", () => {
      const ctx = createContext();
      const line = "zsh: node: command not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Command not found");
      expect(result?.message).toContain("node");
    });
  });

  describe("Permission Denied Errors", () => {
    it("parses: ./script.sh: Permission denied", () => {
      const ctx = createContext();
      const line = "./script.sh: Permission denied";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Permission denied");
      expect(result?.message).toContain("./script.sh");
      expect(result?.file).toBe("./script.sh");
      expect(result?.category).toBe("infrastructure");
      expect(result?.ruleId).toBe("exit-126");
    });

    it("parses: /usr/bin/test: Permission denied", () => {
      const ctx = createContext();
      const line = "/usr/bin/test: Permission denied";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Permission denied");
      expect(result?.file).toBe("/usr/bin/test");
    });
  });

  describe("npm Errors", () => {
    it("parses: npm ERR! code ELIFECYCLE", () => {
      const ctx = createContext();
      const line = "npm ERR! code ELIFECYCLE";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("npm error");
      expect(result?.message).toContain("ELIFECYCLE");
      expect(result?.category).toBe("infrastructure");
      expect(result?.ruleId).toBe("npm-ELIFECYCLE");
      expect(result?.suggestions).toContain(
        "Check the script output above for the actual error"
      );
    });

    it("parses: npm ERR! code ENOENT", () => {
      const ctx = createContext();
      const line = "npm ERR! code ENOENT";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      // The infrastructure parser provides a more descriptive message for ENOENT
      expect(result?.message).toContain("ENOENT");
      expect(result?.ruleId).toBe("npm-ENOENT");
    });

    it("parses: npm ERR! code E404", () => {
      const ctx = createContext();
      const line = "npm ERR! code E404";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("npm-E404");
    });
  });

  describe("Docker Errors", () => {
    it("parses: docker: Error response from daemon: pull access denied", () => {
      const ctx = createContext();
      const line =
        "docker: Error response from daemon: pull access denied for private/image";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Docker daemon error");
      expect(result?.message).toContain("pull access denied");
      expect(result?.category).toBe("infrastructure");
      expect(result?.ruleId).toBe("docker-daemon");
    });

    it("parses: docker: Error response from daemon: conflict", () => {
      const ctx = createContext();
      const line =
        "docker: Error response from daemon: conflict: unable to remove repository";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Docker daemon error");
    });

    it("parses: docker: Error response from daemon: image not found", () => {
      const ctx = createContext();
      const line = "docker: Error response from daemon: image not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
    });
  });

  describe("Git Fatal Errors", () => {
    it("parses: fatal: repository not found", () => {
      const ctx = createContext();
      const line =
        "fatal: repository 'https://github.com/org/repo.git/' not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Git fatal error");
      expect(result?.message).toContain("repository");
      expect(result?.category).toBe("infrastructure");
      expect(result?.ruleId).toBe("git-fatal");
    });

    it("parses: fatal: Authentication failed", () => {
      const ctx = createContext();
      const line =
        "fatal: Authentication failed for 'https://github.com/org/repo.git/'";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Git fatal error");
      expect(result?.message).toContain("Authentication failed");
    });

    it("parses: fatal: Could not read from remote repository", () => {
      const ctx = createContext();
      const line = "fatal: Could not read from remote repository.";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Git fatal error");
    });

    it("parses: fatal: not a git repository", () => {
      const ctx = createContext();
      const line =
        "fatal: not a git repository (or any of the parent directories): .git";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
    });
  });

  describe("Noise Detection", () => {
    it("filters npm informational messages as noise", () => {
      const noiseLines = [
        "npm warn deprecated package@1.0.0",
        "npm notice Package update available",
        "npm info lifecycle package@1.0.0~build",
      ];

      for (const line of noiseLines) {
        expect(parser.isNoise(line)).toBe(true);
      }
    });

    it("filters Docker build progress as noise", () => {
      const noiseLines = [
        "Step 1/10 : FROM node:18",
        "Layer abc123: Pulling",
        "#5 [internal] load build definition",
      ];

      for (const line of noiseLines) {
        expect(parser.isNoise(line)).toBe(true);
      }
    });

    it("filters Git progress as noise", () => {
      const noiseLines = [
        "remote: Counting objects: 100% (10/10)",
        "Receiving objects: 50% (5/10)",
        "Resolving deltas: 100% (3/3)",
        "Unpacking objects: 100% (10/10)",
      ];

      for (const line of noiseLines) {
        expect(parser.isNoise(line)).toBe(true);
      }
    });

    it("filters package manager progress as noise", () => {
      const noiseLines = [
        "Downloading package@1.0.0",
        "Installing dependencies...",
        "Resolving packages...",
      ];

      for (const line of noiseLines) {
        expect(parser.isNoise(line)).toBe(true);
      }
    });

    it("filters success indicators as noise", () => {
      const noiseLines = [
        "Build completed successfully",
        "Installation completed",
        "50% done",
        "Progress: 75%",
      ];

      for (const line of noiseLines) {
        expect(parser.isNoise(line)).toBe(true);
      }
    });
  });

  describe("Parser Properties", () => {
    it("has correct parser id", () => {
      expect(parser.id).toBe("infrastructure");
    });

    it("has appropriate priority (higher than generic, lower than language parsers)", () => {
      expect(parser.priority).toBe(70);
    });

    it("does not support multi-line parsing", () => {
      expect(parser.supportsMultiLine()).toBe(false);
    });

    it("provides noise patterns for registry optimization", () => {
      const patterns = parser.noisePatterns();
      expect(patterns.fastPrefixes).toBeDefined();
      expect(patterns.fastContains).toBeDefined();
      expect(patterns.regex).toBeDefined();
      expect(patterns.fastPrefixes.length).toBeGreaterThan(0);
    });
  });

  describe("Edge Cases", () => {
    it("rejects very long lines", () => {
      const ctx = createContext();
      const longLine = `error: script "${"a".repeat(2500)}" exited with code 1`;
      expect(parser.canParse(longLine, ctx)).toBe(0);
    });

    it("handles ANSI escape codes", () => {
      const ctx = createContext();
      const line = '\x1b[31merror: script "test" exited with code 1\x1b[0m';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).not.toContain("\x1b");
    });

    it("handles leading/trailing whitespace", () => {
      const ctx = createContext();
      const line = '   error: script "test" exited with code 1   ';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
    });

    it("applies workflow context when provided", () => {
      const ctx = createContext({
        job: "build",
        step: "lint",
        workflowContext: {
          job: "build",
          step: "lint",
        },
      });
      const line = 'error: script "lint" exited with code 1';

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.workflowContext?.job).toBe("build");
      expect(result?.workflowContext?.step).toBe("lint");
    });

    it("does not match lines without recognized patterns", () => {
      const ctx = createContext();
      const nonMatchingLines = [
        "Some random log line",
        "info: Starting build process",
        "warning: This is a warning",
        "npm run build",
      ];

      for (const line of nonMatchingLines) {
        expect(parser.canParse(line, ctx)).toBe(0);
      }
    });
  });

  describe("Confidence Scores", () => {
    it("returns highest confidence for context canceled", () => {
      const ctx = createContext();
      expect(parser.canParse("Error: context canceled", ctx)).toBe(0.95);
    });

    it("returns high confidence for docker errors", () => {
      const ctx = createContext();
      const confidence = parser.canParse(
        "docker: Error response from daemon: not found",
        ctx
      );
      expect(confidence).toBe(0.93);
    });

    it("returns high confidence for shell not found", () => {
      const ctx = createContext();
      const confidence = parser.canParse("bash: node: command not found", ctx);
      expect(confidence).toBe(0.92);
    });

    it("returns high confidence for permission denied", () => {
      const ctx = createContext();
      const confidence = parser.canParse("./script.sh: Permission denied", ctx);
      expect(confidence).toBe(0.91);
    });

    it("returns high confidence for package manager script failures", () => {
      const ctx = createContext();
      const confidence = parser.canParse(
        'error: script "test" exited with code 1',
        ctx
      );
      expect(confidence).toBe(0.9);
    });

    it("returns high confidence for git fatal errors", () => {
      const ctx = createContext();
      const confidence = parser.canParse("fatal: repository not found", ctx);
      expect(confidence).toBe(0.9);
    });
  });
});
