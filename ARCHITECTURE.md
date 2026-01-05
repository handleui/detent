# Detent Architecture

## Overview

Detent fixes CI failures automatically. Two environments:

- **CLI**: Local execution with `act`, git worktrees, Go healing
- **Cloud**: GitHub webhooks, E2B sandboxes, TS healing

## Current State

```
packages/core/              # Go - shared parsing logic
├── errors/                 # Error types, severity, grouping
├── extract/                # Extract errors from CI logs
├── tools/*                 # Language parsers (Go, TS, Rust, ESLint)
├── workflow/               # GitHub Actions YAML parsing
├── git/                    # Git operations (CLI uses worktrees)
├── heal/                   # Go healing loop (CLI only)
├── act/                    # Act runner (CLI only)
├── ci/                     # CI metadata types
└── progress/               # Progress reporter interface

apps/go-cli/                # Go CLI (renamed, deprecated)
apps/web/                   # Next.js landing
```

## Cloud Architecture (Planned)

```
┌─────────────────────────────────────────────────────────────────┐
│                        GitHub                                    │
│  workflow fails → webhook (workflow_run.completed)              │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  apps/api (Hono on Cloudflare Workers)                          │
│  ├── Better Auth (user accounts)                                │
│  ├── POST /webhooks/github                                      │
│  ├── Fetch GHA logs via GitHub API                              │
│  └── Orchestrate parsing + healing                              │
└──────────────┬─────────────────────────────┬────────────────────┘
               │                             │
               ▼                             ▼
┌──────────────────────────┐   ┌──────────────────────────────────┐
│  apps/parser (Go/Fly.io) │   │  E2B Sandbox                     │
│  POST /parse             │   │  ├── Clone repo                  │
│  └── Uses core/extract   │   │  ├── TS healing loop             │
│  └── Returns errors JSON │   │  ├── read_file → sandbox.fs      │
└──────────────────────────┘   │  ├── edit_file → sandbox.fs      │
                               │  ├── run_command → sandbox.proc  │
                               │  └── Return fix diff             │
                               └──────────────────────────────────┘
                                              │
                                              ▼
                               ┌──────────────────────────────────┐
                               │  Create PR with fix              │
                               └──────────────────────────────────┘
```

## Healing: CLI vs Cloud

Same algorithm, different runtimes:

| Step | CLI (Go) | Cloud (TS) |
|------|----------|------------|
| Get errors | `core/extract` | `apps/parser` API |
| Prompt | `core/heal/prompt` | `apps/api/healer.ts` |
| Claude call | Anthropic Go SDK | Anthropic TS SDK |
| read_file | `os.ReadFile()` | `sandbox.fs.read()` |
| edit_file | `os.WriteFile()` | `sandbox.fs.write()` |
| run_command | `exec.Command()` | `sandbox.process.start()` |
| Isolation | Git worktree | E2B sandbox |

Not duplicated logic - same algorithm, different execution environments.

## Shared Types (OpenAPI)

When building cloud, create `specs/parser.yaml`:

```yaml
openapi: 3.0.0
paths:
  /parse:
    post:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                logs:
                  type: string
      responses:
        '200':
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/ExtractedError'

components:
  schemas:
    ExtractedError:
      type: object
      properties:
        file: { type: string }
        line: { type: integer }
        message: { type: string }
        category: { type: string, enum: [lint, type, test, compile, runtime] }
        severity: { type: string, enum: [error, warning, info] }
```

Generate types:
- Go: `oapi-codegen`
- TS: `@hey-api/openapi-ts`

## Package Responsibilities

| Package | CLI | Cloud | Notes |
|---------|-----|-------|-------|
| `core/errors` | ✅ | ✅ | Shared types |
| `core/extract` | ✅ | ✅ | Parser service wraps this |
| `core/tools/*` | ✅ | ✅ | Language parsers |
| `core/workflow` | ✅ | ✅ | YAML parsing |
| `core/git` | ✅ | ❌ | Cloud uses E2B, not worktrees |
| `core/heal` | ✅ | ❌ | Cloud has TS heal in apps/api |
| `core/act` | ✅ | ❌ | Cloud uses real GHA |
| `core/progress` | ✅ | ✅ | Reporter interface |
| `core/agent` | ✅ | ❌ | CLI verbose mode for AI agents |

## Cost Projection

| Service | Free Tier | Paid |
|---------|-----------|------|
| Cloudflare Workers | 100k req/day | $5/mo |
| Fly.io | 3 shared-cpu VMs | $3-5/mo |
| E2B | ? | Usage-based |

## Key Decisions

1. **No WASM**: TinyGo regex broken, Cloudflare 1MB limit
2. **Go parser service**: 8K LOC regex too complex to port to TS
3. **TS healing in cloud**: E2B SDK is TS, better async/await
4. **Separate heal implementations**: Same algorithm, different tool backends
5. **OpenAPI for types**: Single source of truth, generate for both languages
