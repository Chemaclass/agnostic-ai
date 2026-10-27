# Release process

[Contributor docs](README.md)

To publish a version, follow [Cut a release](#cut-a-release). For changes to an installation channel, use [Distribution](#distribution) to find its scripts and required setup.

## Versioning

Use Semantic Versioning (`major.minor.patch`). Before 1.0, minor releases may break the spec format; patch releases contain bug fixes only. Breaking changes go in `CHANGELOG.md`.

## Cut a release

Follow the `cut-release` skill's [release guide](../../.agnostic-ai/skills/cut-release/SKILL.md). It prepares the dated changelog, updates the version, and creates `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` immediately before the signed release commit and tag. The announcement summarizes the release and links to the GitHub release for the complete record. Keep verified upstream CLI and model news separate.

Run `make site-build site-test` before the release commit. Confirm the article,
updates archive, RSS entry identifier (GUID), old `.html` address, and sitemap URL are generated.
The version, changelog, and announcement must share one commit. Push that commit,
wait for its complete continuous integration (CI) run on Linux, macOS, and Windows, then sign and push the tag.

The tag workflow uses `scripts/release-notes.sh` to build GitHub Release notes from the dated `CHANGELOG.md` section.

After pushing the tag, watch both workflows:

- `Release` builds release files and publishes the GitHub Release from the tag.
- `Pages` deploys the Zola site and playground automatically from the release
  commit on `main`.

The Pages workflow also supports `workflow_dispatch`. Dispatch
`playground.yml` on `main` only when the automatic run is absent or needs a
safe retry, then watch the manual run to completion.

## Distribution

| Channel | Notes |
|---|---|
| GitHub Releases | compiled programs, main download source |
| Homebrew tap | `chemaclass/tap/agnostic-ai`, cask updated by the tap itself on a half-hourly schedule, no credential here |
| `go install` | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| Install scripts | `scripts/install.sh`, `scripts/install.ps1`, served raw from `main`. No release step: they resolve the latest tag at runtime |
| Scoop | manifest pushed to `Chemaclass/scoop-bucket` (`SCOOP_BUCKET_TOKEN`) |
| winget | manifest branch in `Chemaclass/winget-pkgs`, PR opened against `microsoft/winget-pkgs` (`WINGET_TOKEN`) |
| npm | seven packages: `agnostic-ai` plus one `@agnostic-ai/<os>-<cpu>` per platform, all at the tag version (`NPM_TOKEN`). The scoped packages belong to the project's npm organization |

### One-time setup per channel

Each publishing channel requires its own credential. Without the secret, GoReleaser builds the manifest and skips the push; missing setup does not fail a release. Its log does not distinguish a skipped push from a successful one. The `distribution` job checks that the channel serves the new version (#920).

- **Homebrew**: `Chemaclass/homebrew-tap` updates its own cask, so this repository needs no secret for it. Its [Update agnostic-ai cask workflow](https://github.com/Chemaclass/homebrew-tap/blob/main/.github/workflows/update-agnostic-ai-cask.yml) is scheduled at minutes 23 and 53 each hour, resolves the latest release, rewrites `Casks/agnostic-ai.rb` from `checksums.txt`, checks all four archives return 200, and commits with its existing `GITHUB_TOKEN`. Cross-repository pushes need a credential. GitHub has no API to mint one: creating a GitHub App or personal access token (PAT) requires a browser. Missing credentials left ten releases with a stale cask (#943, #920). The Homebrew cask can therefore lag a published release until the next successful workflow run; dispatch that workflow to update it sooner. For optional tag-time pushes, the release job supports `HOMEBREW_TAP_APP_ID`/`HOMEBREW_TAP_APP_PRIVATE_KEY` or `HOMEBREW_TAP_TOKEN`; both routes produce byte-identical files. Scoop and winget still have no route and stay skipped.
- **Scoop**: create the public repo `Chemaclass/scoop-bucket` with a `main` branch, then add a `SCOOP_BUCKET_TOKEN` repo secret (PAT with `contents: write` on that repo). Users: `scoop bucket add chemaclass https://github.com/Chemaclass/scoop-bucket`.
- **winget**: fork `microsoft/winget-pkgs` to `Chemaclass/winget-pkgs`, then add `WINGET_TOKEN` (PAT with `contents: write` on the fork and `pull_requests: write` upstream). Microsoft reviews each PR, so a new version lands in `winget search` hours to days after the GitHub release.
- **npm**: `npm/` holds the parent package. Add `NPM_TOKEN` (automation token). The token has to be able to publish the `agnostic-ai` package *and* create packages under the `agnostic-ai` npm organization, which must exist before the first release that ships platform packages.

### npm publish order

The parent requires exact versions of six platform packages, so the release publishes them first and the parent last:

1. `scripts/npm-binaries.sh <tag> <dir>` downloads the six release archives, verifies them against `checksums.txt`, and unpacks one program per platform.
2. `npm/scripts/build-platform-packages.js --binaries <dir> --version <x.y.z>` writes `npm/platforms/<os>-<cpu>/` and pins the parent to all six.
3. `scripts/npm-publish.sh <x.y.z>` publishes the six platform packages with required provenance and waits until npm serves them. When this run uploaded any of them, it waits five more minutes (`NPM_ATTESTATION_SETTLE`) before verifying those uploads. The parent follows the same steps. A version an earlier run published is verified straight away. Each check retries with backoff (`NPM_PROVENANCE_RETRIES`, default 10) so a short registry outage does not fail a good version.

   The wait before the first check matters. npm caches each signed record response for a year, and npm adds its own signed record about a minute after it serves the version. A check that reads the record too early stores an incomplete copy, and later checks that hit the same cache keep failing with "has attestations but no corresponding public key(s) can be found".

The `distribution` job checks all seven afterwards. Keep this order: publishing the parent before a required platform package breaks `npm install` on that platform until the next release. npm scans each publish before serving it, commonly taking about five minutes and sometimes more than 15. Publication and verification therefore wait about 20 minutes, increasing the delay between attempts up to a fixed maximum.

### Required npm provenance and recovery

Every new publish uses `--provenance`, which attaches signed records of the build and publication. If publication fails with those records enabled, it never retries without them. An existing version counts as complete only when its signed records pass verification. The publisher and distribution job check all seven packages without installing or executing them.

`scripts/npm-verify-provenance.js` uses the pinned npm CLI's bundled verification libraries and Sigstore trust root. It verifies the registry signature, build and registry publish attestations, and downloaded tarball integrity. It requires the build certificate to name this repository's `.github/workflows/release.yml` at the version tag, issued by GitHub Actions, and the signed source commit to match the release commit. A registry signature alone does not prove where a package was built.

Re-run the same release after a temporary registry or signing-service failure when the published packages have valid attestations. Verified platform packages are skipped and missing packages publish before the parent. If a tarball reached npm without its required attestation, stop: npm versions are immutable and re-running cannot attach a replacement. Prepare a new version of all seven packages through the normal release process. Do not plain-publish, unpublish, or move the old tag to bypass verification.

See [npm provenance verification](https://docs.npmjs.com/viewing-package-provenance/) and the [npm verifier implementation](https://github.com/npm/cli/blob/v11.5.1/lib/utils/verify-signatures.js).

### npm dist-tags

`npm publish` with no `--tag` writes `latest`, and `latest` is what `npm install agnostic-ai` selects when no version is specified. The release workflow fires on every `v*` tag, including prereleases, so an untagged prerelease publish would replace the stable release for everyone.

`npm_dist_tag` in `scripts/npm-publish.sh` derives the tag from the version, and every publish with required build-origin verification (provenance) uses it:

| Version | dist-tag |
|---|---|
| `0.64.0` | `latest` |
| `0.64.0-beta.1` | `beta` |
| `0.64.0-rc.2` | `rc` |
| `0.64.0-alpha.0` | `alpha` |
| `0.64.0-20260101`, anything else | `next` |

The `distribution` job sources the same helper and asserts, per package, that the version exists *and* that the dist-tag resolves to it. Install a prerelease with `npm install -g agnostic-ai@beta`.

`scripts/npm-smoke.sh` runs the same generator against programs built locally for each platform and installs the result from tarballs. Use it to check a change to any of the three scripts without cutting a release.

## Backporting

Cherry-pick fixes for the previous minor onto `release/X.Y`. Tag `vX.Y.Zpatch`.
