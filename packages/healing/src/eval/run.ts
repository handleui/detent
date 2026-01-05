/**
 * Braintrust evaluation runner for the healing loop.
 *
 * Run with: bun run eval
 *
 * Environment variables:
 * - BRAINTRUST_API_KEY: Required for logging to Braintrust
 * - ANTHROPIC_API_KEY: Required for running actual healing loops
 * - EVAL_MODE: "mock" (default) or "live" - controls whether to use real API calls
 */

if (!process.env.BRAINTRUST_API_KEY) {
  console.error("Error: BRAINTRUST_API_KEY environment variable is required.");
  console.error(
    "Get your API key from: https://www.braintrust.dev/app/settings"
  );
  console.error("\nRun with:");
  console.error("  export BRAINTRUST_API_KEY='your-key-here'");
  console.error("  bun run eval");
  process.exit(1);
}

import { Eval } from "braintrust";
import { HEALING_DATASET } from "./dataset.js";
import {
  costEfficiencyScore,
  iterationEfficiencyScore,
  keywordPresenceScore,
  overallQualityScore,
  successScore,
} from "./scorers.js";
import type { HealingEvalResult, HealingTestCase } from "./types.js";

/**
 * Mock task that simulates healing results for testing the eval infrastructure.
 * Replace with real HealLoop integration when ready for live evals.
 */
const mockTask = async (input: HealingTestCase): Promise<HealingEvalResult> => {
  // Simulate varying results based on the test case
  const baseIterations = input.tags?.includes("lint") ? 2 : 4;
  const baseCost = baseIterations * 0.05;

  // Add some randomness to simulate real behavior
  const jitter = Math.random() * 0.3;

  return {
    success: Math.random() > 0.2, // 80% success rate in mock
    iterations: Math.round(baseIterations + jitter * 3),
    toolCalls: Math.round(baseIterations * 2 + jitter * 5),
    costUSD: baseCost + jitter * 0.1,
    duration: 5000 + Math.random() * 10_000,
    finalMessage: `Fixed the issue in ${input.id}. Applied ${
      input.expected.expectedKeywords?.join(", ") ?? "standard fix"
    }.`,
  };
};

// TODO: Uncomment and implement when ready for live evals
// const liveTask = async (input: HealingTestCase): Promise<HealingEvalResult> => {
//   const client = new Client();
//   const registry = new ToolRegistry();
//   // Register tools...
//   const loop = new HealLoop(client, registry, { verbose: true });
//   const result = await loop.run(SYSTEM_PROMPT, input.errorPrompt);
//   return {
//     success: result.success,
//     iterations: result.iterations,
//     toolCalls: result.toolCalls,
//     costUSD: result.costUSD,
//     duration: result.duration,
//     finalMessage: result.finalMessage,
//   };
// };

const isLiveMode = process.env.EVAL_MODE === "live";
const task = isLiveMode ? mockTask : mockTask; // Replace with liveTask when ready

/**
 * Scorer functions for Braintrust.
 * Each returns { name, score } object.
 */
const successScorer = ({
  output,
  expected,
}: {
  output: HealingEvalResult;
  expected: HealingTestCase["expected"];
}) => ({
  name: "success",
  score: successScore({ output, expected }),
});

const iterationEfficiencyScorer = ({
  output,
  expected,
}: {
  output: HealingEvalResult;
  expected: HealingTestCase["expected"];
}) => ({
  name: "iteration_efficiency",
  score: iterationEfficiencyScore({ output, expected }),
});

const costEfficiencyScorer = ({
  output,
  expected,
}: {
  output: HealingEvalResult;
  expected: HealingTestCase["expected"];
}) => ({
  name: "cost_efficiency",
  score: costEfficiencyScore({ output, expected }),
});

const keywordPresenceScorer = ({
  output,
  expected,
}: {
  output: HealingEvalResult;
  expected: HealingTestCase["expected"];
}) => ({
  name: "keyword_presence",
  score: keywordPresenceScore({ output, expected }),
});

const overallQualityScorer = ({
  output,
  expected,
}: {
  output: HealingEvalResult;
  expected: HealingTestCase["expected"];
}) => ({
  name: "overall_quality",
  score: overallQualityScore({ output, expected }),
});

/**
 * Main evaluation entry point.
 */
Eval("Detent", {
  experimentName: `healing-eval-${new Date().toISOString().split("T")[0]}`,

  data: () =>
    HEALING_DATASET.map((tc) => ({
      input: tc,
      expected: tc.expected,
      metadata: {
        id: tc.id,
        tags: tc.tags,
        description: tc.description,
      },
    })),

  task: async (input) => {
    const result = await task(input);
    return result;
  },

  scores: [
    successScorer,
    iterationEfficiencyScorer,
    costEfficiencyScorer,
    keywordPresenceScorer,
    overallQualityScorer,
  ],
});
