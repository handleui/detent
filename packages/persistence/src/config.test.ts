import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import {
  loadRepoConfigSafe,
  validateApiKey,
  validateBudgetMonthly,
  validateBudgetPerRun,
  validateModel,
  validateTimeout,
} from "./config.js";

// ============================================================================
// Test Fixtures
// ============================================================================

let testDir: string;

beforeEach(() => {
  testDir = join(
    tmpdir(),
    `detent-test-${Date.now()}-${Math.random().toString(36).slice(2)}`
  );
  mkdirSync(testDir, { recursive: true });
});

afterEach(() => {
  rmSync(testDir, { recursive: true, force: true });
});

// ============================================================================
// loadRepoConfigSafe Tests
// ============================================================================

describe("loadRepoConfigSafe", () => {
  test("returns empty config for missing file", () => {
    const result = loadRepoConfigSafe(testDir);
    expect(result.config).toEqual({});
    expect(result.error).toBeUndefined();
  });

  test("returns empty config for empty file", () => {
    const detentDir = join(testDir, ".detent");
    mkdirSync(detentDir, { recursive: true });
    writeFileSync(join(detentDir, "config.json"), "");

    const result = loadRepoConfigSafe(testDir);
    expect(result.config).toEqual({});
    expect(result.error).toBeUndefined();
  });

  test("returns empty config for whitespace-only file", () => {
    const detentDir = join(testDir, ".detent");
    mkdirSync(detentDir, { recursive: true });
    writeFileSync(join(detentDir, "config.json"), "   \n  ");

    const result = loadRepoConfigSafe(testDir);
    expect(result.config).toEqual({});
    expect(result.error).toBeUndefined();
  });

  test("loads valid config", () => {
    const detentDir = join(testDir, ".detent");
    mkdirSync(detentDir, { recursive: true });
    writeFileSync(
      join(detentDir, "config.json"),
      JSON.stringify({
        apiKey: "sk-ant-test-key-12345678901234567890",
        model: "claude-sonnet-4-5",
        budgetPerRunUsd: 5,
      })
    );

    const result = loadRepoConfigSafe(testDir);
    expect(result.config.apiKey).toBe("sk-ant-test-key-12345678901234567890");
    expect(result.config.model).toBe("claude-sonnet-4-5");
    expect(result.config.budgetPerRunUsd).toBe(5);
    expect(result.error).toBeUndefined();
  });

  test("returns error for corrupted JSON", () => {
    const detentDir = join(testDir, ".detent");
    mkdirSync(detentDir, { recursive: true });
    writeFileSync(join(detentDir, "config.json"), "{ invalid json }");

    const result = loadRepoConfigSafe(testDir);
    expect(result.config).toEqual({});
    expect(result.error).toContain("corrupted");
    expect(result.error).toContain("invalid JSON");
  });

  test("returns error when config path is a directory", () => {
    const detentDir = join(testDir, ".detent");
    const configPath = join(detentDir, "config.json");
    mkdirSync(configPath, { recursive: true }); // Create as directory instead of file

    const result = loadRepoConfigSafe(testDir);
    expect(result.config).toEqual({});
    expect(result.error).toContain("directory");
  });
});

// ============================================================================
// validateApiKey Tests
// ============================================================================

describe("validateApiKey", () => {
  test("accepts valid API key", () => {
    const result = validateApiKey(
      "sk-ant-api03-valid-key-with-enough-length-12345"
    );
    expect(result.valid).toBe(true);
    expect(result.error).toBeUndefined();
  });

  test("rejects empty API key", () => {
    const result = validateApiKey("");
    expect(result.valid).toBe(false);
    expect(result.error).toContain("required");
  });

  test("rejects API key that's too short", () => {
    const result = validateApiKey("sk-ant-short");
    expect(result.valid).toBe(false);
    expect(result.error).toContain("too short");
  });

  test("rejects API key with wrong prefix", () => {
    const result = validateApiKey("wrong-prefix-12345678901234567890");
    expect(result.valid).toBe(false);
    expect(result.error).toContain("sk-ant-");
  });

  test("rejects API key that's too long", () => {
    const result = validateApiKey(`sk-ant-${"a".repeat(200)}`);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("too long");
  });
});

// ============================================================================
// validateModel Tests
// ============================================================================

describe("validateModel", () => {
  test("accepts allowed models", () => {
    expect(validateModel("claude-sonnet-4-5").valid).toBe(true);
    expect(validateModel("claude-opus-4-5").valid).toBe(true);
    expect(validateModel("claude-haiku-4-5").valid).toBe(true);
  });

  test("accepts custom claude- prefixed models", () => {
    expect(validateModel("claude-custom-model").valid).toBe(true);
  });

  test("rejects empty model", () => {
    const result = validateModel("");
    expect(result.valid).toBe(false);
    expect(result.error).toContain("required");
  });

  test("rejects non-claude model", () => {
    const result = validateModel("gpt-4");
    expect(result.valid).toBe(false);
    expect(result.error).toContain("claude-");
  });
});

// ============================================================================
// validateBudgetPerRun Tests
// ============================================================================

describe("validateBudgetPerRun", () => {
  test("accepts valid budget", () => {
    expect(validateBudgetPerRun(0).valid).toBe(true);
    expect(validateBudgetPerRun(1).valid).toBe(true);
    expect(validateBudgetPerRun(100).valid).toBe(true);
  });

  test("rejects negative budget", () => {
    const result = validateBudgetPerRun(-1);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("negative");
  });

  test("rejects budget exceeding max", () => {
    const result = validateBudgetPerRun(101);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("100");
  });

  test("rejects NaN", () => {
    const result = validateBudgetPerRun(Number.NaN);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("number");
  });
});

// ============================================================================
// validateBudgetMonthly Tests
// ============================================================================

describe("validateBudgetMonthly", () => {
  test("accepts zero (unlimited)", () => {
    expect(validateBudgetMonthly(0).valid).toBe(true);
  });

  test("accepts valid monthly budget", () => {
    expect(validateBudgetMonthly(100).valid).toBe(true);
    expect(validateBudgetMonthly(1000).valid).toBe(true);
  });

  test("rejects negative budget", () => {
    const result = validateBudgetMonthly(-1);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("negative");
  });

  test("rejects budget exceeding max", () => {
    const result = validateBudgetMonthly(1001);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("1000");
  });
});

// ============================================================================
// validateTimeout Tests
// ============================================================================

describe("validateTimeout", () => {
  test("accepts valid timeout", () => {
    expect(validateTimeout(1).valid).toBe(true);
    expect(validateTimeout(10).valid).toBe(true);
    expect(validateTimeout(60).valid).toBe(true);
  });

  test("accepts zero (disabled)", () => {
    expect(validateTimeout(0).valid).toBe(true);
  });

  test("rejects timeout below minimum (when > 0)", () => {
    const result = validateTimeout(0.5);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("at least 1");
  });

  test("rejects negative timeout", () => {
    const result = validateTimeout(-1);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("negative");
  });

  test("rejects timeout exceeding max", () => {
    const result = validateTimeout(61);
    expect(result.valid).toBe(false);
    expect(result.error).toContain("60");
  });
});
