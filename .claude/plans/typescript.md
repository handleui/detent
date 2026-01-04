# TypeScript CLI Migration Plan

## Overview

Migrate detent CLI from Go to TypeScript while keeping Go for parsing (the only performance-critical part). This enables:

- **One healing implementation** shared between CLI and cloud
- **Unified persistence** (Drizzle) for local SQLite and cloud Neon
- **Simpler maintenance** (one language for most code)
- **Better Anthropic SDK ergonomics** (TS SDK is first-class)

---

## Current vs Target Architecture

### Current

```
apps/cli/           (Go - 12K LOC, everything)
packages/core/      (Go - parsing, healing, git, act, etc.)
```

### Target

```
apps/go-cli/        (Go - renamed, eventually deprecated)
apps/cli/           (TypeScript/Bun - new)
apps/parser/        (Go - HTTP service for cloud, uses packages/parsing)

packages/parsing/   (Go - renamed from core, parsing only)
packages/healing/   (TypeScript - shared healing loop)
packages/persistence/ (TypeScript - Drizzle, SQLite + Neon)
```

---

## Package Breakdown

### packages/parsing (Go, ~3K LOC) - KEEP

Renamed from `packages/core`, contains only parsing-critical code:

| Directory | Purpose | Keep |
|-----------|---------|------|
| `tools/` | Language parsers (8K regex patterns) | Yes |
| `extract/` | Error extraction, credential scrubbing | Yes |
| `errors/` | Error types, severity | Yes |
| `workflow/` | YAML parsing, injection | Yes |
| `ci/` | CI context types | Yes |

**Remove from this package:**
- `heal/` → becomes `packages/healing` (TS)
- `act/` → reimplemented in TS CLI
- `git/` → reimplemented in TS CLI (simple-git)
- `agent/` → trivial TS rewrite
- `progress/` → TS interface
- `retry/` → use npm package

### packages/healing (TypeScript, new)

The healing loop, shared between CLI and cloud:

```
packages/healing/
├── src/
│   ├── loop.ts           # Main healing loop
│   ├── client.ts         # Anthropic SDK wrapper
│   ├── prompt/
│   │   ├── system.ts     # System prompt
│   │   └── user.ts       # User prompt builder
│   ├── tools/
│   │   ├── registry.ts   # Tool registry
│   │   ├── read-file.ts
│   │   ├── edit-file.ts
│   │   ├── glob.ts
│   │   ├── grep.ts
│   │   └── run-command.ts
│   ├── types.ts          # HealResult, Config, etc.
│   └── index.ts
├── package.json
└── tsconfig.json
```

**Key exports:**
```typescript
export { heal } from './loop';
export type { HealConfig, HealResult, ToolContext } from './types';
```

### packages/persistence (TypeScript, new)

Drizzle-based persistence, works with SQLite (local) and Neon (cloud):

```
packages/persistence/
├── src/
│   ├── schema.ts         # Drizzle schema (mirrors current SQLite)
│   ├── adapters/
│   │   ├── sqlite.ts     # Local SQLite (better-sqlite3)
│   │   └── neon.ts       # Cloud Neon
│   ├── repositories/
│   │   ├── runs.ts
│   │   ├── errors.ts
│   │   ├── heals.ts
│   │   ├── assignments.ts
│   │   ├── fixes.ts
│   │   └── spend.ts
│   ├── migrations/       # Drizzle migrations
│   └── index.ts
├── drizzle.config.ts
├── package.json
└── tsconfig.json
```

### apps/cli (TypeScript/Bun, new)

New CLI implementation:

```
apps/cli/
├── src/
│   ├── commands/
│   │   ├── check.ts      # Run workflows
│   │   ├── heal.ts       # Heal errors
│   │   ├── clean.ts      # Cleanup worktrees
│   │   ├── config.ts     # Settings
│   │   └── workflows.ts  # Enable/disable jobs
│   ├── runner/
│   │   ├── executor.ts   # Spawn act process
│   │   ├── processor.ts  # Call Go parser binary
│   │   └── persister.ts  # Save to DB
│   ├── git/
│   │   ├── worktree.ts   # Worktree management
│   │   └── operations.ts # Git commands
│   ├── tui/
│   │   ├── prompts.ts    # API key, trust prompts
│   │   └── progress.ts   # Progress display (simplified)
│   ├── util/
│   │   ├── parser-bridge.ts  # Calls Go parser binary
│   │   └── config.ts     # Config loading
│   └── index.ts          # CLI entrypoint
├── package.json
└── tsconfig.json
```

**Parser bridge:**
```typescript
// src/util/parser-bridge.ts
import { spawn } from 'bun';

export const extractErrors = async (logs: string): Promise<ExtractedError[]> => {
  const proc = spawn(['detent-parser', 'extract'], {
    stdin: new TextEncoder().encode(logs),
  });
  const output = await new Response(proc.stdout).text();
  return JSON.parse(output);
};
```

---

## Migration Phases

### Phase 1: Prepare Go CLI for Coexistence

**Goal:** Rename and isolate so new TS CLI can be built alongside.

**Tasks:**
1. Rename `apps/cli` → `apps/go-cli`
2. Update build scripts, binary name (`detent-go`)
3. Rename `packages/core` → `packages/parsing`
4. Remove non-parsing code from `packages/parsing`:
   - Move `heal/`, `act/`, `git/`, `agent/`, `progress/`, `retry/` to `apps/go-cli/internal/legacy/`
   - Update imports
5. Create `detent-parser` binary that exposes parsing via stdin/stdout JSON
6. Test: `echo "<logs>" | detent-parser extract` outputs JSON

**Deliverable:** Go CLI still works, parsing is isolated, parser binary exists.

---

### Phase 2: Create packages/persistence

**Goal:** Drizzle schema matching current SQLite, works with both SQLite and Neon.

**Tasks:**
1. Create `packages/persistence` package structure
2. Port SQLite schema to Drizzle:
   - `runs`, `errors`, `run_errors` tables
   - `heals`, `heal_locks` tables
   - `assignments`, `suggested_fixes`, `fix_errors` tables
   - All indexes for cache lookups
3. Implement SQLite adapter (better-sqlite3 or Bun's built-in)
4. Implement Neon adapter (@neondatabase/serverless)
5. Create repository functions:
   - `recordRun()`, `getRunById()`
   - `recordErrors()`, `getErrorsByRunId()`, `getErrorByContentHash()`
   - `recordHeal()`, `getPendingHealByFileHash()` (cache lookup!)
   - `acquireHealLock()`, `releaseHealLock()`
   - `recordSpend()`, `getMonthlySpend()`
6. Write migrations matching current schema (v19)
7. Test: Create/read/update in both SQLite and Neon

**Deliverable:** `@detent/persistence` package published to workspace.

**Simplified schema (vs current):**
- Merge `heal_locks` into simple file-based lock (cross-platform)
- Remove deprecated fields from `heals` table
- Simplify `suggested_fixes` verification fields

---

### Phase 3: Create packages/healing

**Goal:** TypeScript healing loop that matches Go implementation.

**Tasks:**
1. Create `packages/healing` package structure
2. Port healing loop (`packages/core/heal/loop/loop.go` → `loop.ts`):
   - Message iteration with tool calls
   - Token tracking (input, output, cache read, cache write)
   - Cost calculation
   - Budget enforcement
3. Port tool implementations:
   - `read_file` - fs.readFile
   - `edit_file` - string replacement with validation
   - `glob` - fast-glob
   - `grep` - ripgrep via subprocess or @pnpm/find-packages
   - `run_command` - Bun.spawn with approval flow
4. Port prompt templates (`packages/core/heal/prompt/`)
5. Create Anthropic client wrapper with caching support
6. Test: Heal a simple error, verify output matches Go version

**Deliverable:** `@detent/healing` package, tested standalone.

**Key API:**
```typescript
import { heal } from '@detent/healing';

const result = await heal({
  errors: extractedErrors,
  worktreePath: '/tmp/worktree-1',
  config: {
    model: 'claude-sonnet-4-20250514',
    budgetPerRunUSD: 5.0,
    remainingMonthlyUSD: 50.0,
    timeout: 600_000,
  },
  onProgress: (event) => console.log(event),
});
```

---

### Phase 4: Create apps/cli (TypeScript)

**Goal:** New CLI with minimal commands, using shared packages.

**Tasks:**
1. Create `apps/cli` package structure
2. Implement parser bridge (calls `detent-parser` binary)
3. Implement `check` command:
   - Run act via Bun.spawn
   - Extract errors via parser bridge
   - Persist to SQLite via `@detent/persistence`
4. Implement `heal` command:
   - Load errors from DB
   - Check cache (skip if cached heal exists)
   - Create worktree
   - Call `@detent/healing`
   - Record spend
5. Implement `clean` command:
   - Remove worktrees
   - Optional: clear old DB entries
6. Implement `config` command:
   - API key management
   - Budget settings
7. Simple TUI:
   - API key prompt (if missing)
   - Trust prompt (first run per repo)
   - Progress output (text-based, not full TUI)
8. Build: `bun build` → `dist/detent`

**Deliverable:** Working TS CLI for `check`, `heal`, `clean`, `config`.

**Removed commands (simplification):**
- `frankenstein` (test tool, not needed)
- `allow` (shell allowlist, simplify to config)
- `workflows` (can use config or flags)
- `update` (use npm/bun update)

---

### Phase 5: Integration and Cutover

**Goal:** TS CLI becomes primary, Go CLI deprecated.

**Tasks:**
1. Update `detent` alias to point to TS CLI
2. Keep `detent-go` available for fallback
3. Test all commands against Go CLI behavior
4. Update documentation
5. CI: Build both, test both
6. Announce deprecation timeline for Go CLI

**Deliverable:** Users use TS CLI by default.

---

### Phase 6: Cloud Integration (Future)

**Goal:** Same healing package works in cloud.

**Tasks:**
1. `apps/api` imports `@detent/healing`
2. `apps/api` uses `@detent/persistence` with Neon adapter
3. E2B sandbox uses same tool implementations
4. Shared spend tracking across CLI and cloud

**Deliverable:** One healing implementation, two runtimes.

---

## Command Simplification

### Current (Go CLI)

```
detent check [workflow] [job]
detent heal [--force]
detent clean
detent config
detent allow <command>
detent workflows enable/disable
detent frankenstein
detent update
```

### Target (TS CLI)

```
detent check [workflow] [job]   # Run and extract errors
detent heal [--force]           # Heal errors (with cache)
detent clean                    # Cleanup worktrees + old data
detent config [key] [value]     # Get/set config
```

**Removed:**
- `allow` → merged into config (`detent config allow.commands`)
- `workflows` → use flags (`detent check --enable job1 --disable job2`)
- `frankenstein` → dev tool, not needed
- `update` → use package manager

---

## TUI Simplification

### Current (Go)

- Bubbletea-based interactive TUI
- Live job status tracker
- Progress bars
- Spinner animations

### Target (TS)

- Text-based output (like verbose mode)
- Simple prompts for API key, trust
- No live updates (unnecessary for CLI)
- Progress via log lines

**Rationale:** The fancy TUI is nice but not essential. Most users run in CI or pipe output. Simpler = faster to build, easier to maintain.

---

## Persistence Simplification

### Current Schema (19 migrations)

Complex schema with many fields for tracking every detail.

### Target Schema (fresh start)

Keep only what's needed:

```typescript
// Core tables
export const runs = sqliteTable('runs', {
  runId: text('run_id').primaryKey(),
  workflowName: text('workflow_name'),
  commitSha: text('commit_sha'),
  treeHash: text('tree_hash'),
  startedAt: integer('started_at'),
  completedAt: integer('completed_at'),
  exitCode: integer('exit_code'),
});

export const errors = sqliteTable('errors', {
  errorId: text('error_id').primaryKey(),
  contentHash: text('content_hash').notNull(),  // For dedup
  filePath: text('file_path'),
  lineNumber: integer('line_number'),
  message: text('message'),
  severity: text('severity'),
  source: text('source'),  // eslint, tsc, go, etc.
  firstSeenAt: integer('first_seen_at'),
}, (t) => ({
  contentIdx: index('idx_errors_content').on(t.contentHash),
}));

export const runErrors = sqliteTable('run_errors', {
  runId: text('run_id').notNull(),
  errorId: text('error_id').notNull(),
}, (t) => ({
  pk: primaryKey(t.runId, t.errorId),
}));

// Heal cache (the important part!)
export const heals = sqliteTable('heals', {
  healId: text('heal_id').primaryKey(),
  contentHash: text('content_hash').notNull(),  // Error content hash
  fileHash: text('file_hash').notNull(),        // File state hash
  filePath: text('file_path').notNull(),
  diffContent: text('diff_content'),            // The fix
  status: text('status').default('pending'),    // pending, applied, rejected
  costUsd: real('cost_usd'),
  createdAt: integer('created_at'),
}, (t) => ({
  cacheIdx: index('idx_heals_cache').on(t.contentHash, t.fileHash, t.status),
}));

// Spend tracking
export const spendLog = sqliteTable('spend_log', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  costUsd: real('cost_usd').notNull(),
  repoId: text('repo_id'),
  createdAt: integer('created_at'),
});
```

**Removed:**
- `assignments`, `suggested_fixes`, `fix_errors` → simplify to single `heals` table
- `heal_locks` → use file-based lock
- Many tracking fields → add back if needed

---

## Timeline Estimate

| Phase | Effort | Parallel |
|-------|--------|----------|
| Phase 1: Prepare Go CLI | 1 week | - |
| Phase 2: packages/persistence | 1 week | Yes |
| Phase 3: packages/healing | 2 weeks | Yes |
| Phase 4: apps/cli | 2 weeks | After 2+3 |
| Phase 5: Integration | 1 week | - |
| **Total** | **~5-6 weeks** | - |

With parallel agents on Phase 2 and 3, could be faster.

---

## Success Criteria

1. `detent check` extracts errors via Go parser, persists to SQLite
2. `detent heal` uses TS healing, respects cache, tracks spend
3. Cache hit rate matches Go CLI (no regression)
4. Cost per heal matches Go CLI (no token increase)
5. All existing tests pass
6. Cloud `apps/api` can import `@detent/healing` directly

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Parser bridge latency | Benchmark; keep binary warm |
| SQLite compatibility | Test with Bun's built-in, fallback to better-sqlite3 |
| TUI feature requests | Document: "use verbose mode", add features later |
| Healing behavior drift | Snapshot tests comparing Go vs TS output |

---

## Open Questions

1. **Parser distribution:** Ship Go binary with npm package, or require separate install?
2. **Config migration:** Auto-migrate from Go CLI config, or fresh start?
3. **DB migration:** Auto-migrate SQLite schema, or fresh start per-repo?

---

## Summary

| Component | Language | Notes |
|-----------|----------|-------|
| Parsing | Go | Binary, stdin/stdout JSON |
| Healing | TypeScript | Shared package |
| Persistence | TypeScript | Drizzle, SQLite + Neon |
| CLI | TypeScript | Bun, simplified commands |
| TUI | TypeScript | Text-based, minimal |
