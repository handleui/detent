# @detent/cli

## 0.1.0

### Minor Changes

- a5bac3a: Initial release of the Detent CLI - TypeScript rewrite from scratch

  ### Breaking Changes from Previous Go Implementation

  - Complete rewrite in TypeScript - no longer Act-first architecture
  - New `mock` command replaces integrated check behavior
  - Configuration moved to `config` subcommand

  ### Commands

  - **`detent mock`**: Run CI workflows locally with full TUI, streaming logs, and error parsing
  - **`detent config`**: Manage repository-specific and global settings
  - **`detent init`**: Initialize Detent in a repository
  - **`detent update`**: Self-update the CLI binary
  - **`detent version`**: Display version information

  ### Features

  - **Ink-based TUI**: Rich terminal interface with progress indicators, job status, and error highlighting
  - **Multi-platform Support**: Pre-built binaries for macOS (Intel/ARM), Linux (x64/ARM), and Windows
  - **Smart Error Parsing**: Integrates with `@detent/parser` for TypeScript, ESLint, Go, Python, and Rust errors
  - **GitHub Actions Compatibility**: Parse and display GitHub Actions workflow commands
  - **Signal Handling**: Graceful shutdown and cleanup on SIGINT/SIGTERM

  ### Technical Details

  - Built with `citty` for CLI argument parsing
  - React 18 + Ink 5 for terminal UI components
  - Uses workspace packages: `@detent/git`, `@detent/parser`, `@detent/persistence`, `@detent/healing`
  - Vitest for unit testing

### Patch Changes

- Updated dependencies [a5bac3a]
- Updated dependencies [50d0ad0]
- Updated dependencies [a5bac3a]
- Updated dependencies [50d0ad0]
  - @detent/persistence@0.1.0
  - @detent/git@0.1.0
  - @detent/healing@0.1.0
  - @detent/parser@0.1.0
