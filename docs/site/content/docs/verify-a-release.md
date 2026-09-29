+++
title = "Verify a release"
description = "Check that the agnostic-ai binary you install is the one this repository's release workflow built, before you run it."
weight = 12

[extra]
group = "Start"
+++

# Verify a release

Every release ships proof you can check yourself instead of trusting the download. Each check below answers a different question, so pick the ones your threat model needs.

| Check | Proves | Command |
|---|---|---|
| Checksum | The archive matches the release's `checksums.txt`. | `sha256sum -c` |
| Build provenance | This repository's release workflow built the archive from the tagged commit. | `gh attestation verify` |
| SBOM | Which Go modules, at which versions, went into the binary. | `<archive>.sbom.json` |
| npm provenance | The npm package came from this repository's release workflow. | `npm audit signatures` |
| Signed tag | The maintainer signed the release commit and tag. | `git verify-tag` |

## Checksums

`install.sh`, `install.ps1`, and `agnostic-ai upgrade` compare the archive's SHA-256 with `checksums.txt` from the same release and stop on a mismatch. The installers also stop when `checksums.txt` cannot be downloaded or no SHA-256 tool is found, so an archive never installs unchecked. By hand:

```bash
curl -fsSLO https://github.com/Chemaclass/agnostic-ai/releases/download/vX.Y.Z/agnostic-ai_linux_amd64.tar.gz
curl -fsSLO https://github.com/Chemaclass/agnostic-ai/releases/download/vX.Y.Z/checksums.txt
grep ' agnostic-ai_linux_amd64.tar.gz$' checksums.txt | sha256sum -c    # macOS: shasum -a 256 -c
```

A checksum comes from the same release page as the archive, so it catches a corrupted or swapped download, not a compromised release.

## Build provenance

Releases after 0.74.0 carry a signed [build provenance attestation](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations) for every archive and SBOM. It is signed through Sigstore by the release workflow itself, so no long-lived key exists to leak. Verify an archive with the [GitHub CLI](https://cli.github.com/):

```bash
gh attestation verify agnostic-ai_linux_amd64.tar.gz --repo Chemaclass/agnostic-ai
```

A pass means the file was built by a workflow run in `Chemaclass/agnostic-ai` from a tagged commit. To have the installers run that check, set `AGNOSTIC_AI_VERIFY_ATTESTATION=1` for `install.sh`, or pass `-VerifyAttestation` to `install.ps1`. Both then stop when `gh` is missing or the attestation does not verify.

## SBOM

Each archive has an SPDX SBOM next to it, such as `agnostic-ai_linux_amd64.tar.gz.sbom.json`, listing every Go module in the binary. It is in `checksums.txt` and carries its own provenance. Feed it to a scanner such as `grype sbom:agnostic-ai_linux_amd64.tar.gz.sbom.json` to check for known vulnerabilities.

## npm

The `agnostic-ai` package and its six `@agnostic-ai/*` platform packages are published with [npm provenance](https://docs.npmjs.com/generating-provenance-statements). In a project that installs them:

```bash
npm audit signatures
```

It checks the registry signature of every installed package and the provenance attestation of each that has one. The platform package's binary is the release archive's binary, checked against `checksums.txt` before it is packed.

## Signed tags and commits

Release commits and `vX.Y.Z` tags are GPG-signed with the maintainer's key, published at [github.com/Chemaclass.gpg](https://github.com/Chemaclass.gpg):

```bash
curl -fsSL https://github.com/Chemaclass.gpg | gpg --import
git verify-tag vX.Y.Z
```

## Build from source

`go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@vX.Y.Z` builds the tagged source on your machine. The Go toolchain checks the module against the [Go checksum database](https://sum.golang.org/), so a tag moved after publication fails the install.

## Report a problem

A check that fails on a published release is a security issue. Report it through a [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new), not a public issue.
