# Heal Architecture

## Overview

The `heal` command uses AI to automatically fix errors found by `check`. Fixes are applied in a shared persistent worktree, verified by re-running the failing step, then presented to the user for approval.

## Run Identity

Each codebase state has a deterministic run ID:

```
runID = sha256(treeHash + commitSHA)[:16]
```

- Same code state = same runID = reuse worktree + cache
- Changed code = new runID = new worktree

## Flow

```
detent check
├── Compute runID from tree+commit hash
├── Cache hit? → return cached errors
├── Create worktree at ~/.detent/worktrees/{runID}/
├── Run workflows via act
├── Extract errors, save to SQLite
└── Return errors (worktree persists)

detent heal
├── Compute runID from tree+commit hash
├── Worktree exists? → reuse it
│   └── No? → run check first
├── Load errors from database
├── AI agentic loop (per file, priority order):
│   ├── Build prompt with error context
│   ├── AI uses tools to explore + fix
│   ├── run_step to verify fix
│   └── Retry if failed (max 2 attempts)
├── Extract diff via git diff
├── User reviews and approves/rejects
└── If approved → apply to main repo
```

## Shared Worktree Strategy

- **Location**: `~/.detent/worktrees/{runID}/`
- **Lifecycle**: Created by `check`, reused by `heal`
- **Persistence**: No auto-cleanup (manual cleanup command later)
- **Reuse**: If worktree exists at same commit, reuse it
- **Isolation**: Each runID gets its own worktree

## AI Tools

| Tool | Purpose | Schema |
|------|---------|--------|
| `read_file` | Read source code | `{path, offset?, limit?}` |
| `edit_file` | Apply targeted edits | `{path, old_string, new_string}` |
| `glob` | Find files by pattern | `{pattern, path?}` |
| `grep` | Search code | `{pattern, path?, type?}` |
| `run_step` | Verify fix by running CI step | `{workflow, job, step}` |
| `run_command` | Restricted bash (whitelist) | `{command}` |

## AI Integration

- Uses Anthropic SDK (Go) for API calls
- Agentic loop: prompt → tool calls → verify → repeat
- Max 2 attempts per file
- Haiku for simple fixes, Sonnet for complex reasoning (`--sonnet`)
- Files processed in priority order: compile > type-check > test > lint

## Storage

```
~/.detent/
├── config.yaml              # User config
├── worktrees/               # Persistent worktrees
│   └── {runID}/             # One per codebase state
└── (per-repo)
    └── .detent/
        └── detent.db        # SQLite: runs, errors, heals
```

## Cost Model

- ~$0.015/heal with Haiku
- ~$0.045/heal with Sonnet
- Prompt caching for repeated operations
- Token tracking per heal for visibility

## Current Implementation Status

| Component | Status |
|-----------|--------|
| Run ID system | ✅ Implemented |
| Shared worktrees | ✅ Implemented |
| Cache (skip if same state) | ✅ Implemented |
| Error extraction | ✅ Implemented |
| Prompt generation | ✅ Implemented |
| AI agentic loop | ❌ Not started |
| Tool definitions | ❌ Not started |
| User approval flow | ❌ Not started |
| Apply to main repo | ❌ Not started |

## Future Considerations

- Cleanup command for old worktrees
- Cloud sync for team collaboration
- Parallel worktrees for independent errors
- Batch API for CI/CD pipelines
