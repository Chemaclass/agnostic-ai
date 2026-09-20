# Release process

[Contributor docs](README.md)

## Versioning

Semantic Versioning. Pre-1.0: minor bumps may break spec format; patches are bug-fix only. Breaking changes go in `CHANGELOG.md`.

## Cut a release

Use the `cut-release` skill for the complete publication flow. It finalizes the
dated changelog, bumps the version, then creates exactly one
`docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` release briefing immediately
before the signed release commit and tag. The briefing reproduces the exact
release changelog and keeps verified upstream CLI and model news in a separate
section. Breaking behavior, changed defaults, removals, and deprecations come
before additions.

Run `make site-build site-test` before the release commit. Confirm the article,
archive, RSS GUID, `.html` compatibility alias, and sitemap route are generated.
The version, changelog, and briefing must share one commit and tag.

The mechanical release script remains available:

```bash
scripts/release.sh vX.Y.Z              # bump + tag + push
scripts/release.sh vX.Y.Z --dry-run    # preview only
scripts/release.sh vX.Y.Z --no-push    # commit + tag locally
```

The script does not author or validate the editorial briefing. Use the release
skill when publishing a normal release. The script:

1. Validates: clean tree, on `main`, in sync with `origin/main`, tag absent.
2. Runs `gofmt -s -l`, `go vet`, `go test ./...`, `agnostic-ai sync --check`.
3. Bumps `version` in `cmd/agnostic-ai/main.go`.
4. Drops empty `### ` subsections from `[Unreleased]`, then promotes it to `## vX.Y.Z - YYYY-MM-DD` (no brackets) and inserts a fresh empty `[Unreleased]` block.
5. Commits `chore(release): vX.Y.Z`, creates annotated tag `vX.Y.Z`.
6. Pushes `main` + tag. CI runs GoReleaser; release notes come from `scripts/release-notes.sh` against the matching `CHANGELOG.md` section.

After the push, watch both workflows:

- `Release` builds artifacts and publishes the GitHub Release from the tag.
- `Pages` deploys the Zola site and playground automatically from the release
  commit on `main`.

The Pages workflow also supports `workflow_dispatch`. Dispatch
`playground.yml` on `main` only when the automatic run is absent or needs a
safe retry, then watch the manual run to completion.

## Distribution

| Channel | Notes |
|---|---|
| GitHub Releases | raw binaries, primary |
| Homebrew tap | `chemaclass/tap/agnostic-ai`, cask auto-updated by CI (GitHub App, `HOMEBREW_TAP_TOKEN` until it exists) |
| `go install` | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| Install scripts | `scripts/install.sh`, `scripts/install.ps1`, served raw from `main`. No release step: they resolve the latest tag at runtime |
| Scoop | manifest pushed to `Chemaclass/scoop-bucket` (`SCOOP_BUCKET_TOKEN`) |
| winget | manifest branch in `Chemaclass/winget-pkgs`, PR opened against `microsoft/winget-pkgs` (`WINGET_TOKEN`) |
| npm | `agnostic-ai` package publishing the platform binaries (`NPM_TOKEN`) |

### One-time setup per channel

Every publisher is gated on its credential: with the secret absent, GoReleaser builds the manifest and skips the push, so a release never fails over missing setup. That is also why the `distribution` job exists. A skipped push and a successful one look the same in the GoReleaser log, so only that job proves the channel serves the new version (#920).

- **Homebrew**: the release mints a per-run installation token from a GitHub App. Register an App owned by `Chemaclass` with `contents: write`, install it on `Chemaclass/homebrew-tap` alone, and store `HOMEBREW_TAP_APP_ID` and `HOMEBREW_TAP_APP_PRIVATE_KEY`. The private key does not expire, so nothing is on a rotation timer. Until both secrets exist the release falls back to the `HOMEBREW_TAP_TOKEN` PAT, which is the credential that went stale unnoticed for ten releases (#943, #920). The same App can later cover the Scoop bucket and the winget fork, one installation per target repository.
- **Scoop**: create the public repo `Chemaclass/scoop-bucket` with a `main` branch, then add a `SCOOP_BUCKET_TOKEN` repo secret (PAT with `contents: write` on that repo). Users: `scoop bucket add chemaclass https://github.com/Chemaclass/scoop-bucket`.
- **winget**: fork `microsoft/winget-pkgs` to `Chemaclass/winget-pkgs`, then add `WINGET_TOKEN` (PAT with `contents: write` on the fork and `pull_requests: write` upstream). Microsoft reviews each PR, so a new version lands in `winget search` hours to days after the GitHub release.
- **npm**: `npm/` holds the wrapper package. Add `NPM_TOKEN` (automation token on the `agnostic-ai` package).

## Backporting

Cherry-pick fixes for the previous minor onto `release/X.Y`. Tag `vX.Y.Zpatch`.
