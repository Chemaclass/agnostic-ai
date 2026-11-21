# Release process

[Contributor docs](README.md)

To publish a version, follow [Cut a release](#cut-a-release). For changes to an installation channel, use [Distribution](#distribution) to find its scripts and required setup.

## Versioning

Use Semantic Versioning (`major.minor.patch`). Before 1.0, minor releases may break the spec format; patch releases contain bug fixes only. Breaking changes go in `CHANGELOG.md`.

## Cut a release

Follow the `cut-release` skill's [release guide](../../.agnostic-ai/skills/cut-release/SKILL.md). It folds the `changes/*.md` entries into `CHANGELOG.md` with `make changelog`, prepares the dated changelog, raises the binary size upper bound with `make size-budget`, updates the version, and creates `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` immediately before the signed release commit and tag. The announcement summarizes the release and links to the GitHub release for the complete record. Keep verified upstream CLI and model news separate.

Run `make site-build site-test` before the release commit. Confirm the article,
updates archive, RSS entry identifier (GUID), old `.html` address, and sitemap URL are generated.
The version, changelog, and announcement must share one commit. Push that commit,
wait for its complete continuous integration (CI) run on Linux, macOS, and Windows, then sign and push the tag.

Before tagging, confirm that the manual CI run for the release commit built and verified both macOS disk images and companion archives, and that its web client check passed. Go users can install a stable tag as soon as it is pushed.

The tag workflow builds one candidate and tests the dashboard inside its Linux archive. It checks the asset inventory and checksums, then signs the candidate files and package manifests. It uploads the verified assets to a draft, compares the uploaded digests, and publishes the draft. Stable releases require both macOS disk images and both companion archives. Prereleases can omit them.

The tag workflow uses `scripts/release-notes.sh` to build GitHub Release notes from the dated `CHANGELOG.md` section and the verified asset inventory.

After pushing the tag, watch both workflows:

- `Release` publishes the GitHub Release and then starts the distribution jobs.
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

GoReleaser prepares the manifests. Once the release is public, each channel job verifies its inputs before publishing with its own credential. Unconfigured publishers show as skipped. The npm job verifies all seven packages and their distribution tags. The Homebrew availability job checks the stable cask independently. A winget submission shows as pending until the catalog accepts it.

- **Homebrew**: `Chemaclass/homebrew-tap` updates its own cask without a secret in this repository. Its [Update agnostic-ai cask workflow](https://github.com/Chemaclass/homebrew-tap/blob/main/.github/workflows/update-agnostic-ai-cask.yml) runs at minutes 23 and 53 each hour. It resolves the latest release, rewrites `Casks/agnostic-ai.rb` from `checksums.txt`, checks that all four archives return 200, and commits with its `GITHUB_TOKEN`. The cask can lag until the next successful run; dispatch the workflow to update it sooner.

  Optional pushes at tag time use `HOMEBREW_TAP_APP_ID`/`HOMEBREW_TAP_APP_PRIVATE_KEY` or `HOMEBREW_TAP_TOKEN`; both produce identical files. These cross-repository pushes need a credential created through a browser, since GitHub has no API to create an App or personal access token (PAT). Missing credentials left ten releases with a stale cask (#943, #920). Scoop and winget still lack credentials and stay skipped.
- **Scoop**: create the public repo `Chemaclass/scoop-bucket` with a `main` branch, then add a `SCOOP_BUCKET_TOKEN` repo secret (PAT with `contents: write` on that repo). Users: `scoop bucket add chemaclass https://github.com/Chemaclass/scoop-bucket`.
- **winget**: fork `microsoft/winget-pkgs` to `Chemaclass/winget-pkgs`, then add `WINGET_TOKEN` (PAT with `contents: write` on the fork and `pull_requests: write` upstream). Microsoft reviews each PR, so a new version lands in `winget search` hours to days after the GitHub release.
- **npm**: `npm/` holds the parent package. Add `NPM_TOKEN` (automation token). The token must publish `agnostic-ai` and create packages in the `agnostic-ai` npm organization. Create that organization before the first release with platform packages.

### npm publish order

The parent requires exact versions of six platform packages, so the release publishes them first and the parent last:

1. `scripts/npm-binaries.sh <tag> <dir> <commit>` downloads the six release archives at that tag. It checks each archive against `checksums.txt` and the build provenance from `release.yml` for that exact tag and commit, then unpacks one program per platform.
2. `npm/scripts/build-platform-packages.js --binaries <dir> --version <x.y.z>` writes `npm/platforms/<os>-<cpu>/` and pins the parent to all six.
3. `scripts/npm-publish.sh <x.y.z>` runs up to three platform package operations at once. It waits for all six versions and their provenance before publishing the parent. After a group of fresh uploads is visible, it waits five more minutes (`NPM_ATTESTATION_SETTLE`) before reading provenance. The parent follows the same checks. Existing versions are verified before they count as complete. The log reports publication, registry visibility, and provenance timing separately.

   The wait before the first check matters. npm caches each signed record response for a year, and npm adds its own signed record about a minute after it serves the version. A check that reads the record too early stores an incomplete copy, and later checks that hit the same cache keep failing with "has attestations but no corresponding public key(s) can be found".

Publishing the parent before a required platform package breaks `npm install` on that platform. npm scans each publish before serving it, commonly taking about five minutes and sometimes more than 15. Visibility and distribution tag checks use bounded backoff that covers about 20 minutes. Provenance read failures also retry with backoff.

The v0.96.0 npm job took 32m 40s, including 10 minutes of attestation settle time. In an offline six-package fixture with fixed registry delays, the old publisher took 41.5s and the bounded publisher took 19.7s. A retry with one missing platform took 14.6s and 18.0s respectively. Use the phase timings from the next release to measure live registry performance.

### Required npm provenance and recovery

Every new publish uses `--provenance`, which attaches signed records of the build and publication. If publication fails with those records enabled, it never retries without them. An existing version counts as complete only when its signed records pass verification. The publisher and distribution job check all seven packages without installing or executing them.

`scripts/npm-verify-provenance.js` uses the pinned npm CLI's bundled verification libraries and Sigstore trust root. It verifies the registry signature, build and registry publish attestations, and downloaded tarball integrity. It requires the build certificate to name this repository's `.github/workflows/release.yml` at the version tag, issued by GitHub Actions, and the signed source commit to match the release commit. A registry signature alone does not prove where a package was built.

To retry a failed channel, run the Release workflow on the published tag that contains the recovery workflow:

```bash
gh workflow run release.yml --ref vX.Y.Z -f channel=npm
```

Use `homebrew`, `scoop`, or `winget` to retry that channel. The dispatch checks the release and its commit. npm uses the public, attested CLI archives. The manifest channels use the original signed Actions artifact, retained for up to 90 days, and compare it with the published release. The Homebrew tap's own update workflow can rebuild its cask from the public release. For Scoop or winget, prepare a new verified release if the retained manifest bytes have expired and there is no original artifact to verify.

Verified npm versions are reused. If an older release needs a missing package and a newer version holds the channel tag, the publisher uses a version-specific recovery tag and preserves the newer tag. An npm version published without its required attestation needs a new release of all seven packages because npm versions are immutable.

If candidate validation or attestation fails, no GitHub release is published. Re-run the failed job against the staged candidate while its Actions artifact is available. If publication stops after an upload, the release stays a draft. Re-run **Publish the verified release** against the same signed candidate. It compares existing draft assets and uploads missing files. It stops if any existing file has a different digest. Check the failing file and the retained candidate before touching a draft. Published releases keep their original files.

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

The npm job verifies the exact packages and confirms the intended distribution tags. A superseded release can still be installed by exact version. Install a prerelease with `npm install -g agnostic-ai@beta`.

`scripts/npm-smoke.sh` runs the same generator against programs built locally for each platform and installs the result from tarballs. Use it to check a change to any of the three scripts without cutting a release.

## Backporting

Cherry-pick fixes for the previous minor onto `release/X.Y`. Tag `vX.Y.Zpatch`.
