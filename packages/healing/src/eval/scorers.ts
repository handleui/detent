import type { HealingTestCase } from "./types.js";

/**
 * Score whether the healing succeeded as expected.
 */
export const successScore = ({
  output,
  expected,
}: {
  output: { success: boolean };
  expected: HealingTestCase["expected"];
}): number => {
  if (expected.shouldSucceed) {
    return output.success ? 1 : 0;
  }
  // If we expected failure, success is wrong
  return output.success ? 0 : 1;
};

/**
 * Score iteration efficiency (fewer iterations = better).
 * Returns 1.0 for 1 iteration, decreasing as iterations increase.
 */
export const iterationEfficiencyScore = ({
  output,
  expected,
}: {
  output: { iterations: number };
  expected: HealingTestCase["expected"];
}): number => {
  const maxExpected = expected.maxIterations ?? 10;
  if (output.iterations <= 1) {
    return 1;
  }
  if (output.iterations >= maxExpected) {
    return 0;
  }
  return 1 - (output.iterations - 1) / (maxExpected - 1);
};

/**
 * Score cost efficiency (lower cost = better).
 * Returns 1.0 for $0, decreasing as cost approaches max.
 */
export const costEfficiencyScore = ({
  output,
  expected,
}: {
  output: { costUSD: number };
  expected: HealingTestCase["expected"];
}): number => {
  const maxExpected = expected.maxCostUSD ?? 1.0;
  if (output.costUSD <= 0) {
    return 1;
  }
  if (output.costUSD >= maxExpected) {
    return 0;
  }
  return 1 - output.costUSD / maxExpected;
};

/**
 * Score whether expected keywords appear in the final message.
 */
export const keywordPresenceScore = ({
  output,
  expected,
}: {
  output: { finalMessage: string };
  expected: HealingTestCase["expected"];
}): number => {
  const keywords = expected.expectedKeywords;
  if (!keywords || keywords.length === 0) {
    return 1;
  }

  const message = output.finalMessage.toLowerCase();
  const found = keywords.filter((kw) => message.includes(kw.toLowerCase()));
  return found.length / keywords.length;
};

/**
 * Combined quality score (weighted average of all metrics).
 */
export const overallQualityScore = ({
  output,
  expected,
}: {
  output: {
    success: boolean;
    iterations: number;
    costUSD: number;
    finalMessage: string;
  };
  expected: HealingTestCase["expected"];
}): number => {
  const success = successScore({ output, expected });
  const efficiency = iterationEfficiencyScore({ output, expected });
  const cost = costEfficiencyScore({ output, expected });
  const keywords = keywordPresenceScore({ output, expected });

  // Weights: success is most important, then efficiency
  return success * 0.5 + efficiency * 0.25 + cost * 0.15 + keywords * 0.1;
};
