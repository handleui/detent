# @detent/cli

## 0.8.0

### Minor Changes

- 6ed486d: Enable GitHub releases for version visibility alongside blob distribution

## 0.7.0

### Minor Changes

- 2c1e2d8: Moved core functionality to external package :)

## 0.6.0

### Minor Changes

- 83db356: Improved check command to support depends flags on yaml files, comprehensive safelist for production releases (are skipped)

  - Check command properly creates a manifest of all jobs and steps
  - We properly track the progress of all jobs and steps
  - We inject bypasses for dependent jobs so we grep all errors
  - Sensitive runs are skipped (production deployments, version bumps, npm releases, docker publishes, etc) and properly disclosed on the TUI

## 0.5.0

### Minor Changes

- cec0217: Split tool parsing into dedicated per-language parsers, improve check UI, and add Sentry monitoring

  # Tool Parsing Improvements

  - Split monolithic error parser into dedicated per-tool parsers (Go, TypeScript, ESLint, Rust)
  - Add ESLint parser supporting stylish, compact, and unix output formats
  - Add Rust/Cargo parser with Clippy lint support and multi-line error handling
  - Implement parser registry with priority-based routing and confidence scoring
  - Remove premature tool patterns for unimplemented parsers (Python, Java, Ruby, etc.)

  # Check Command UI

  - Improve error display with structured output and better formatting
  - Add tool detection feedback showing which parsers are being used
  - Better progress indicators and status messages

  # Sentry Integration

  - Add crash reporting and error tracking via Sentry SDK
  - Track unsupported tool usage to prioritize parser development
  - PII scrubbing and filtering for sensitive data

  # Config & Schema

  - Add config migrations for schema version upgrades
  - Update JSON schema with new configuration options
  - Improve validation and error messages for invalid configs

  # Frankenstein Command

  - Add experimental `frankenstein` command for parallel Claude iterations
  - Support for testing AI-powered error fixing workflows

## 0.4.0

### Minor Changes

- 160be02: Add heal command infrastructure, frankenstein command, and config management

  - New `heal` command with Claude Agent SDK integration for AI-powered error fixing
  - New `frankenstein` command for parallel Claude iterations
  - New `config` command with interactive TUI for managing settings
  - Agent tools: edit_file, glob, grep, read_file, run_command with security controls
  - Heal loop with budget tracking and pricing calculations
  - Trust prompts, API key prompts, and command approval workflows
  - JSON config schema with IDE autocomplete (`https://detent.sh/detent.schema.json`)
  - Install script hosted at `https://detent.sh/install.sh`
  - Git worktree and commit improvements for heal operations

## 0.3.0

### Minor Changes

- 10a46be: Add smart caching and heal infrastructure

  - **Run caching**: Skip workflow execution when commit unchanged (use `--force` to bypass)
  - **File hash tracking**: Populate `file_hash` on error records for cache invalidation
  - **Heal caching**: Add `file_hash` column to heals table with composite index for efficient pending heal lookups
  - **Dry-run mode**: Add `--dry-run` flag to preview check command UI without execution

## 0.2.0

### Minor Changes

- 5045daa: Add preflight checks with git validation and interactive cleanup

  - Add git preflight validation to ensure clean working directory before operations
  - Introduce interactive worktree cleanup prompt allowing users to stash, commit, or clean changes
  - Harden hooks security with environment variable isolation
  - Drop dangerous Docker capabilities (SYS_ADMIN, NET_ADMIN, SYS_PTRACE, MKNOD)
  - Improve error parsing with enhanced location tracking and deduplication
  - Prepare persistence layer for remote SQL collaboration
  - Update branding with green color scheme

  UX improvements:

  - Add default commit message so pressing Enter immediately commits
  - Use subtle grey text for option hints instead of parentheses
  - Add commit message validation for control characters
  - Improve Docker check errors with specific diagnostics (not installed vs not running vs permissions)
  - Improve stash restoration messages with recovery instructions

## 0.1.0

### Minor Changes

- b4e1952: Major refactor for basics, added base parse commands, refactored architecture, imrpoved code quality and standardized command to 'dt'
