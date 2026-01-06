# @detent/persistence

## 0.1.0

### Minor Changes

- a5bac3a: Initial release of the Detent persistence layer

  ### Features

  - **SQLite-based Storage**: Fast, embedded database for local state management
  - **Run Tracking**: Store and query CI run history, status, and metadata
  - **Assignment Management**: Track error-to-fix assignments across runs
  - **Fix History**: Record applied fixes with success/failure status
  - **Per-Repository Configuration**: Store settings scoped to individual repos

  ### Technical Details

  - Uses `better-sqlite3` for synchronous, high-performance SQLite access
  - Schema migrations for safe upgrades
  - Type-safe query interfaces with TypeScript
  - Designed for CLI and local development workflows
