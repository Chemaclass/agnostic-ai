+++
title = "Installation"
description = "Install agnostic-ai on macOS, Linux, or Windows, then verify and upgrade it."
weight = 10

[extra]
group = "Start"
+++

# Installation


The install scripts download a prebuilt release for your OS and CPU, then verify it against the release checksums. You do not need Go.

## macOS and Linux

```bash
curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
```

The binary goes to `/usr/local/bin` when writable, otherwise `~/.local/bin`. Add the destination to `PATH` if needed. Set `AGNOSTIC_AI_INSTALL_DIR` to choose a directory or `AGNOSTIC_AI_VERSION` to pin a version.

## Windows

Run in PowerShell:

```powershell
irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 | iex
```

The default destination is `%LOCALAPPDATA%\Programs\agnostic-ai`. Open a new terminal if the command is not found. Download the [script](https://github.com/Chemaclass/agnostic-ai/blob/main/scripts/install.ps1) for its `-InstallDir` and `-Version` options.

## Other install options

| Method | Instructions |
|---|---|
| Go | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` (Go version from [go.mod](https://github.com/Chemaclass/agnostic-ai/blob/main/go.mod) or newer; put `$(go env GOPATH)/bin` on `PATH`) |
| Manual download | Download your OS/CPU archive and `checksums.txt` from [GitHub Releases](https://github.com/Chemaclass/agnostic-ai/releases), verify the checksum, and extract the binary into a directory on `PATH` |

Package-manager publishing is maintained separately from release archives. See [release distribution](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/release-process.md#distribution) for the configured channels.

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
