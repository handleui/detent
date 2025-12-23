# GitHub Actions Workflows

This directory contains the CI/CD workflows for the Detent CLI project.

## Workflows

### CI Workflow (`ci.yml`)

**Triggers:**
- Push to `main` branch
- Pull requests targeting `main` branch

**Jobs:**

1. **Lint** - Runs `golangci-lint` with the project's configuration
   - Uses Go 1.24.0
   - Caches Go modules for faster builds
   - Timeout: 5 minutes

2. **Test** - Runs the test suite
   - Executes tests with race detection enabled
   - Generates coverage reports
   - Optionally uploads coverage to Codecov (requires `CODECOV_TOKEN` secret)

3. **Build** - Builds the binary
   - Uses Bun to run the build script (which injects version from `package.json`)
   - Verifies the binary runs successfully
   - Uploads the binary as an artifact (retained for 7 days)

**Concurrency:** Workflows for the same ref are cancelled when a new run starts to save resources.

### Changesets Workflow (`changesets.yml`)

**Triggers:**
- Push to `main` branch

**Process:**

1. **Version Management:**
   - When changesets are present, creates or updates a "Version Packages" PR
   - The PR bumps package versions, updates CHANGELOGs, and removes consumed changesets
   - Uses conventional commit format: `chore: version packages`

2. **Tag Creation:**
   - When the "Version Packages" PR is merged (and no changesets remain)
   - Automatically creates git tags for new versions (e.g., `v1.0.0`)
   - Only tags `@detent/cli` package (the main publishable package)
   - Checks for existing tags to prevent duplicates

3. **Release Trigger:**
   - Tags automatically trigger the Release workflow
   - Integrates seamlessly with existing release process

**Permissions:** Requires `contents: write` and `pull-requests: write`.

### Release Workflow (`release.yml`)

**Triggers:**
- Tag push matching `v*` pattern (e.g., `v1.0.0`, `v2.1.3-beta`)

**Process:**

1. **Build Matrix** - Builds binaries for multiple platforms in parallel:
   - Linux: amd64, arm64
   - macOS: amd64 (Intel), arm64 (Apple Silicon)
   - Windows: amd64

2. **Build Steps:**
   - Extracts version from the Git tag (removes `v` prefix)
   - Injects version into binary using `-ldflags`
   - Uses `CGO_ENABLED=0` for static binaries
   - Strips debug symbols with `-s -w` for smaller binaries
   - Creates compressed archives (`.tar.gz` for Unix, `.zip` for Windows)
   - Generates SHA256 checksums for verification

3. **Release Creation:**
   - Downloads all build artifacts
   - Generates release notes from Git commit history
   - Creates a GitHub release with:
     - Installation instructions for each platform
     - List of changes since the previous release
     - All binaries and checksums
   - Marks as prerelease if tag contains `alpha`, `beta`, or `rc`

**Permissions:** Requires `contents: write` to create releases.

## Creating a Release

### Using Changesets (Recommended)

The project uses Changesets for automated version management:

1. **Add a changeset** when you make changes:
   ```bash
   bun run changeset
   ```
   - Select affected packages
   - Choose bump type (major/minor/patch)
   - Write a description

2. **Push to main** with your changeset:
   ```bash
   git add .
   git commit -m "feat: add new feature"
   git push origin main
   ```

3. **Review the Version Packages PR:**
   - Changesets workflow creates/updates a "Version Packages" PR
   - Review the version bumps and CHANGELOG updates

4. **Merge the PR:**
   - When merged, tags are created automatically
   - Release workflow triggers automatically
   - Binaries are built and published

### Manual Release (Not Recommended)

For manual releases (only if needed):

1. Update the version in `apps/cli/package.json`
2. Commit the change:
   ```bash
   git add apps/cli/package.json
   git commit -m "chore: bump version to 1.0.0"
   ```
3. Create and push a tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

The release workflow will automatically:
- Build binaries for all platforms
- Create a GitHub release
- Upload all artifacts with checksums
- Publish to npm with provenance

## Local Testing

Before pushing, you can test the build locally:

```bash
# From the repository root
cd apps/cli

# Test the build
bun run build
./dist/dt --version

# Test cross-compilation
GOOS=darwin GOARCH=arm64 go build -o dist/dt-darwin-arm64
GOOS=windows GOARCH=amd64 go build -o dist/dt-windows-amd64.exe
```

## Secrets

The workflows use the following secrets:

- `GITHUB_TOKEN` - Automatically provided by GitHub Actions (no setup required)
- `CODECOV_TOKEN` - (Optional) For uploading test coverage to Codecov

To add the Codecov token:
1. Go to repository Settings → Secrets and variables → Actions
2. Add a new secret named `CODECOV_TOKEN`
3. The CI workflow will automatically use it when pushing to `main`

## Troubleshooting

### Build Failures

If the build fails:
1. Check that `apps/cli/go.mod` specifies the correct Go version
2. Ensure all dependencies are properly vendored
3. Test locally with `bun run build`

### Release Failures

If the release fails:
1. Verify the tag follows the `v*` pattern
2. Check that the version in `package.json` matches the tag
3. Ensure `cmd.Version` variable exists in the cmd package
4. Verify `GITHUB_TOKEN` has sufficient permissions

### Lint Failures

If linting fails:
1. Run locally: `cd apps/cli && golangci-lint run ./...`
2. Fix issues with: `golangci-lint run --fix ./...`
3. Check `.golangci.yml` configuration

## Best Practices

1. **Always test locally before pushing**
   - Run `bun run build` to verify the build works
   - Run tests with `go test ./...`
   - Run linter with `golangci-lint run ./...`

2. **Use semantic versioning** for tags
   - MAJOR: Breaking changes (v2.0.0)
   - MINOR: New features (v1.1.0)
   - PATCH: Bug fixes (v1.0.1)
   - Prerelease: Use suffixes (v1.0.0-beta.1)

3. **Review the generated release notes**
   - Edit them after creation if needed
   - Add migration guides for breaking changes
   - Highlight important features

4. **Keep workflows up to date**
   - Update Go version when upgrading
   - Update action versions regularly
   - Monitor for security advisories

## Performance Optimizations

The workflows include several optimizations:

- **Caching**: Go modules are cached to speed up builds
- **Concurrency**: CI runs are cancelled when new commits are pushed
- **Parallelization**: Release builds run in parallel using matrix strategy
- **Artifact retention**: Build artifacts are kept for 7 days only
- **Binary optimization**: Release binaries are stripped of debug symbols

## Monitoring

Monitor workflow runs at:
- **Actions tab**: https://github.com/[owner]/[repo]/actions
- **Release page**: https://github.com/[owner]/[repo]/releases

Set up notifications in your GitHub settings to get alerts for:
- Failed workflow runs
- Successful releases
- Security alerts
