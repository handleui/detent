import type { HealingTestCase } from "./types.js";

/**
 * Golden dataset of error scenarios for evaluation.
 *
 * Start small and expand as you encounter real CI failures.
 * Each case should represent a distinct error pattern.
 */
export const HEALING_DATASET: HealingTestCase[] = [
  {
    id: "ts-undefined-property",
    description:
      "TypeScript error: accessing property on potentially undefined",
    errorPrompt: `# CI Error Report

## Error 1: TypeScript Compilation Error
**File:** src/utils/config.ts:42
**Category:** type-check

\`\`\`
error TS2532: Object is possibly 'undefined'.

  40 |   const loadConfig = (path: string) => {
  41 |     const config = configs.get(path);
> 42 |     return config.value;
     |            ^^^^^^
  43 |   };
\`\`\`

Please fix this error following the research → understand → fix → verify workflow.`,
    expected: {
      shouldSucceed: true,
      expectedKeywords: ["optional chaining", "?."],
      maxIterations: 5,
      maxCostUSD: 0.5,
    },
    tags: ["typescript", "type-check", "null-safety"],
  },

  {
    id: "go-unused-variable",
    description: "Go compilation error: declared but not used",
    errorPrompt: `# CI Error Report

## Error 1: Go Compilation Error
**File:** internal/handler/api.go:28
**Category:** compile

\`\`\`
./api.go:28:2: declared and not used: resp
\`\`\`

**Stack trace:**
\`\`\`
internal/handler/api.go:28:2
\`\`\`

Please fix this error following the research → understand → fix → verify workflow.`,
    expected: {
      shouldSucceed: true,
      maxIterations: 3,
      maxCostUSD: 0.3,
    },
    tags: ["go", "compile", "unused-variable"],
  },

  {
    id: "jest-assertion-failure",
    description: "Jest test failure with expected vs received mismatch",
    errorPrompt: `# CI Error Report

## Error 1: Test Failure
**File:** src/services/auth.test.ts:45
**Category:** test

\`\`\`
FAIL src/services/auth.test.ts
  ● AuthService › validateToken › should return false for expired tokens

    expect(received).toBe(expected) // Object.is equality

    Expected: false
    Received: true

      43 |     const token = createExpiredToken();
      44 |     const result = authService.validateToken(token);
    > 45 |     expect(result).toBe(false);
         |                    ^
      46 |   });
\`\`\`

Please fix this error following the research → understand → fix → verify workflow.`,
    expected: {
      shouldSucceed: true,
      expectedKeywords: ["expired", "token", "validation"],
      maxIterations: 8,
      maxCostUSD: 0.8,
    },
    tags: ["jest", "test", "auth"],
  },

  {
    id: "eslint-unused-import",
    description: "ESLint error: unused import",
    errorPrompt: `# CI Error Report

## Error 1: Lint Error
**File:** src/components/Button.tsx:2
**Category:** lint

\`\`\`
error  'useState' is defined but never used  @typescript-eslint/no-unused-vars

  1 | import React from 'react';
> 2 | import { useState, useEffect } from 'react';
    |          ^^^^^^^^
  3 |
  4 | export const Button = ({ onClick, children }) => {
\`\`\`

Please fix this error following the research → understand → fix → verify workflow.`,
    expected: {
      shouldSucceed: true,
      maxIterations: 3,
      maxCostUSD: 0.2,
    },
    tags: ["eslint", "lint", "unused-import"],
  },

  {
    id: "missing-dependency",
    description: "Module not found error",
    errorPrompt: `# CI Error Report

## Error 1: Module Resolution Error
**File:** src/index.ts:5
**Category:** compile

\`\`\`
Cannot find module 'lodash-es' or its corresponding type declarations.

  3 | import { config } from './config';
  4 | import { logger } from './logger';
> 5 | import { debounce } from 'lodash-es';
    | ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
  6 |
\`\`\`

Please fix this error following the research → understand → fix → verify workflow.`,
    expected: {
      shouldSucceed: true,
      expectedKeywords: ["install", "package.json", "lodash"],
      maxIterations: 5,
      maxCostUSD: 0.4,
    },
    tags: ["module", "dependency", "compile"],
  },
];

/**
 * Get test cases by tag.
 */
export const getTestCasesByTag = (tag: string): HealingTestCase[] =>
  HEALING_DATASET.filter((tc) => tc.tags?.includes(tag));

/**
 * Get a single test case by ID.
 */
export const getTestCaseById = (id: string): HealingTestCase | undefined =>
  HEALING_DATASET.find((tc) => tc.id === id);
