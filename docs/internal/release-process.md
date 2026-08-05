# Release process

## Versioning

Semantic Versioning. Pre-1.0: minor bumps may break spec format; patches are bug-fix only. Breaking changes go in `CHANGELOG.md`.

## Cut a release

```bash
scripts/release.sh vX.Y.Z              # bump + tag + push
scripts/release.sh vX.Y.Z --dry-run    # preview only
scripts/release.sh vX.Y.Z --no-push    # commit + tag locally
```

The script:

1. Validates: clean tree, on `main`, in sync with `origin/main`, tag absent.
2. Runs `gofmt -s -l`, `go vet`, `go test ./...`, `agnostic-ai sync --check`.
3. Bumps `version` in `cmd/agnostic-ai/main.go`.
4. Drops empty `### ` subsections from `[Unreleased]`, then promotes it to `## vX.Y.Z - YYYY-MM-DD` (no brackets) and inserts a fresh empty `[Unreleased]` block.
5. Commits `chore(release): vX.Y.Z`, creates annotated tag `vX.Y.Z`.
6. Pushes `main` + tag. CI runs GoReleaser; release notes come from `scripts/release-notes.sh` against the matching `CHANGELOG.md` section.

## Distribution

| Channel | Notes |
|---|---|
| GitHub Releases | raw binaries, primary |
| Homebrew tap | `chemaclass/tap/agnostic-ai`, cask auto-updated by CI (`HOMEBREW_TAP_TOKEN`) |
| `go install` | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| Install scripts | `scripts/install.sh`, `scripts/install.ps1`, served raw from `main`. No release step: they resolve the latest tag at runtime |
| Scoop | manifest pushed to `Chemaclass/scoop-bucket` (`SCOOP_BUCKET_TOKEN`) |
| winget | manifest branch in `Chemaclass/winget-pkgs`, PR opened against `microsoft/winget-pkgs` (`WINGET_TOKEN`) |
| npm | `agnostic-ai` package publishing the platform binaries (`NPM_TOKEN`) |

### One-time setup per channel

Both Windows publishers are gated on their token: with the secret absent, GoReleaser builds the manifest and skips the push, so a release never fails over missing setup.

- **Scoop**: create the public repo `Chemaclass/scoop-bucket` with a `main` branch, then add a `SCOOP_BUCKET_TOKEN` repo secret (PAT with `contents: write` on that repo). Users: `scoop bucket add chemaclass https://github.com/Chemaclass/scoop-bucket`.
- **winget**: fork `microsoft/winget-pkgs` to `Chemaclass/winget-pkgs`, then add `WINGET_TOKEN` (PAT with `contents: write` on the fork and `pull_requests: write` upstream). Microsoft reviews each PR, so a new version lands in `winget search` hours to days after the GitHub release.
- **npm**: `npm/` holds the wrapper package. Add `NPM_TOKEN` (automation token on the `agnostic-ai` package).

## Backporting

Cherry-pick fixes for the previous minor onto `release/X.Y`. Tag `vX.Y.Zpatch`.
