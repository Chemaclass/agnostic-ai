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
| Homebrew tap | `chemaclass/tap/agnostic-ai`, cask auto-updated by CI (`HOMEBREW_TAP_TOKEN`) |
| `go install` | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| Install scripts | `scripts/install.sh`, `scripts/install.ps1`, served raw from `main`. No release step: they resolve the latest tag at runtime |
| Scoop | manifest pushed to `Chemaclass/scoop-bucket` (`SCOOP_BUCKET_TOKEN`) |
| winget | manifest branch in `Chemaclass/winget-pkgs`, PR opened against `microsoft/winget-pkgs` (`WINGET_TOKEN`) |
| npm | seven packages: `agnostic-ai` plus one `@agnostic-ai/<os>-<cpu>` per platform, all at the tag version (`NPM_TOKEN`) |

### One-time setup per channel

Both Windows publishers are gated on their token: with the secret absent, GoReleaser builds the manifest and skips the push, so a release never fails over missing setup.

- **Scoop**: create the public repo `Chemaclass/scoop-bucket` with a `main` branch, then add a `SCOOP_BUCKET_TOKEN` repo secret (PAT with `contents: write` on that repo). Users: `scoop bucket add chemaclass https://github.com/Chemaclass/scoop-bucket`.
- **winget**: fork `microsoft/winget-pkgs` to `Chemaclass/winget-pkgs`, then add `WINGET_TOKEN` (PAT with `contents: write` on the fork and `pull_requests: write` upstream). Microsoft reviews each PR, so a new version lands in `winget search` hours to days after the GitHub release.
- **npm**: `npm/` holds the parent package. Add `NPM_TOKEN` (automation token). The token has to be able to publish the `agnostic-ai` package *and* create packages under the `agnostic-ai` npm organization, which must exist before the first release that ships platform packages.

### npm publish order

The parent pins exact versions of six platform packages, so the release publishes them first and the parent last:

1. `scripts/npm-binaries.sh <tag> <dir>` downloads the six release archives, verifies them against `checksums.txt`, and unpacks one binary per target.
2. `npm/scripts/build-platform-packages.js --binaries <dir> --version <x.y.z>` writes `npm/platforms/<os>-<cpu>/` and pins the parent to all six.
3. `scripts/npm-publish.sh <x.y.z>` publishes the six, waits until the registry serves every one, then publishes the parent.

The `distribution` job checks all seven afterwards. A parent on the registry whose platform package is missing breaks `npm install` on that platform until the next release, so nothing in this sequence is safe to reorder.

### npm dist-tags

`npm publish` with no `--tag` writes `latest`, and `latest` is what an unpinned `npm install agnostic-ai` resolves. The release workflow fires on every `v*` tag and `scripts/release.sh` accepts a prerelease, so an untagged prerelease publish would replace the stable release for everyone.

`npm_dist_tag` in `scripts/npm-publish.sh` derives the tag from the version, and both the provenance publish and the plain retry pass it:

| Version | dist-tag |
|---|---|
| `0.64.0` | `latest` |
| `0.64.0-beta.1` | `beta` |
| `0.64.0-rc.2` | `rc` |
| `0.64.0-alpha.0` | `alpha` |
| `0.64.0-20260101`, anything else | `next` |

The `distribution` job sources the same helper and asserts, per package, that the version exists *and* that the dist-tag resolves to it. Install a prerelease with `npm install -g agnostic-ai@beta`.

`scripts/npm-smoke.sh` runs the same generator against locally cross-compiled binaries and installs the result from tarballs. Use it to check a change to any of the three scripts without cutting a release.

## Backporting

Cherry-pick fixes for the previous minor onto `release/X.Y`. Tag `vX.Y.Zpatch`.
