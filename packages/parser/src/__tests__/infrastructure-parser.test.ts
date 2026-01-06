import { beforeEach, describe, expect, it } from "vitest";
import type { ParseContext } from "../parser-types.js";
import { createInfrastructureParser } from "../parsers/infrastructure.js";

/**
 * Tests for the InfrastructureParser directly.
 * These tests cover infrastructure-specific error patterns.
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

describe("InfrastructureParser", () => {
  let parser: ReturnType<typeof createInfrastructureParser>;

  beforeEach(() => {
    parser = createInfrastructureParser();
  });

  describe("Parser Properties", () => {
    it("has correct parser id", () => {
      expect(parser.id).toBe("infrastructure");
    });

    it("has correct priority (lower than language parsers, higher than generic)", () => {
      expect(parser.priority).toBe(70);
    });
  });

  describe("Node.js Version Errors", () => {
    it("parses Node.js version requirement error", () => {
      const ctx = createContext();
      const line =
        "error You are running Node 14.x but this package requires Node >= 18";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Node.js version mismatch");
      expect(result?.message).toContain("14.x");
      expect(result?.message).toContain(">= 18");
      expect(result?.ruleId).toBe("node-version");
      expect(result?.category).toBe("infrastructure");
    });

    it("parses engine incompatible error", () => {
      const ctx = createContext();
      const line = 'The engine "node" is incompatible with this module';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("engine incompatible");
      expect(result?.ruleId).toBe("node-engine-incompatible");
    });

    it("parses engine incompatible error with package prefix", () => {
      const ctx = createContext();
      const line =
        'error @package@1.0.0: The engine "node" is incompatible with this module';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.ruleId).toBe("node-engine-incompatible");
    });
  });

  describe("Missing Scripts/Dependencies Errors", () => {
    it('parses Yarn/Bun missing script: error Missing script: "build"', () => {
      const ctx = createContext();
      const line = 'error Missing script: "build"';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Missing script: "build"');
      expect(result?.ruleId).toBe("missing-script");
      expect(result?.suggestions).toContain(
        'Add "build" script to package.json'
      );
    });

    it("parses npm missing script: npm ERR! missing script: test", () => {
      const ctx = createContext();
      const line = "npm ERR! missing script: test";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Missing script: "test"');
      expect(result?.ruleId).toBe("missing-script");
    });

    it("parses generic missing script: error: no such script: lint", () => {
      const ctx = createContext();
      const line = "error: no such script: lint";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain('Missing script: "lint"');
      expect(result?.ruleId).toBe("missing-script");
    });

    it("parses sh command not found: sh: 1: eslint: not found", () => {
      const ctx = createContext();
      const line = "sh: 1: eslint: not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Command not found: eslint");
      expect(result?.ruleId).toBe("exit-127");
    });
  });

  describe("Network/Registry Errors", () => {
    it("parses npm ETIMEDOUT error", () => {
      const ctx = createContext();
      const line = "npm ERR! code ETIMEDOUT";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Network error: ETIMEDOUT");
      expect(result?.ruleId).toBe("npm-ETIMEDOUT");
      expect(result?.category).toBe("infrastructure");
    });

    it("parses npm ECONNREFUSED error", () => {
      const ctx = createContext();
      const line = "npm ERR! code ECONNREFUSED";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Network error: ECONNREFUSED");
      expect(result?.ruleId).toBe("npm-ECONNREFUSED");
    });

    it("parses registry request failed error", () => {
      const ctx = createContext();
      const line =
        "error: request to https://registry.npmjs.org/package failed";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Registry request failed");
      expect(result?.ruleId).toBe("registry-request-failed");
    });

    it("parses DNS resolution error", () => {
      const ctx = createContext();
      const line = "Error: getaddrinfo ENOTFOUND registry.npmjs.org";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("DNS resolution failed");
      expect(result?.message).toContain("registry.npmjs.org");
      expect(result?.ruleId).toBe("dns-enotfound");
    });
  });

  describe("Disk/Resource Errors", () => {
    it("parses ENOSPC error", () => {
      const ctx = createContext();
      const line = "ENOSPC: no space left on device";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("No space left on device");
      expect(result?.ruleId).toBe("enospc");
      expect(result?.suggestions).toContain("Free up disk space");
    });

    it("parses ENOMEM error", () => {
      const ctx = createContext();
      const line = "ENOMEM Cannot allocate memory";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Cannot allocate memory");
      expect(result?.ruleId).toBe("enomem");
    });

    it("parses npm ENOENT error", () => {
      const ctx = createContext();
      const line = "npm ERR! code ENOENT";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("File or directory not found");
      expect(result?.ruleId).toBe("npm-ENOENT");
    });
  });

  describe("Authentication Errors", () => {
    it("parses npm 401 Unauthorized", () => {
      const ctx = createContext();
      const line = "npm ERR! 401 Unauthorized";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("401 Unauthorized");
      expect(result?.ruleId).toBe("npm-401");
    });

    it("parses npm 403 Forbidden", () => {
      const ctx = createContext();
      const line = "npm ERR! 403 Forbidden";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("403 Forbidden");
      expect(result?.ruleId).toBe("npm-403");
    });

    it("parses generic authentication required", () => {
      const ctx = createContext();
      const line = "error: Authentication required";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Authentication required");
      expect(result?.ruleId).toBe("auth-required");
    });
  });

  // NOTE: CI-runner-specific patterns (like "Process completed with exit code")
  // are NOT handled by the infrastructure parser. Those are GitHub Actions-specific
  // and should be handled by CI context parsers, not tool parsers.

  describe("Tool-Level Infrastructure Patterns", () => {
    it("parses bash command not found", () => {
      const ctx = createContext();
      const line = "bash: node: command not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Command not found");
    });

    it("parses git fatal error", () => {
      const ctx = createContext();
      const line = "fatal: repository not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Git fatal error");
    });

    it("parses docker daemon error", () => {
      const ctx = createContext();
      const line = "docker: Error response from daemon: image not found";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("Docker daemon error");
    });

    it("parses npm ELIFECYCLE error", () => {
      const ctx = createContext();
      const line = "npm ERR! code ELIFECYCLE";

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).toContain("npm error: ELIFECYCLE");
    });
  });

  describe("Edge Cases", () => {
    it("handles ANSI escape codes", () => {
      const ctx = createContext();
      const line = '\x1b[31merror Missing script: "build"\x1b[0m';

      expect(parser.canParse(line, ctx)).toBeGreaterThan(0);

      const result = parser.parse(line, ctx);
      expect(result).not.toBeNull();
      expect(result?.message).not.toContain("\x1b");
    });

    it("rejects very long lines", () => {
      const ctx = createContext();
      const longLine = `error: ${"a".repeat(3000)}`;
      expect(parser.canParse(longLine, ctx)).toBe(0);
    });

    it("provides noise patterns", () => {
      const patterns = parser.noisePatterns();
      expect(patterns.fastPrefixes).toBeDefined();
      expect(patterns.fastContains).toBeDefined();
      expect(patterns.regex).toBeDefined();
    });
  });
});
