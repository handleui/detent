# @detent/cli

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
