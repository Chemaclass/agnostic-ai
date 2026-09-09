# Installation

[User docs](README.md) · Next: [Getting started](getting-started.md)

The install scripts download a prebuilt release for your OS and CPU and verify it against the release checksums. You do not need Go.

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

The default destination is `%LOCALAPPDATA%\Programs\agnostic-ai`. Open a new terminal if the command is not found. Download the [script](../../scripts/install.ps1) to use its `-InstallDir` and `-Version` options.

## Other install options

| Method | Instructions |
|---|---|
| Go | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` (Go version from [go.mod](../../go.mod) or newer; put `$(go env GOPATH)/bin` on `PATH`) |
| Manual download | Download your OS/CPU archive and `checksums.txt` from [GitHub Releases](https://github.com/Chemaclass/agnostic-ai/releases), verify the checksum, and extract the binary into a directory on `PATH` |

Package-manager publishing is maintained separately from release archives. See [release distribution](../internal/release-process.md#distribution) for the configured channels.

## Verify the install

```bash
agnostic-ai --version
agnostic-ai --help
```

Then follow [Getting started](getting-started.md), or [Migration](migration.md) if you already have tool configuration.

## Upgrade

```bash
agnostic-ai upgrade
```

This prints the command for your detected install method. Add `--run` to execute it. Use `agnostic-ai upgrade --check` to diagnose an older binary shadowing the new one on `PATH`. See the [upgrade reference](cli-reference.md#upgrade).

## Shell completion

See [completion](cli-reference.md#completion) for Bash, Zsh, Fish, and PowerShell setup.

## Claude Code plugin

The [plugin](../../plugins/agnostic-ai/) provides install, setup, import, and sync commands inside Claude Code:

```text
/plugin marketplace add Chemaclass/agnostic-ai
/plugin install agnostic-ai@chemaclass
```
