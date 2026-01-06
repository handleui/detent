/**
 * Tool-specific error parsers (CI-agnostic).
 *
 * Each parser handles a specific tool's output format (error CONTENT).
 * These parsers work identically whether the log comes from CI or local execution.
 *
 * Parser priorities (higher = checked first):
 * - Language parsers (80): go, typescript, python, rust, eslint
 * - Infrastructure parser (70): npm, docker, git, shell errors
 * - Generic fallback (10): Catches unknown error patterns
 *
 * For CI context parsers (log FORMAT), see ../context/
 */

export { createESLintParser } from "./eslint.js";
export { createGenericParser } from "./generic.js";
export { createGolangParser, GolangParser } from "./golang.js";
export { createInfrastructureParser } from "./infrastructure.js";
export { createPythonParser, PythonParser } from "./python.js";
export { createRustParser } from "./rust.js";
export { createTypeScriptParser, TypeScriptParser } from "./typescript.js";
