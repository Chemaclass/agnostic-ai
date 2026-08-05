---
name: install
description: Install or upgrade the agnostic-ai CLI on this machine, picking the route that fits the OS and what is already installed. Use when agnostic-ai is missing, when `agnostic-ai --version` fails, or before any other agnostic-ai skill runs.
---

# install

Puts the `agnostic-ai` binary on PATH without asking the user to hunt for a release archive.

## Steps

1. Run `agnostic-ai --version`. If it prints a version, stop: it is installed. Report the version and offer `agnostic-ai upgrade` if the user asked for the newest.
2. Detect the platform and pick the first route that applies:

   | Platform | Route | Command |
   |---|---|---|
   | macOS / Linux with Homebrew | Homebrew | `brew install --cask Chemaclass/tap/agnostic-ai` |
   | Windows with winget | winget | `winget install Chemaclass.agnostic-ai` |
   | Windows with Scoop | Scoop | `scoop bucket add chemaclass https://github.com/Chemaclass/scoop-bucket && scoop install agnostic-ai` |
   | Windows, neither | install script | `irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 \| iex` |
   | macOS / Linux, no Homebrew | install script | `curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh \| bash` |
   | Any platform with Go | Go | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
   | Any platform with Node | npx, no install | `npx agnostic-ai <command>` |

   Check for a tool with `command -v brew` / `command -v go`, or `Get-Command winget` / `Get-Command scoop` on Windows.
3. Run the chosen command. Do not run more than one route.
4. Verify with `agnostic-ai --version`.
5. If the shell reports "command not found" right after a successful install, PATH has not been reloaded yet. Tell the user to open a new terminal. The install scripts print the directory they used.

## Notes

- The install scripts verify the download against the release `checksums.txt`. Do not replace them with a bare `curl` of the binary.
- `npx agnostic-ai` needs no install at all, so it is the fastest way to try one command. Prefer a real install when the user will run `sync` repeatedly.
- Already installed but out of date: `agnostic-ai upgrade` detects the original install method and prints the matching command. `agnostic-ai upgrade --run` executes it.
