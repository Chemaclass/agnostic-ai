+++
title = "Installation"
description = "Install agnostic-ai on macOS, Linux, or Windows, then verify and upgrade it."
weight = 10

[extra]
group = "Start"
scripts = ["assets/scripts/landing.js"]
+++

# Installation

Every installer puts the same prebuilt binary for your OS and CPU on your machine. You do not need Go.

{{ <install_picker /> }}

## Verify the install

```bash
agnostic-ai --version
# agnostic-ai version X.Y.Z
```

Command not found? Open a new terminal, or add the install directory to `PATH`.

Next, follow [Getting started](@/docs/getting-started.md). If you already have `CLAUDE.md`, `AGENTS.md`, or other tool configuration, follow [Migration](@/docs/migration.md) instead.

## Upgrade

| Command | Effect |
|---|---|
| `agnostic-ai upgrade` | Upgrades the detected install to the latest release. |
| `agnostic-ai upgrade --check` | Inspects the install and finds older binaries on `PATH`, without changing anything. |
| `agnostic-ai upgrade --version vX.Y.Z` | Installs one release instead of the latest, including an older one a project pins. |

Package-manager installs upgrade through their package manager. A standalone binary on macOS or Linux is downloaded, checked against the release checksum, and replaced in place. See the [upgrade reference](@/docs/cli-reference/maintain.md#upgrade).

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

With npm, pin the package: `npm install -g agnostic-ai@X.Y.Z`.

## Optional extras

Shell completion for Bash, Zsh, Fish, and PowerShell: see [completion](@/docs/cli-reference/maintain.md#completion).

The Claude Code [plugin](https://github.com/Chemaclass/agnostic-ai/tree/main/plugins/agnostic-ai) adds install, setup, import, and sync commands inside Claude Code:

```text
/plugin marketplace add Chemaclass/agnostic-ai
/plugin install agnostic-ai@chemaclass
```

## Other install methods

| Method | Instructions |
|---|---|
| Go | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` (Go version from [go.mod](https://github.com/Chemaclass/agnostic-ai/blob/main/go.mod) or newer; put `$(go env GOPATH)/bin` on `PATH`) |
| Manual download | Download your OS/CPU archive and `checksums.txt` from [GitHub Releases](https://github.com/Chemaclass/agnostic-ai/releases), verify the checksum, and extract the binary into a directory on `PATH` |

## Troubleshooting

{% <details summary="npm reports a missing platform package"> %}
npm skipped the optional dependency that carries the binary. That happens with `--omit=optional`, and with a lockfile copied from another platform. Reinstall with the optional dependencies included:

```bash
npm install -g agnostic-ai --force --include=optional
```

Drop `-g` for a project install. `--include=optional` also overrides an `omit=optional` left in your npm config. The CLI's error message names the right form for your install.
{% </details> %}

{% <details summary="How the npm package works"> %}
[`agnostic-ai`](https://www.npmjs.com/package/agnostic-ai) is the only package you install. It is a wrapper whose optional dependencies are six platform packages in the [`@agnostic-ai`](https://www.npmjs.com/org/agnostic-ai) organization. npm installs only the one matching your OS and CPU, such as [`@agnostic-ai/darwin-arm64`](https://www.npmjs.com/package/@agnostic-ai/darwin-arm64). Nothing is downloaded and no install script runs, so `--ignore-scripts` and npm 11's install-script prompt change nothing.

Set `AGNOSTIC_AI_BINARY` to an absolute path to run a binary the package does not ship.
{% </details> %}
