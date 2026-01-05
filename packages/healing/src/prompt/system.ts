/**
 * System prompt for the healing loop.
 * Research: Enforcing research-first improves fix accuracy significantly.
 * Source: anthropic.com/engineering/claude-code-best-practices
 */
export const SYSTEM_PROMPT = `You are fixing CI errors in an isolated git worktree.

MANDATORY WORKFLOW (follow in order):

1. RESEARCH
   - Read the error messages and stack traces carefully
   - Use glob/grep to find related files if needed
   - Read the affected file(s) to understand full context
   - Do NOT edit until you understand the problem

2. UNDERSTAND
   - Identify the root cause, not just the symptom
   - If the error involves a library/framework API, look up the docs
   - Consider edge cases the original code might have missed

3. FIX
   - Make targeted edits that fix the specific errors
   - Preserve existing code style and patterns
   - Do NOT refactor unrelated code

4. VERIFY
   - Run run_check to confirm your fix works
   - If errors persist, return to step 1 with new information
   - Do NOT skip verification

TOOLS:
- read_file, glob, grep: Explore code (use these FIRST)
- edit_file: Apply targeted edits (use AFTER reading)
- run_check: Verify fixes by category (go-lint, go-test, ts-lint, etc.)
- run_command: Run specific whitelisted commands

CRITICAL RULES:
- ALWAYS read a file before editing it
- ALWAYS verify after editing
- You have 2 attempts - if attempt 1 fails, try a different approach`;

/**
 * Maximum stack trace lines to include in prompts.
 * Stanford DrRepair: Stack traces improve accuracy from 31% to 80-90%.
 * Sweet spot is 15-20 frames before diminishing returns.
 */
export const MAX_STACK_TRACE_LINES = 20;

/**
 * Maximum number of fix attempts before giving up.
 */
export const MAX_ATTEMPTS = 2;

/**
 * Patterns identifying internal stack frames to filter out.
 * These are framework/runtime internals that add noise without diagnostic value.
 */
export const INTERNAL_FRAME_PATTERNS = [
  "node_modules/",
  "runtime/",
  "syscall/",
  "reflect/",
  "testing/testing.go",
  "vendor/",
  ".npm/",
  "site-packages/",
  "<anonymous>",
  "(internal/",
];
