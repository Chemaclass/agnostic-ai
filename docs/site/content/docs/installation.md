+++
title = "Installation"
description = "Install agnostic-ai on macOS, Linux, or Windows, then verify and upgrade it."
weight = 10

[extra]
group = "Start"
+++

# Installation


Every route installs the same prebuilt binary for your OS and CPU. You do not need Go.

Homebrew on macOS, the install script on Linux and Windows, npm wherever Node already is.

## Homebrew

macOS and Linux.

```bash
brew install --cask Chemaclass/tap/agnostic-ai
```

Upgrade with `brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai`.

## Install script

macOS and Linux.

```bash
curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
```

The script picks the archive for your OS and CPU, then verifies it against the release checksums. The binary goes to `/usr/local/bin` when writable, otherwise `~/.local/bin`. Add the destination to `PATH` if needed. Set `AGNOSTIC_AI_INSTALL_DIR` to choose a directory or `AGNOSTIC_AI_VERSION` to pin a version.

## Windows

Run in PowerShell:

```powershell
irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 | iex
```

The default destination is `%LOCALAPPDATA%\Programs\agnostic-ai`. Open a new terminal if the command is not found. Download the [script](https://github.com/Chemaclass/agnostic-ai/blob/main/scripts/install.ps1) for its `-InstallDir` and `-Version` options.

## npm

Node 18 or newer, on macOS, Linux, or Windows.

```bash
npm install -g agnostic-ai
```

Run it once without installing:

```bash
npx agnostic-ai sync
```

The package is a wrapper. The binary ships inside a platform package (`@agnostic-ai/darwin-arm64` and five siblings), and npm installs the one matching your OS and CPU. Nothing is downloaded and no install script runs, so `--ignore-scripts` and npm 11's install-script prompt change nothing.

Pin a version by pinning the package: `npm install -g agnostic-ai@<version>`. Set `AGNOSTIC_AI_BINARY` to an absolute path to run a binary the package does not ship.

If the CLI reports a missing platform package, npm skipped the optional dependency. That happens with `--omit=optional`, and with a lockfile copied from another platform. Reinstall with `npm install -g agnostic-ai --force --include=optional` for a global install, or the same command without `-g` for a project one. `--include=optional` is what overrides an `omit=optional` left in your npm config, and the wrapper's error message already names the right form for your install.

## Other install options

| Method | Instructions |
|---|---|
| Go | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` (Go version from [go.mod](https://github.com/Chemaclass/agnostic-ai/blob/main/go.mod) or newer; put `$(go env GOPATH)/bin` on `PATH`) |
| Manual download | Download your OS/CPU archive and `checksums.txt` from [GitHub Releases](https://github.com/Chemaclass/agnostic-ai/releases), verify the checksum, and extract the binary into a directory on `PATH` |

winget and Scoop are built by the release pipeline but not published yet, so `winget install Chemaclass.agnostic-ai` and `scoop install agnostic-ai` both fail today. See [release distribution](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/release-process.md#distribution) for every configured channel.

## Verify the install

```bash
agnostic-ai --version
agnostic-ai --help
```

Then follow [Getting started](@/docs/getting-started.md), or [Migration](@/docs/migration.md) if you already have tool configuration.

## Upgrade

```bash
agnostic-ai upgrade
```

Upgrades the detected install to the latest release. Package-manager installs go through their package manager. A standalone binary on macOS or Linux is downloaded, checked against the release checksum, and replaced in place. `agnostic-ai upgrade --check` inspects the install and finds older binaries on `PATH` without changing anything. To pick one release instead of the latest, including an older one a project pins, pass `agnostic-ai upgrade --version v0.56.1`. See the [upgrade reference](@/docs/cli-reference.md#upgrade).

`agnostic-ai update` is an alias. The old `--run` flag still works but is no longer needed.

## Shell completion

See [completion](@/docs/cli-reference.md#completion) for Bash, Zsh, Fish, and PowerShell setup.

## Claude Code plugin

The [plugin](https://github.com/Chemaclass/agnostic-ai/tree/main/plugins/agnostic-ai) adds install, setup, import, and sync commands inside Claude Code:

```text
/plugin marketplace add Chemaclass/agnostic-ai
/plugin install agnostic-ai@chemaclass
```
