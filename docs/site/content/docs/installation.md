+++
title = "Installation"
description = "Install agnostic-ai on macOS, Linux, or Windows, then verify and upgrade it."
weight = 10

[extra]
group = "Start"
scripts = ["assets/scripts/landing.js"]
+++

# Installation

Every installer puts the same prebuilt binary on your machine. You don't need Go. macOS needs 13 Ventura or later.

{{ <install_picker /> }}

## Verify the install

```bash
agnostic-ai --version
# agnostic-ai version X.Y.Z
```

Command not found? Open a new terminal, or add the install directory to `PATH`. To prove the binary came from this repository, see [Verify a release](@/docs/verify-a-release.md).

Next: [Getting started](@/docs/getting-started.md), or [Migration](@/docs/migration.md) if you already have `CLAUDE.md`, `AGENTS.md`, or other tool files.

## Upgrade

| Command | Effect |
|---|---|
| `agnostic-ai upgrade` | Upgrade to the latest release. |
| `agnostic-ai upgrade --check` | Report the install and older binaries on `PATH`. Changes nothing. |
| `agnostic-ai upgrade --version vX.Y.Z` | Install one specific release. |
| `agnostic-ai upgrade --requires` | Pin this project's config and schema to the installed release, then sync. |

Package-manager installs upgrade through their package manager. See the [upgrade reference](@/docs/cli-reference/maintain.md#upgrade).

## Pin a version or directory

The install script reads two environment variables:

```bash
curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh \
  | AGNOSTIC_AI_VERSION=vX.Y.Z AGNOSTIC_AI_INSTALL_DIR="$HOME/bin" bash
```

On Windows, download [`install.ps1`](https://github.com/Chemaclass/agnostic-ai/blob/main/scripts/install.ps1) and pass its options:

```powershell
.\install.ps1 -Version vX.Y.Z -InstallDir C:\tools\agnostic-ai
```

With npm: `npm install -g agnostic-ai@X.Y.Z`.

## Pin it per project

A JavaScript project can add the CLI as a dev dependency. Then every clone and CI job runs the release in the lockfile:

```bash
pnpm add -D agnostic-ai@X.Y.Z    # or npm install -D, yarn add -D, bun add -D
```

From the project root, run `agnostic-ai upgrade --requires` with that installed CLI, such as `pnpm exec agnostic-ai upgrade --requires`. It sets [`requires`](@/docs/configuration.md#requires) and the schema URL to the installed release, then syncs. Commit the config and dependency changes together. After a pull bumps both, run your package manager's install. Until then, commands stop with AAI-005.

## Optional extras

- Shell completion for Bash, Zsh, Fish, and PowerShell: [completion](@/docs/cli-reference/maintain.md#completion).
- The Claude Code [plugin](https://github.com/Chemaclass/agnostic-ai/tree/main/plugins/agnostic-ai) adds install, setup, import, and sync commands:

```text
/plugin marketplace add Chemaclass/agnostic-ai
/plugin install agnostic-ai@chemaclass
```

## Other install methods

| Method | Instructions |
|---|---|
| Go | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest`, with the Go version from [go.mod](https://github.com/Chemaclass/agnostic-ai/blob/main/go.mod) or newer. Put `$(go env GOPATH)/bin` on `PATH`. |
| Manual | Download your archive and `checksums.txt` from [GitHub Releases](https://github.com/Chemaclass/agnostic-ai/releases), verify the checksum, and put the binary on `PATH`. |

## Troubleshooting

{% <details summary="npm reports a missing platform package"> %}
npm skipped the optional dependency that holds the binary. This happens with `--omit=optional`, and with a lockfile copied from another platform. Reinstall with the optional dependencies included:

```bash
npm install -g agnostic-ai --force --include=optional
```

Drop `-g` for a project install. `--include=optional` also overrides an `omit=optional` left in your npm config. The CLI's error message names the right form for your install.
{% </details> %}

{% <details summary="How the npm package works"> %}
[`agnostic-ai`](https://www.npmjs.com/package/agnostic-ai) is the only package you install. It's a wrapper, and its optional dependencies are six platform packages in the [`@agnostic-ai`](https://www.npmjs.com/org/agnostic-ai) organization. npm installs only the one matching your OS and CPU, such as [`@agnostic-ai/darwin-arm64`](https://www.npmjs.com/package/@agnostic-ai/darwin-arm64). Nothing is downloaded and no install script runs, so `--ignore-scripts` and npm 11's install-script prompt change nothing.

Set `AGNOSTIC_AI_BINARY` to an absolute path to run a binary the package does not ship.
{% </details> %}
