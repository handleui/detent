# Changesets

This project uses [Changesets](https://github.com/changesets/changesets) for version management and changelog generation.

## Workflow

### 1. Making Changes

When you make changes that should be released, add a changeset:

```bash
bun run changeset
```

This will prompt you to:
- Select which packages are affected
- Choose the bump type (major, minor, patch)
- Write a summary of the changes

The changeset will be saved as a markdown file in `.changeset/`.

### 2. Creating a Release

When you push changes to `main` with changesets:

1. The **Changesets workflow** (`.github/workflows/changesets.yml`) automatically runs
2. It creates or updates a "Version Packages" PR that:
   - Bumps package versions according to changesets
   - Updates CHANGELOG.md files
   - Removes consumed changeset files

### 3. Publishing

When you merge the "Version Packages" PR:

1. The **Changesets workflow** detects no more changesets remain
2. It automatically creates git tags (e.g., `v1.0.0`) for the new version
3. The tags trigger the **Release workflow** (`.github/workflows/release.yml`)
4. The Release workflow:
   - Builds binaries for Linux, macOS, and Windows
   - Creates a GitHub Release with release notes
   - Publishes to npm with provenance

## Changeset Types

- **patch**: Bug fixes and minor updates (0.0.x)
- **minor**: New features, backwards compatible (0.x.0)
- **major**: Breaking changes (x.0.0)

## Commands

```bash
# Create a new changeset
bun run changeset

# Version packages (happens automatically in CI)
bun run version

# View changeset status
bun changeset status
```

## Configuration

Configuration is in `.changeset/config.json`:

- `access: "public"` - Packages are published publicly
- `baseBranch: "main"` - Release from main branch
- `commit: false` - Don't auto-commit (CI handles this)

## Commit Convention

This project uses conventional commits (header-only):

```
feat: add new command
fix: resolve parsing issue
chore: update dependencies
```

The Version Packages PR follows this convention with:
```
chore: version packages
```

---

For more information, see the [Changesets documentation](https://github.com/changesets/changesets)
