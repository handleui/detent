# @detent/git

## 0.1.0

### Minor Changes

- 50d0ad0: Initial release of the Detent git utilities package

  ### Features

  - **Repository Detection**: Find git root and validate repository structure
  - **Branch Operations**: Safe branch creation, switching, and cleanup
  - **Diff Utilities**: Generate and parse git diffs for change tracking
  - **Status Helpers**: Check working tree status and staged changes

  ### Technical Details

  - Shell-out to git CLI for maximum compatibility
  - TypeScript interfaces for git operation results
  - Error handling for common git failure modes
  - Vitest test suite for core operations
