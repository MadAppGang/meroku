# Release playbook

How a meroku release is cut. Facts that live elsewhere are pointed at, not copied.

## Authority

- Version: `version.txt` at the repo root (`v<major>.<minor>.<patch>`). `meroku`
  copies it into a project's `infrastructure/version.txt`. The binary gets its
  version from the tag through `-ldflags "-X main.version=..."` in
  `.github/workflows/release.yml`.
- Changelog: `CHANGELOG.md`, newest first, one `## vX.Y.Z` section per release.
- Project command: `.claude/commands/release.md` (bump `version.txt`, commit, tag, push).

## Who authorises

Jack. A release runs end to end when he asks for one.

## Stages

none: one stage. Pushing a `v*` tag is the release.

## Order

1. `go vet ./...` and `go test ./...` in `app/` pass, and `gofmt -l .` prints nothing.
2. One commit bumps `version.txt` and adds the `CHANGELOG.md` section.
3. It reaches `main`: through a PR when the work came from a branch, merged once
   `ci.yml` is green on it. Recent v5.x releases were pushed straight to `main`.
4. Annotated tag `vX.Y.Z` on the commit on `main`, pushed as an explicit ref.
5. `release.yml` (triggered by the tag) builds the web app and the binaries,
   creates the GitHub Release and updates `MadAppGang/homebrew-tap`.

## Versioning

Conventional commits since the last tag: `fix` → patch, `feat` → minor, `feat!` → major.

## Verification

- `git ls-remote --tags origin refs/tags/vX.Y.Z` points at the merge commit.
- The `Release` workflow run for the tag concluded `success` in every job,
  including `Update Homebrew Tap`.
- `gh release view vX.Y.Z` shows the release with its archives and checksums.
- `Formula/meroku.rb` in `MadAppGang/homebrew-tap` names `vX.Y.Z`.

## Deploy monitoring

none

## Unknown

- Whether every release goes through a PR or straight to `main` is not settled
  (v4.x used PRs, v5.x did not). TODO: Jack to say which.
- `CHANGELOG.md` has no sections for v5.0.0–v5.3.5. TODO: whether to backfill.

verified: unverified
