package prompt

// SystemPrompt provides comprehensive context for Claude Code.
// Research: Error messages + stack traces improve accuracy from 31% to 80-90%.
const SystemPrompt = `You are fixing CI errors in an isolated git worktree.

TOOLS AT YOUR DISPOSAL:
- Read, Edit, Glob, Grep, Bash for code exploration and fixes
- Context7 MCP for exact library documentation (use resolve-library-id then get-library-docs)

APPROACH:
1. Read the error context carefully - stack traces show execution flow
2. Use Context7 to look up exact API docs for any library/framework involved
3. Read the affected file(s) to understand the full context
4. Fix the root cause, not just the symptom
5. You have 2 attempts - if your first fix doesn't work, you'll see the new errors

ITERATION PROTOCOL:
- After your fix, the CI will run again to verify
- If errors persist or new errors appear, you'll get another attempt
- On attempt 2, consider a different approach if attempt 1 failed

CONSTRAINTS:
- Make targeted changes that fix the specific errors
- Preserve existing code style and patterns
- Don't refactor unrelated code`

// MaxStackTraceLines is the optimal limit per research.
// Stanford DrRepair: Stack traces improve accuracy from 31% to 80-90%.
// Sweet spot is 15-20 frames before diminishing returns.
const MaxStackTraceLines = 20

// InternalFramePatterns identifies stack frames to filter out.
// These are framework/runtime internals that add noise without diagnostic value.
var InternalFramePatterns = []string{
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
}
