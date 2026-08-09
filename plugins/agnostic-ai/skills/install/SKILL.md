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
   | macOS / Linux, no Homebrew | install script | `curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh \| bash` |
   | Windows | install script | `irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 \| iex` |
   | Any platform with Go | Go | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |

   Check for a tool with `command -v brew` / `command -v go`.

   winget, Scoop, and npm are wired into the release pipeline but not published yet, so `winget install Chemaclass.agnostic-ai`, `scoop install agnostic-ai`, and `npx agnostic-ai` all fail today. Do not offer them.
3. Run the chosen command. Do not run more than one route. If it reports that no package matches, fall back to the install script for that platform, which reads straight from the repo.
4. Verify with `agnostic-ai --version`.
5. If the shell reports "command not found" right after a successful install, PATH has not been reloaded yet. Tell the user to open a new terminal. The install scripts print the directory they used.

## Notes

- The install scripts verify the download against the release `checksums.txt`. Do not replace them with a bare `curl` of the binary.
- Already installed but out of date: `agnostic-ai upgrade` detects the original install method and prints the matching command. `agnostic-ai upgrade --run` executes it.
