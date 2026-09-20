# Release process

[Contributor docs](README.md)

## Versioning

Semantic Versioning. Pre-1.0: minor bumps may break spec format; patches are bug-fix only. Breaking changes go in `CHANGELOG.md`.

## Cut a release

Use the `cut-release` skill for the complete publication flow. It finalizes the
dated changelog, bumps the version, then creates exactly one
`docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` release announcement immediately
before the signed release commit and tag. It is a concise reader-facing summary,
not a duplicate changelog. Link readers to the GitHub release for the complete
record and keep verified upstream CLI and model news separate.

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
| Homebrew tap | `chemaclass/tap/agnostic-ai`, cask updated by the tap itself on a half-hourly schedule, no credential here |
| `go install` | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| Install scripts | `scripts/install.sh`, `scripts/install.ps1`, served raw from `main`. No release step: they resolve the latest tag at runtime |
| Scoop | manifest pushed to `Chemaclass/scoop-bucket` (`SCOOP_BUCKET_TOKEN`) |
| winget | manifest branch in `Chemaclass/winget-pkgs`, PR opened against `microsoft/winget-pkgs` (`WINGET_TOKEN`) |
| npm | `agnostic-ai` package publishing the platform binaries (`NPM_TOKEN`) |

### One-time setup per channel

Every publisher is gated on its credential: with the secret absent, GoReleaser builds the manifest and skips the push, so a release never fails over missing setup. That is also why the `distribution` job exists. A skipped push and a successful one look the same in the GoReleaser log, so only that job proves the channel serves the new version (#920).

- **Homebrew**: the cask is not pushed from this repository, and needs no secret here. `Chemaclass/homebrew-tap` runs its own `Update agnostic-ai cask` workflow every half hour: it resolves the latest release, rewrites `Casks/agnostic-ai.rb` from `checksums.txt`, checks all four archives return 200, and commits with the `GITHUB_TOKEN` that workflow already has. Writing to another repository is what needs a credential, and GitHub has no API to mint one, since creating a GitHub App and creating a PAT are both browser flows. That is why ten releases went out with a stale cask (#943, #920). A release can therefore lag brew by up to half an hour; dispatch that workflow to pull it forward. The release job still supports `HOMEBREW_TAP_APP_ID`/`HOMEBREW_TAP_APP_PRIVATE_KEY` or `HOMEBREW_TAP_TOKEN` if you ever want the push to happen at tag time, and the generated file is byte-identical either way, so the two never fight. Scoop and winget still have no route and stay skipped.
- **Scoop**: create the public repo `Chemaclass/scoop-bucket` with a `main` branch, then add a `SCOOP_BUCKET_TOKEN` repo secret (PAT with `contents: write` on that repo). Users: `scoop bucket add chemaclass https://github.com/Chemaclass/scoop-bucket`.
- **winget**: fork `microsoft/winget-pkgs` to `Chemaclass/winget-pkgs`, then add `WINGET_TOKEN` (PAT with `contents: write` on the fork and `pull_requests: write` upstream). Microsoft reviews each PR, so a new version lands in `winget search` hours to days after the GitHub release.
- **npm**: `npm/` holds the wrapper package. Add `NPM_TOKEN` (automation token on the `agnostic-ai` package).

## Backporting

Cherry-pick fixes for the previous minor onto `release/X.Y`. Tag `vX.Y.Zpatch`.
