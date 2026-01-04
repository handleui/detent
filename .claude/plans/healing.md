# Parallel Agent Healing Architecture

## Overview

Parallel AI agents fixing errors in a shared codebase. Each agent iterates (edit → verify → adjust) until the fix works. The main cost is API tokens from agent loops.

This architecture uses **per-agent worktrees with dependency clustering**. Each agent gets an isolated worktree and a cluster of related errors. Agents run in parallel with zero coordination. The orchestrator merges proposals via git.

---

## Execution Pipeline

```
CI Failure
    ↓
Error Extraction (parse stdout/stderr)
    ↓
Autofix Phase (run formatters, apply mechanical fixes)
    ↓
Re-check (see what's left)
    ↓
Root Cause Deduplication (50 errors → 10 root causes)
    ↓
Dependency Clustering (group by edit zone overlap)
    ↓
Phase 1: Fast Fixes (lint, type, compile)
    ↓
Checkpoint + Re-check
    ↓
Phase 2: Slow Fixes (test failures)
    ↓
User Review + Apply
```

---

## Phase 0: Autofix

Before any AI prompting, run deterministic autofix tools. This eliminates trivially fixable errors that would waste tokens.

**Autofix runners by language:**

| Language | Tool | Command | Fixes |
|----------|------|---------|-------|
| Go | gofmt | `gofmt -w .` | Formatting |
| Go | goimports | `goimports -w .` | Import sorting, unused imports |
| TS/JS | Biome | `biome check --fix .` | Formatting, lint autofixes |
| TS/JS | ESLint | `eslint --fix .` | Lint autofixes |
| Python | Ruff | `ruff check --fix .` | Lint autofixes |
| Python | Black | `black .` | Formatting |
| Rust | rustfmt | `cargo fmt` | Formatting |

**Autofix behavior:**
1. Detect project language(s) from files or config
2. Run applicable autofix commands in worktree
3. Commit changes with message: `chore: autofix [tools used]`
4. Re-run check to get fresh error list
5. Only remaining errors proceed to AI healing

**Why autofix first:**
- Zero token cost for formatting fixes
- Sub-second execution vs minutes of AI iteration
- Reduces noise in error list (agent sees real problems, not style issues)
- Many "lint errors" are one `--fix` away from resolved

**Autofix is aggressive:** If a tool supports `--fix`, run it. False positives are rare for formatters. The subsequent re-check catches any regressions.

---

## Core Architecture

### 1. Per-Agent Worktrees

Each agent gets its own git worktree. Worktrees share the .git directory and only duplicate working files.

**Benefits:**
- Zero coordination between agents
- Agents run builds/tests simultaneously without interference
- No region lock tracking or line number drift
- Cross-file fixes work naturally
- Git handles merge conflicts at the end

**Disk management:**
- Use sparse checkout for large repos (only checkout cluster-relevant files)
- Pre-check available disk space before spawning N worktrees
- Create worktrees in waves if disk constrained (10 at a time, not 50)
- Cleanup is defer-safe (worktrees removed even on panic)

### 2. Dependency Clustering

Errors are grouped by dependency relationships, not file location.

**Algorithm:**
1. Build file dependency graph from imports
2. For each error, compute its "edit zone" (files that might need changes)
3. Errors with overlapping edit zones go to the same agent
4. Semantic clustering pass: group errors sharing test subjects or type definitions

**Example:** Three "undefined: Config" errors in different files become one cluster—they all need the same fix.

**Cluster limits:**
- Max 10 errors per cluster (context window)
- Max 5 files per cluster (complexity)
- Min 3 errors for parallel mode (worktree overhead not worth it otherwise)

### 3. Three-Phase Execution

**Phase 0: Autofix** (no AI)
- Run deterministic formatters and lint fixers
- Commit results, re-check
- Zero token cost

**Phase 1: Fast Fixes** (AI)
- Lint errors, type errors, compile errors
- Verification under 5 seconds
- Higher parallelism, smaller clusters
- Sonnet with `temperature: 0` for deterministic output

**Phase 2: Slow Fixes** (AI)
- Test failures, runtime errors
- Verification 10+ seconds
- Lower parallelism, larger clusters
- Sonnet with `temperature: 0`
- Multiple verification passes for flaky test detection

Between phases: commit fixes, re-run check. Phase 1 fixes often resolve Phase 2 errors (missing import → test failure).

### 4. Edit-Based Fixes

Fixes are surgical edits: find exact string, replace with new string. Not line numbers (which drift), not full file content (which can't merge).

**Validation:**
- old_string must appear exactly once in file
- Zero matches = stale edit (file changed)
- Multiple matches = ambiguous edit (rejected)

**Reverse edits:** Each proposal stores its reverse (swap old/new). Enables rollback if a merged fix causes new problems.

### 5. Ralph Wiggum Agent Loop

Agents run in a hook-enforced loop until all assigned errors are addressed.

**Loop behavior:**
1. Agent receives full error list + pre-fetched file content
2. Agent fixes errors, calls `suggest_fix` for each
3. Stop hook checks: "All errors addressed?"
4. If not, re-prompt: "Fixed E1, E3. Missing: E2, E4, E5. Continue."
5. Loop continues until all errors have `suggest_fix` or `report_unfixable`

**Context pruning:** After each `suggest_fix`, remove that error from subsequent prompts. Reduces input tokens 20-40% per iteration.

**Early exit signals:**
- Same error after 2 fix attempts → give up
- Duplicate edit fingerprint → deterministic failure
- Error count increased >50% → destructive fix
- 3 failures on same error → force `report_unfixable`

**Failed edit tracking:** Store fingerprints of failed edits in DB. Inject into future prompts: "These edits were tried before and failed." Prevents repeating mistakes across runs.

### 6. Model Selection

All AI healing uses Sonnet with `temperature: 0`. Deterministic output reduces retry loops.

Haiku is reserved for future experimentation with truly mechanical fixes (if Sonnet proves too slow). Current approach: don't optimize prematurely.

### 7. Budget and Limits

One total budget per run. Agent iteration limit (max 10) is the primary throttle.

If budget exhausts mid-run, remaining agents fail gracefully with `report_unfixable`. User can increase budget and re-run.

### 8. Orchestrator

The orchestrator is pure code, not AI:

**Does:**
- Run autofix phase
- Collect errors from check
- Root cause deduplication
- Dependency clustering
- Create worktrees (sparse checkout for large repos)
- Spawn agents in parallel
- Collect proposals for user review
- Track error resolution provenance

**Does NOT:**
- Apply fixes automatically
- Use AI for coordination
- Retry failed agents (Ralph loop handles iteration)

---

## Agent Tools

| Tool | Purpose |
|------|---------|
| `read_file` | Read source code (usually pre-fetched) |
| `edit_file` | Modify source code |
| `glob`, `grep` | Find files and search content |
| `run_check` | Run verification command |
| `suggest_fix` | Submit verified fix proposal |
| `report_unfixable` | Exit with explanation |
| Context7 MCP | Library documentation lookup |

### suggest_fix

Submit a verified fix proposal. Agent can call multiple times—once per error or group.

**Input:**
- Error IDs this fix addresses
- List of edits (file, old_string, new_string)
- Explanation
- Confidence score (1-100)
- Verification evidence (command, exit code, output)

**Validation:**
- Error IDs must be assigned to this agent
- Each edit's old_string must exist exactly once
- Verification exit code must be 0
- For test fixes: verification must pass 2/2 times (flaky detection)

**Behavior:**
- Computes content-addressed proposal ID
- Stores proposal with pending status
- Computes and stores reverse edits for rollback
- Does NOT release Stop hook—agent continues

### report_unfixable

Exit without a fix, but with explanation.

**Input:**
- Error IDs that couldn't be fixed
- What was tried
- Why it failed
- Suggestions for manual resolution

Agents never silently fail. Every error gets either `suggest_fix` or `report_unfixable`.

---

## Cost Optimization

Main cost is input tokens. Each iteration re-sends conversation history. Optimizations target: fewer iterations, smaller context, cache hits, early exit.

### Prompt Caching

Anthropic caches identical prefixes at 90% discount. Use 1-hour TTL for cross-phase cache survival:

```json
{
  "cache_control": { "type": "ephemeral", "ttl": "1h" }
}
```

**Prompt structure (static first):**
1. System prompt → cached (1h TTL)
2. Tool definitions → auto-cached
3. Error cluster + file content → cached per agent
4. Dynamic content (tool results) → not cached

### Batch API for Analysis

Anthropic's Message Batches API provides 50% cost reduction for async requests. Use for initial error analysis:

**Hybrid approach:**
1. First prompt per agent: "Analyze errors, propose edits" → Batch API (50% off)
2. Subsequent iterations with tools → Real-time API

Batch API doesn't support tool use, so verification loops remain real-time. But first-pass analysis (often 40% of tokens) gets the discount.

### Context Pruning

After each successful `suggest_fix`, remove that error from conversation:

```
Iteration 1: "Fix E1, E2, E3, E4, E5"
Iteration 2: "Fix E3, E4, E5" (E1, E2 fixed)
Iteration 3: "Fix E5" (E3, E4 fixed)
```

Reduces input tokens 20-40% per iteration.

### Pre-Fetched Context

Include file content in initial prompt—don't make agent read files:

```
Error: main.go:10 undefined: Config
File context (lines 1-60):
[content]
```

Saves 1 iteration per file. 30 agents × 2 files = 60 fewer API calls.

### Dynamic Response Limits

Set `max_tokens` by complexity:

| Task | Max tokens |
|------|------------|
| Single file fix | 2048 |
| Cross-file fix | 4096 |
| Complex test fix | 8192 |

### Speculative Cache Warming

While building clusters (2-5 seconds), pre-warm the cache in background:

```go
go func() {
    client.Messages.Create(ctx, MessageRequest{
        MaxTokens: 1,
        System:    systemPrompt, // With cache_control
        Messages:  []Message{{Role: "user", Content: "x"}},
    })
}()

clusters := buildDependencyClusters(errors) // Takes time
// Now agents start with warm cache
```

### Cost Model

**200 errors, typical run:**

| Phase | Errors | Agents | Cost |
|-------|--------|--------|------|
| Autofix | 200 → 120 | 0 | $0 |
| Dedup | 120 → 60 | 0 | $0 |
| Phase 1 | 45 lint | 12 | ~$3 |
| Phase 2 | 15 test | 4 | ~$5 |
| **Total** | | **16** | **~$8** |

With Batch API for first iteration: ~$6 (25% savings).

---

## Verification

### Flaky Test Detection

Test failures require multiple verification passes:

```go
type VerificationConfig struct {
    Passes       int  // How many times to run
    RequireAll   bool // All must pass
}

// Lint errors: 1 pass
// Test failures: 2 passes, all must succeed
```

If verification passes once but fails once, the fix is unreliable. Agent must try again or `report_unfixable`.

### Error Resolution Tracking

Track how each error was resolved:

```go
type ErrorResolution struct {
    ErrorID    string
    ResolvedBy string // "autofix:gofmt" | "fix:proposal_123" | "cascade:phase1"
}
```

Distinguishes between:
- Autofix resolved it
- AI fix resolved it
- Phase 1 fix cascaded to resolve Phase 2 error
- Still unresolved

---

## Implementation Phases

### Phase 1: Autofix Infrastructure
- Language detection from project files
- Autofix runner registry (gofmt, biome, ruff, etc.)
- Commit autofix changes
- Re-check integration

### Phase 2: Schema and Types
- Edit type (file_path, old_string, new_string, reverse)
- ErrorCluster with root cause fingerprint
- FailedEdit tracking table
- ErrorResolution tracking
- Database migration

### Phase 3: Root Cause Deduplication
- Extract identifiers from error messages
- Group by fingerprint before clustering
- 50 "undefined: X" → 1 root cause

### Phase 4: Dependency Graph + Clustering
- Parse imports (go/parser, regex for TS)
- Compute edit zones
- Semantic clustering (shared test subjects, type definitions)
- Output: ErrorClusters with pre-fetched content

### Phase 5: Worktree Manager
- Sparse checkout support
- Disk space pre-check
- Wave-based creation for large agent counts
- Defer-safe cleanup

### Phase 6: Agent Loop + Tools
- suggest_fix with validation + reverse edit computation
- report_unfixable with explanation
- Stop hook integration
- Context pruning between iterations
- Failed edit injection
- Early exit signal detection

### Phase 7: Prompt Optimization
- Cache control with 1h TTL
- Batch API integration for first iteration
- Pre-fetched file content (50 lines around error)
- Dynamic max_tokens
- Speculative cache warming

### Phase 8: Verification
- Flaky test detection (multi-pass)
- Error resolution tracking
- Provenance recording

### Phase 9: Parallel Orchestrator
- Autofix phase runner
- Phase 1: fast errors, high parallelism
- Checkpoint, re-check
- Phase 2: slow errors, lower parallelism

### Phase 10: User Review CLI
- `heal list`: show pending proposals as diffs
- `heal apply`: select proposals to apply
- `heal reject`: mark as rejected
- `heal rollback`: reverse a previously applied fix
- `heal unfixable`: show agent explanations

---

## Explicit Non-Goals

**No auto-apply.** User reviews diffs, chooses what to apply.

**No incomplete exits.** Every error gets `suggest_fix` or `report_unfixable`.

**No AI orchestrator.** Clustering is pure code. AI only runs in agents.

**No worktree persistence.** Ephemeral. Created at start, deleted at end.

**No Haiku experimentation yet.** Sonnet-only until we have data showing mechanical fixes are bottlenecked on cost, not quality.

---

## Summary

| Decision | Choice |
|----------|--------|
| Pre-AI phase | Autofix (formatters, lint --fix) |
| Isolation | Per-agent worktrees (sparse checkout) |
| Grouping | Root cause dedup → dependency + semantic clustering |
| Agent loop | Ralph Wiggum (context pruning, failed edit tracking) |
| Model | Sonnet, temperature 0 |
| Caching | 1h TTL, speculative warming |
| Batch API | First iteration (50% discount) |
| Verification | Multi-pass for tests (flaky detection) |
| Budget | One total, iteration limit is throttle |
| Phases | Autofix → Fast → Slow |
| Rollback | Reverse edits stored per proposal |
