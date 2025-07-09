# Release Guide for Meroku

This guide explains how to create a new release for the Meroku application using GoReleaser.

## Prerequisites

1. Install GoReleaser:
   ```bash
   # macOS
   brew install goreleaser

   # Linux
   curl -sfL https://goreleaser.com/static/run | bash -s -- check

   # Or download from https://github.com/goreleaser/goreleaser/releases
   ```

2. Ensure you have:
   - Go 1.23+ installed
   - Node.js 20+ and pnpm installed (for building the web app)
   - GitHub token with `repo` scope set as `GITHUB_TOKEN` environment variable
   - (Optional) `HOMEBREW_TAP_GITHUB_TOKEN` for Homebrew tap releases

## Release Process

### 1. Manual Release (Local)

1. **Build the web app first:**
   ```bash
   cd web
   pnpm install
   pnpm build
   cd ..
   ```

2. **Test the build locally:**
   ```bash
   cd app
   goreleaser build --snapshot --clean
   ```

3. **Create a new tag:**
   ```bash
   # Update version as needed (e.g., v1.0.0, v1.0.1, etc.)
   git tag -a v1.0.0 -m "Release version 1.0.0"
   git push origin v1.0.0
   ```

4. **Create the release:**
   ```bash
   # Ensure GITHUB_TOKEN is set
   export GITHUB_TOKEN="your-github-token"
   
   # Optional: For Homebrew tap
   export HOMEBREW_TAP_GITHUB_TOKEN="your-homebrew-tap-token"
   
   # Run GoReleaser from the app directory
   cd app
   goreleaser release --clean
   ```

### 2. Automated Release (GitHub Actions)

Simply push a tag to trigger the automated release:

```bash
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0
```

The GitHub Actions workflow will automatically:
- Build the web application
- Create binaries for multiple platforms
- Create a GitHub release with the artifacts
- Update the Homebrew tap (if token is configured)

## Using Make Commands

From the `app` directory:

```bash
# Check if ready for release
make release-check

# Create a snapshot release (local only)
make release-snapshot

# Create a full release (requires tag)
make release
```

## Version Numbering

Follow semantic versioning (SemVer):
- `MAJOR.MINOR.PATCH` (e.g., `1.2.3`)
- Increment MAJOR for breaking changes
- Increment MINOR for new features
- Increment PATCH for bug fixes

## Release Artifacts

GoReleaser will create:
- Binary archives for:
  - macOS (Intel and Apple Silicon)
  - Linux (x86_64 and ARM64)
  - Windows (x86_64)
- Checksums file
- Changelog based on commit messages
- Homebrew formula in the MadAppGang/homebrew-meroku tap

## Homebrew Installation

After release, users can install via Homebrew:

```bash
brew tap MadAppGang/meroku
brew install meroku
```

## Commit Message Convention

For better changelogs, use conventional commits:
- `feat:` for new features
- `fix:` for bug fixes
- `perf:` for performance improvements
- `docs:` for documentation (excluded from changelog)
- `chore:` for maintenance (excluded from changelog)
- `test:` for tests (excluded from changelog)

Example:
```
feat: add support for AWS Lambda deployments
fix: resolve connection timeout in SSH terminal
perf: optimize web app bundle size
```

## Testing a Release Locally

Before creating an actual release:

```bash
# From the app directory
cd app

# Create a snapshot release (doesn't push to GitHub)
goreleaser release --snapshot --clean

# Check the dist/ directory for artifacts
ls -la dist/
```

## Troubleshooting

1. **Build fails**: Ensure all dependencies are installed:
   ```bash
   cd web && pnpm install
   cd ../app && go mod download
   ```

2. **Permission denied**: Ensure your GitHub token has the required permissions

3. **Tag already exists**: Delete the tag and try again:
   ```bash
   git tag -d v1.0.0
   git push origin :refs/tags/v1.0.0
   ```

4. **Homebrew tap fails**: Ensure `HOMEBREW_TAP_GITHUB_TOKEN` has write access to the tap repository

## Post-Release

After a successful release:
1. Verify the release on GitHub releases page
2. Test the Homebrew installation
3. Update any documentation with the new version
4. Announce the release (if needed)