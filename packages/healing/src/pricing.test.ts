import { describe, expect, test } from "vitest";
import { calculateCost } from "./pricing.js";
import type { TokenUsage } from "./types.js";

const usage = (overrides: Partial<TokenUsage> = {}): TokenUsage => ({
  inputTokens: 0,
  outputTokens: 0,
  cacheReadInputTokens: 0,
  cacheCreationInputTokens: 0,
  ...overrides,
});

describe("calculateCost", () => {
  test("standard pricing for sonnet model", () => {
    const cost = calculateCost(
      "claude-sonnet-4-5-20250929",
      usage({ inputTokens: 1_000_000, outputTokens: 1_000_000 })
    );
    expect(cost).toBe(18.0);
  });

  test("cache read discount (0.1x)", () => {
    const cost = calculateCost(
      "claude-sonnet-4-5",
      usage({ cacheReadInputTokens: 1_000_000 })
    );
    expect(cost).toBeCloseTo(0.3);
  });

  test("cache write premium (1.25x)", () => {
    const cost = calculateCost(
      "claude-sonnet-4-5",
      usage({ cacheCreationInputTokens: 1_000_000 })
    );
    expect(cost).toBeCloseTo(3.75);
  });

  test("opus model pricing", () => {
    const cost = calculateCost(
      "claude-opus-4-5",
      usage({ inputTokens: 1_000_000 })
    );
    expect(cost).toBe(5.0);
  });

  test("unknown model uses default sonnet pricing", () => {
    const cost = calculateCost(
      "unknown-model-xyz",
      usage({ inputTokens: 1_000_000 })
    );
    expect(cost).toBe(3.0);
  });

  test("combined usage with all token types", () => {
    // haiku: $0.80 input, $4.00 output per million
    // 500K input = $0.40, 500K output = $2.00
    // 1M cache read = $0.80 * 0.1 = $0.08
    // 500K cache write = $0.40 * 1.25 = $0.50
    // Total = $0.40 + $2.00 + $0.08 + $0.50 = $2.98
    const cost = calculateCost(
      "claude-3-5-haiku",
      usage({
        inputTokens: 500_000,
        outputTokens: 500_000,
        cacheReadInputTokens: 1_000_000,
        cacheCreationInputTokens: 500_000,
      })
    );
    expect(cost).toBeCloseTo(2.98);
  });
});
