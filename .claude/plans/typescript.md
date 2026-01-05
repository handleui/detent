# TypeScript CLI Migration Plan

## Overview

Migrate detent CLI from Go to TypeScript while keeping Go for parsing (via HTTP service). This enables:

- **One healing implementation** shared between CLI and cloud
- **Unified persistence** (Drizzle) for local SQLite and cloud Neon
- **Simpler maintenance** (one language for most code)
- **Better Anthropic SDK ergonomics** (TS SDK is first-class)

---

## Current vs Target Architecture

### Current

```
apps/cli/           (Go - 12K LOC, everything)
apps/parser/        (Go - HTTP service, already exists!)
packages/core/      (Go - parsing, healing, git, act, etc.)
```

### Target

```
apps/go-cli/        (Go - renamed from apps/cli, eventually deprecated)
apps/cli/           (TypeScript/Bun - new primary CLI)
apps/parser/        (Go - HTTP service, UNCHANGED)

packages/core/      (Go - UNCHANGED, used by go-cli and parser)
packages/healing/   (TypeScript - shared healing loop)
packages/persistence/ (TypeScript - Drizzle, SQLite + Neon)
```

### Key Insight: Keep Go Parser as HTTP Service

**Why HTTP instead of stdin/stdout?**
- `apps/parser` already exists and is deployed on Fly.io
- HTTP is simpler than process spawning/IPC
- Parser as a service is valuable (could offer to third parties)
- One localhost HTTP call (~1ms) is invisible compared to Claude API calls (~1-5s)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    PARSER SERVICE (Go)                           │
│                    apps/parser - UNCHANGED                       │
│                                                                  │
│  POST /parse { logs: "..." } → { errors: [...] }                │
│  Deployed: https://parser.detent.sh (Fly.io)                    │
│  Local: http://localhost:8080                                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                         HTTP calls
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
    ┌─────────────────┐             ┌─────────────────┐
    │  apps/cli        │             │   apps/api      │
    │  (NEW - TS/Bun)  │             │   (cloud)       │
    └────────┬─────────┘             └────────┬────────┘
             │                                │
             └────────────────┬───────────────┘
                              ▼
            ┌─────────────────────────────────┐
            │      SHARED PACKAGES (TS)        │
            ├─────────────────────────────────┤
            │  packages/healing               │
            │  • Claude API integration       │
            │  • Tool execution               │
            │  • Prompt templates             │
            ├─────────────────────────────────┤
            │  packages/persistence           │
            │  • Drizzle schema               │
            │  • SQLite (local) / Neon (cloud)│
            │  • Repositories                 │
            └─────────────────────────────────┘
```

---

## Migration Phases

### Phase 1: Prepare for Coexistence

**Goal:** Rename Go CLI so new TS CLI can take the `apps/cli` path.

**Tasks:**
1. Rename `apps/cli` → `apps/go-cli`
2. Update build scripts, turbo.json, go.work
3. Update CI workflows (working directories)
4. Update binary name in .goreleaser.yaml
5. Update Go module path: `github.com/detent/cli` → `github.com/detent/go-cli`
6. Update all internal imports (35+ files)
7. Keep `packages/core` UNCHANGED (go-cli and parser still use it)

**NOT doing (simplified from original plan):**
- ~~Rename packages/core → packages/parsing~~ (not needed)
- ~~Move heal/git/act to legacy/~~ (not needed)
- ~~Create stdin/stdout parser mode~~ (use HTTP instead)

**Deliverable:** Go CLI works at `apps/go-cli`, `apps/cli` path is free.

---

### Phase 2: packages/persistence

**Goal:** Drizzle schema matching current SQLite, works with both SQLite and Neon.

**Structure:**
```
packages/persistence/
├── src/
│   ├── schema.ts           # Drizzle schema
│   ├── adapters/
│   │   ├── sqlite.ts       # Local SQLite (better-sqlite3)
│   │   └── neon.ts         # Cloud Neon
│   ├── repositories/
│   │   ├── runs.ts
│   │   ├── errors.ts
│   │   ├── heals.ts
│   │   └── spend.ts
│   └── index.ts
├── drizzle.config.ts
└── package.json
```

**Schema (simplified):**
```typescript
export const runs = sqliteTable('runs', {
  runId: text('run_id').primaryKey(),
  workflowName: text('workflow_name'),
  commitSha: text('commit_sha'),
  startedAt: integer('started_at'),
  exitCode: integer('exit_code'),
});

export const errors = sqliteTable('errors', {
  errorId: text('error_id').primaryKey(),
  contentHash: text('content_hash').notNull(),
  filePath: text('file_path'),
  message: text('message'),
  severity: text('severity'),
});

export const heals = sqliteTable('heals', {
  healId: text('heal_id').primaryKey(),
  contentHash: text('content_hash').notNull(),
  fileHash: text('file_hash').notNull(),
  diffContent: text('diff_content'),
  status: text('status'),
  costUsd: real('cost_usd'),
});

export const spendLog = sqliteTable('spend_log', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  costUsd: real('cost_usd').notNull(),
  createdAt: integer('created_at'),
});
```

**Deliverable:** `@detent/persistence` package works with SQLite and Neon.

---

### Phase 3: packages/healing

**Goal:** TypeScript healing loop that matches Go implementation.

**Structure:**
```
packages/healing/
├── src/
│   ├── loop.ts             # Main healing loop
│   ├── client.ts           # Anthropic SDK wrapper
│   ├── prompt/
│   │   ├── system.ts       # System prompt template
│   │   └── user.ts         # User prompt builder
│   ├── tools/
│   │   ├── registry.ts     # Tool registry
│   │   ├── read-file.ts
│   │   ├── edit-file.ts
│   │   ├── glob.ts
│   │   ├── grep.ts
│   │   └── run-command.ts
│   ├── types.ts
│   └── index.ts
└── package.json
```

**Key API:**
```typescript
import { heal } from '@detent/healing';

const result = await heal({
  errors: parsedErrors,
  worktreePath: '/path/to/worktree',
  config: {
    model: 'claude-sonnet-4-20250514',
    budgetPerRunUSD: 5.0,
    timeout: 600_000,
  },
  onProgress: (event) => console.log(event),
});
```

**Deliverable:** `@detent/healing` package, tested standalone.

---

### Phase 4: apps/cli (TypeScript)

**Goal:** New CLI with minimal commands, using shared packages.

**Structure:**
```
apps/cli/
├── src/
│   ├── commands/
│   │   ├── check.ts        # Run workflows
│   │   ├── heal.ts         # Heal errors
│   │   ├── clean.ts        # Cleanup worktrees
│   │   └── config.ts       # Settings
│   ├── runner/
│   │   ├── executor.ts     # Spawn act process
│   │   ├── parser.ts       # HTTP call to apps/parser
│   │   └── persister.ts    # Save to DB
│   ├── git/
│   │   ├── worktree.ts     # Worktree management
│   │   └── operations.ts   # Git commands (simple-git)
│   └── index.ts
└── package.json
```

**Parser Bridge (HTTP):**
```typescript
// src/runner/parser.ts
const PARSER_URL = process.env.PARSER_URL ?? 'http://localhost:8080';

export const parseErrors = async (logs: string) => {
  const response = await fetch(`${PARSER_URL}/parse`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ logs }),
  });
  return response.json();
};
```

**Deliverable:** Working TS CLI for `check`, `heal`, `clean`, `config`.

---

### Phase 5: Integration and Cutover

**Goal:** TS CLI becomes primary, Go CLI deprecated.

**Tasks:**
1. Update `detent` alias to point to TS CLI
2. Keep Go CLI available as `detent-go` for fallback
3. Test all commands against Go CLI behavior
4. Update documentation
5. CI: Build both, test both

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

## Execution Timeline

| Phase | Duration | Agents | Dependencies |
|-------|----------|--------|--------------|
| 1: Rename Go CLI | 1 day | 2 | None |
| 2: packages/persistence | 1 week | 2 | After 1 |
| 3: packages/healing | 2 weeks | 3 | After 1 (parallel with 2) |
| 4: apps/cli | 2 weeks | 3 | After 2 + 3 |
| 5: Integration | 1 week | 1 | After 4 |
| **Total** | **~4-5 weeks** | - | - |

**Phases 2 and 3 can run in parallel** after Phase 1 completes.

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
- `allow` → merged into config
- `workflows` → use flags
- `frankenstein` → dev tool
- `update` → use package manager

---

## Success Criteria

1. `detent check` extracts errors via HTTP parser, persists to SQLite
2. `detent heal` uses TS healing, respects cache, tracks spend
3. Cache hit rate matches Go CLI (no regression)
4. Cost per heal matches Go CLI (no token increase)
5. Cloud `apps/api` can import `@detent/healing` directly
6. Parser HTTP service unchanged, still works

---

## Summary

| Component | Language | Notes |
|-----------|----------|-------|
| Parsing | Go | HTTP service (apps/parser), unchanged |
| Go CLI | Go | Renamed to apps/go-cli, deprecated |
| TS CLI | TypeScript | apps/cli, primary |
| Healing | TypeScript | Shared package |
| Persistence | TypeScript | Drizzle, SQLite + Neon |
