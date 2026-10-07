+++
title = "Verify a release"
description = "Check that the agnostic-ai binary you install is the one this repository's release workflow built."
weight = 12

[extra]
group = "Start"
+++

# Verify a release

You can check every release yourself. Each check answers a different question. Pick the ones you need.

| Check | Proves | Command |
|---|---|---|
| Checksum | The archive matches the release's `checksums.txt`. | `sha256sum -c` |
| Build provenance | This repository's release workflow built the archive from the tagged commit. | `gh attestation verify` |
| SBOM | Which Go modules and versions are in the binary. | `<archive>.sbom.json` |
| npm provenance | The npm package came from this repository's release workflow. | `npm audit signatures` |
| Signed tag | The maintainer signed the release commit and tag. | `git verify-tag` |

## Checksums

`install.sh`, `install.ps1`, and `agnostic-ai upgrade` check the archive against `checksums.txt`. They stop on a mismatch, a missing `checksums.txt`, or a missing SHA-256 tool. To check by hand:

```bash
curl -fsSLO https://github.com/Chemaclass/agnostic-ai/releases/download/vX.Y.Z/agnostic-ai_linux_amd64.tar.gz
curl -fsSLO https://github.com/Chemaclass/agnostic-ai/releases/download/vX.Y.Z/checksums.txt
grep ' agnostic-ai_linux_amd64.tar.gz$' checksums.txt | sha256sum -c    # macOS: shasum -a 256 -c
```

The checksum comes from the same release page. It catches a corrupted download, not a compromised release.

## Build provenance

Releases after 0.74.0 carry a [build provenance attestation](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations) for every archive and SBOM. The release workflow signs it through Sigstore, with no long-lived key. Verify it with the [GitHub CLI](https://cli.github.com/):

```bash
gh attestation verify agnostic-ai_linux_amd64.tar.gz --repo Chemaclass/agnostic-ai
```

A pass means a workflow in `Chemaclass/agnostic-ai` built the file from a tagged commit. To check against the release's `agnostic-ai.intoto.jsonl` instead of GitHub's API:

```bash
gh attestation verify agnostic-ai_linux_amd64.tar.gz --repo Chemaclass/agnostic-ai --bundle agnostic-ai.intoto.jsonl
```

To make the installers run this check, set `AGNOSTIC_AI_VERIFY_ATTESTATION=1` for `install.sh` or pass `-VerifyAttestation` to `install.ps1`. Both stop if `gh` is missing or the check fails.

## SBOM

Each archive has an SPDX SBOM next to it, such as `agnostic-ai_linux_amd64.tar.gz.sbom.json`. It lists every Go module in the binary. It is in `checksums.txt` and has its own provenance. Scan it with a tool such as `grype sbom:agnostic-ai_linux_amd64.tar.gz.sbom.json`.

## npm

The `agnostic-ai` package and its six `@agnostic-ai/*` platform packages are published with [npm provenance](https://docs.npmjs.com/generating-provenance-statements). In a project that installs them:

```bash
npm audit signatures
```

It checks each package's registry signature and provenance. Each platform package holds the binary from the release archive, checked against `checksums.txt` before packing.

## Signed tags and commits

Release commits and `vX.Y.Z` tags are GPG-signed with the maintainer's key, published at [github.com/Chemaclass.gpg](https://github.com/Chemaclass.gpg):

```bash
curl -fsSL https://github.com/Chemaclass.gpg | gpg --import
git verify-tag vX.Y.Z
```

## Build from source

`go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@vX.Y.Z` builds the tagged source on your machine. Go checks the module against the [checksum database](https://sum.golang.org/), so a tag moved after publication fails.

## Report a problem

A failed check on a published release is a security issue. Report it through a [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new), not a public issue.
