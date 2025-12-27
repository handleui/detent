# Heal Architecture

## Overview

The `heal` command uses AI to automatically fix errors found by `check`. Fixes are applied in an isolated git worktree, verified by re-running the failing step, then presented to the user for approval.

## Flow

- `detent check` runs workflows, extracts errors, stores in SQLite
- `detent heal` reads errors from database
- Creates a git worktree for isolation
- AI edits files and runs verification until the step passes (or max attempts)
- Extracts diff via `git diff`
- User reviews and approves/rejects
- If approved, changes applied to main worktree

## AI Integration

- Uses Anthropic SDK (Go) for API calls
- Two tools: `edit_file` and `run_verify`
- `tool_choice` forces structured output
- Agentic loop: prompt → edit → verify → repeat
- Haiku for simple fixes, Sonnet for complex reasoning

## Worktree Strategy

- One worktree per heal session
- Sequential fixes within worktree
- Worktree provides natural sandboxing
- Cleanup after user decision (or keep for debugging)

## Storage

- All runs and heals logged to global SQLite database
- Worktrees stored in global cache directory
- Config stored separately from cache
- Per-repo isolation via repo ID hash

## Cost Model

- ~$0.015/heal with Haiku
- ~$0.045/heal with Sonnet
- Prompt caching for repeated operations
- Token tracking per heal for visibility

## Future Considerations

- Cloud sync for team collaboration
- Parallel worktrees for independent errors
- Batch API for CI/CD pipelines
- Claude Code subprocess as alternative backend
