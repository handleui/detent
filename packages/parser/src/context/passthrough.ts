/**
 * Passthrough context parser.
 * Passes lines through unchanged without any CI-specific processing.
 * Use this when parsing raw log output without CI prefixes.
 */

import type { ContextParser, ParseLineResult } from "./types.js";

/**
 * Create a passthrough context parser.
 * Lines pass through unchanged with empty context.
 */
export const createPassthroughParser = (): ContextParser => ({
  parseLine(line: string): ParseLineResult {
    return {
      ctx: { job: "", step: "", isNoise: false },
      cleanLine: line,
      skip: false,
    };
  },
});

/**
 * Singleton instance for convenience.
 */
export const passthroughParser: ContextParser = createPassthroughParser();
