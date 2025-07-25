# Package Manager Distribution Setup

This document explains how to set up and configure the various package managers for distributing Meroku.

## Overview

Meroku is distributed through the following package managers:
- **macOS**: Homebrew (Cask)
- **Windows**: Chocolatey and Scoop
- **Linux**: APT (Debian/Ubuntu) and YUM/DNF (RedHat/Fedora) via .deb and .rpm packages

## Prerequisites

### Repository Setup

You need to create the following repositories:
1. **Scoop Bucket**: `github.com/MadAppGang/scoop-meroku`
2. **Homebrew Tap**: `github.com/MadAppGang/homebrew-meroku` (already exists)

### GitHub Secrets

Add the following secrets to your GitHub repository:
- `HOMEBREW_TAP_GITHUB_TOKEN`: Personal access token with `repo` scope for the Homebrew tap
- `SCOOP_TAP_GITHUB_TOKEN`: Personal access token with `repo` scope for the Scoop bucket
- `CHOCOLATEY_API_KEY`: API key from your Chocolatey account

## Installation Instructions for Users

### macOS (Homebrew)
```bash
brew tap MadAppGang/meroku
brew install --cask meroku
```

### Windows (Chocolatey)
```powershell
choco install meroku
```

### Windows (Scoop)
```powershell
scoop bucket add meroku https://github.com/MadAppGang/scoop-meroku
scoop install meroku
```

### Linux (Debian/Ubuntu)
```bash
# Download the .deb file from releases
wget https://github.com/MadAppGang/meroku/releases/latest/download/meroku_Linux_x86_64.deb
sudo dpkg -i meroku_Linux_x86_64.deb
```

### Linux (RedHat/Fedora)
```bash
# Download the .rpm file from releases
wget https://github.com/MadAppGang/meroku/releases/latest/download/meroku_Linux_x86_64.rpm
sudo rpm -i meroku_Linux_x86_64.rpm
```

## Publishing Process

The publishing process is automated through GitHub Actions when you create a new release tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This will trigger the release workflow which:
1. Builds binaries for all platforms
2. Creates GitHub release with binaries
3. Publishes to Homebrew tap (macOS)
4. Publishes to Chocolatey (Windows)
5. Publishes to Scoop bucket (Windows)
6. Creates .deb and .rpm packages (Linux)

## Setting up Chocolatey

1. Create a Chocolatey account at https://chocolatey.org/
2. Get your API key from https://chocolatey.org/account
3. Add the API key as `CHOCOLATEY_API_KEY` secret in GitHub

## Setting up Scoop Bucket

1. Create a new repository: `scoop-meroku`
2. Initialize it with a basic bucket structure:
   ```json
   {
     "version": 1,
     "name": "Meroku Scoop Bucket",
     "description": "Scoop bucket for Meroku"
   }
   ```
3. Create a personal access token with `repo` scope
4. Add the token as `SCOOP_TAP_GITHUB_TOKEN` secret in GitHub

## Troubleshooting

### Chocolatey Publishing Failed
- Verify your API key is correct
- Check if the package name is already taken
- Review Chocolatey moderation guidelines

### Scoop Publishing Failed
- Ensure the bucket repository exists
- Verify the GitHub token has correct permissions
- Check if the manifest JSON is valid

### Linux Package Installation Issues
- For .deb: Use `sudo apt --fix-broken install` if dependencies are missing
- For .rpm: Use `sudo dnf install` instead of `rpm -i` for automatic dependency resolution