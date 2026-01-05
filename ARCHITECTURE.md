# Detent Architecture

## Overview

Detent fixes CI failures automatically using the CLI with `act`, git worktrees, and Claude-based healing.

## Current State

```
apps/cli/                   # TypeScript CLI (main)
├── src/commands/           # CLI commands (check, heal, etc.)
├── src/runner/             # Act runner, workflow execution
├── src/tui/                # Terminal UI components
└── scripts/                # Build & upload scripts

apps/go-cli/                # Go CLI (deprecated)
apps/web/                   # Next.js landing
apps/docs/                  # Documentation

packages/parser/            # TypeScript error parsing
├── src/parsers/            # Language parsers (Go, TS, Rust, Python, ESLint)
├── src/extractor.ts        # Extract errors from CI logs
└── src/types.ts            # Error types

packages/git/               # Git operations (worktrees, branches)
packages/healing/           # Claude-based healing loop
packages/persistence/       # Caching & state persistence
packages/core/              # Go shared logic (legacy)
packages/ui/                # Shared UI components
packages/typescript-config/ # Shared TS config
```

## CLI Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        detent check                              │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  apps/cli                                                        │
│  ├── Parse .github/workflows/*.yml                              │
│  ├── Run workflows with act (in worktree)                       │
│  ├── Extract errors via packages/parser                         │
│  └── (Healing: packages/healing with Claude)                    │
└─────────────────────────────────────────────────────────────────┘
```

## Package Responsibilities

| Package | Purpose |
|---------|---------|
| `packages/parser` | Parse CI logs, extract errors (TS, Go, Rust, Python, ESLint) |
| `packages/git` | Git operations, worktree management |
| `packages/healing` | Claude-based error healing loop |
| `packages/persistence` | Cache parsed errors, healing state |
| `packages/core` | Legacy Go shared logic |

## Release Flow

1. **Changesets**: Create changesets for changes
2. **PR Merge**: Changesets action creates release PR
3. **Release PR Merge**: Bumps versions, creates git tag
4. **Build**: Compiles binaries for all platforms (Bun compile)
5. **Sign**: Cosign signs checksums
6. **Distribute**:
   - GitHub Release with assets
   - Vercel Blob for `curl -fsSL https://detent.sh/install.sh | bash`

## Key Decisions

1. **TypeScript CLI**: Bun compile for standalone binaries
2. **TypeScript parser**: Replaced Go parser, runs locally in CLI
3. **Git worktrees**: Isolated healing without affecting working directory
4. **Act**: Local GitHub Actions runner for testing workflows
