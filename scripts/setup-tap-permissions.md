# Setting Up GoReleaser to Push to Homebrew Tap

## Option 1: Personal Access Token (Recommended)

1. Create a GitHub Personal Access Token:
   - Go to https://github.com/settings/tokens/new
   - Give it a name like "GoReleaser Homebrew Tap"
   - Select scopes:
     - `repo` (full control of private repositories)
     - `workflow` (if using GitHub Actions)
   - Click "Generate token"

2. Add the token to your repository secrets:
   - Go to https://github.com/MadAppGang/infrastructure/settings/secrets/actions
   - Click "New repository secret"
   - Name: `HOMEBREW_TAP_GITHUB_TOKEN`
   - Value: Your personal access token

3. Update your GitHub Actions workflow to use this token:
   ```yaml
   - name: Run GoReleaser
     uses: goreleaser/goreleaser-action@v5
     with:
       version: latest
       args: release --clean
     env:
       GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
   ```

## Option 2: Deploy Key (Alternative)

1. Generate an SSH key:
   ```bash
   ssh-keygen -t ed25519 -C "goreleaser-homebrew-tap" -f homebrew_tap_key
   ```

2. Add the public key to homebrew-meroku as a deploy key:
   - Go to https://github.com/MadAppGang/homebrew-meroku/settings/keys
   - Click "Add deploy key"
   - Title: "GoReleaser Bot"
   - Key: Contents of homebrew_tap_key.pub
   - Check "Allow write access"

3. Add the private key to infrastructure repo secrets:
   - Name: `HOMEBREW_TAP_PRIVATE_KEY`
   - Value: Contents of homebrew_tap_key

## Option 3: GitHub App (Most Secure)

1. Create a GitHub App for your organization
2. Install it on both repositories
3. Use the app's credentials in GitHub Actions

## Current Workaround

Until you set up proper permissions, you can:

1. Run GoReleaser locally (it will use your personal GitHub auth)
2. Use the `scripts/push-to-tap.sh` script after CI builds
3. Manually update the tap after each release