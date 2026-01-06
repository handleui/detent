// biome-ignore-all lint/performance/noBarrelFile: This is the context module's public API

/**
 * CI context parsers.
 * Handle CI-specific log FORMAT (prefixes, timestamps, noise filtering).
 */

// Parsers
export { actParser, createActParser } from "./act.js";
export { createGitHubContextParser, githubParser } from "./github.js";
export { createPassthroughParser, passthroughParser } from "./passthrough.js";
// Types
export type { ContextParser, LineContext, ParseLineResult } from "./types.js";
