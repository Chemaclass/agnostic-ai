+++
title = "Verify a release"
description = "Check that the agnostic-ai binary you install is the one this repository's release workflow built."
weight = 145

[extra]
group = "Reference"
+++

# Verify a release

The install scripts and `agnostic-ai upgrade` already check each download against the release checksum, and against its [build provenance](#build-provenance) when the GitHub CLI is installed and logged in. Use the checks below for a manual download, or for proof of where a release came from. Replace `vX.Y.Z` and the archive filename with your release and platform.

| Check | Proves | Command |
|---|---|---|
| Checksum | The archive matches the release's `checksums.txt`. | `sha256sum -c` |
| Proof of where the archive was built | This repository's release workflow built the archive from the tagged commit. | `gh attestation verify` |
| List of included modules (SBOM) | Which Go modules and versions are in the binary. | `<archive>.sbom.json` |
| Proof of where the npm package was built | The npm package came from this repository's release workflow. | `npm audit signatures` |
| Signed tag | The maintainer signed the release commit and tag. | `git verify-tag` |

<details>
<summary>Check a downloaded archive</summary>

## Checksums

`install.sh`, `install.ps1`, and `agnostic-ai upgrade` stop on a checksum mismatch, a missing `checksums.txt`, or a missing SHA-256 tool. To check by hand:

```bash
curl -fsSLO https://github.com/Chemaclass/agnostic-ai/releases/download/vX.Y.Z/agnostic-ai_linux_amd64.tar.gz
curl -fsSLO https://github.com/Chemaclass/agnostic-ai/releases/download/vX.Y.Z/checksums.txt
grep ' agnostic-ai_linux_amd64.tar.gz$' checksums.txt | sha256sum -c    # macOS: shasum -a 256 -c
```

The checksum comes from the same release page. It catches a corrupted download, not a compromised release.

</details>

<details>
<summary>Verify where the archive was built</summary>

## Check where the archive was built {#build-provenance}

Releases after 0.74.0 include a signed record of where every archive and module list was built, called a [build provenance attestation](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations). The release workflow signs it through Sigstore without keeping a permanent signing key. Verify it with the [GitHub CLI](https://cli.github.com/):

```bash
gh attestation verify agnostic-ai_linux_amd64.tar.gz --repo Chemaclass/agnostic-ai
```

A pass means a workflow in `Chemaclass/agnostic-ai` built the file from a tagged commit. To check against the release's `agnostic-ai.intoto.jsonl` instead of GitHub's API:

```bash
gh attestation verify agnostic-ai_linux_amd64.tar.gz --repo Chemaclass/agnostic-ai --bundle agnostic-ai.intoto.jsonl
```

`install.sh`, `install.ps1`, and `agnostic-ai upgrade` run this check whenever `gh` is installed and logged in, require the attestation to come from `.github/workflows/release.yml` at the release tag, and stop when it fails. Without `gh`, with a `gh` that is logged out or too old to verify, or for a version you name from before 0.75.0, they say so in one line and install on the checksum alone. They refuse a latest release from before 0.75.0, since whoever can replace release files could publish one. `AGNOSTIC_AI_VERIFY_ATTESTATION` changes this for all three: `1` stops when the check cannot run (`-VerifyAttestation` for `install.ps1`), and `0` skips it.

</details>

<details>
<summary>Inspect the included modules</summary>

## List of included modules {#sbom}

Each archive has a software bill of materials (SBOM) in SPDX format, such as `agnostic-ai_linux_amd64.tar.gz.sbom.json`, that lists every Go module in the binary. Its checksum is in `checksums.txt`, and it has its own signed record of where it was built. Scan it with a tool such as `grype sbom:agnostic-ai_linux_amd64.tar.gz.sbom.json`.

</details>

<details>
<summary>Verify npm packages</summary>

## npm

The `agnostic-ai` package and its six `@agnostic-ai/*` platform packages are published with [npm provenance](https://docs.npmjs.com/generating-provenance-statements), a signed record of where they were built. In a project that installs them:

```bash
npm audit signatures
```

It checks installed packages' registry signatures and verifies build records when present. A pass alone does not prove every package has provenance or that it came from this repository. Each platform package holds the binary from the release archive, checked against `checksums.txt` before packing.

Official releases require provenance for the parent and all six platform packages. The release workflow also checks the GitHub Actions signing identity, this repository's release workflow at the version tag, the signed source commit, and the downloaded tarball integrity. It rejects missing or invalid signed build records, including for already published versions. These checks prove origin and integrity, not that the code has no defects.

From a checkout of the release tag, using the release's npm CLI version (11.5.1), check any one package without installing or running it:

```bash
. scripts/npm-publish.sh
verify_npm_provenance agnostic-ai X.Y.Z
verify_npm_provenance @agnostic-ai/linux-x64 X.Y.Z
```

If the registry is temporarily unavailable, retry later. A package version published without its signed build record cannot be repaired in place; maintainers must release a new version of all seven packages.

</details>

<details>
<summary>Verify signed tags and commits</summary>

## Signed tags and commits

Release commits and `vX.Y.Z` tags are GPG-signed with the maintainer's key, published at [github.com/Chemaclass.gpg](https://github.com/Chemaclass.gpg):

```bash
curl -fsSL https://github.com/Chemaclass.gpg | gpg --import
git verify-tag vX.Y.Z
```

</details>

<details>
<summary>Build the tagged source yourself</summary>

## Build from source

`go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@vX.Y.Z` builds the tagged source on your machine. Go checks the module against the [checksum database](https://sum.golang.org/), so a tag moved after publication fails.

</details>

## Report a problem

A failed check on a published release is a security issue. Report it through a [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new), not a public issue.
