+++
title = "Project CLI and readiness"
description = "Select the installed project CLI, repair dependencies, and check startup readiness."
weight = 56

[extra]
group = "Reference"
+++

# Project CLI and readiness

## project

Run the project's installed agnostic-ai binary after checking its version against any exact stable version declared in `package.json` and the project's `requires` setting. The helper prefers `node_modules/.bin/agnostic-ai` over `PATH`. A project declaring an agnostic-ai dependency must have its local binary installed; a global binary does not silently replace it. On Windows, the helper runs the Node launcher installed with the package directly.

Conflicting exact declarations stop before installation. npm ranges, tags, aliases, prereleases, and file dependencies are not interpreted as requirements for the installed version; use `requires` for those declarations. Checks against `index` or `HEAD` read the contract from the selected staged files or commit and select the installed binary from the working checkout. Unsupported binaries receive an instruction to upgrade the owning package explicitly; released v0.82.0 lacks `project`.

```bash
agnostic-ai project                     # sync --keep-edits --quiet
agnostic-ai project --check             # sync --check, no install or writes
agnostic-ai project --check --against index
agnostic-ai project --bootstrap         # repair dependencies once, then sync
agnostic-ai project -- hook memory --target codex
```

The helper finds the nearest project config at or above the working directory. A built-in hook can use the host's project-directory environment variable instead. The installed version must satisfy `requires`; version mismatches name the installed version, required version, and `agnostic-ai project --bootstrap` recovery command. Bootstrap does not modify the requirement or package manifest, so the committed lockfile must install an allowed version.

Version and command checks have ten seconds in total. If a check fails, its error includes up to 64 KiB of the command's error output and marks any cut-off text. Errors also name timeouts or cancellation when they occur. A successful command's warnings do not make the check fail.

| Flag | Description |
|------|-------------|
| `--check` | Check the version and generated output without installing or writing. |
| `--against index\|HEAD` | With `--check`, use the existing check for staged or committed output. |
| `--bootstrap` | If the binary is missing or mismatched, install once from the npm, pnpm, or bun lockfile, then sync preserving manual edits. |

Bootstrap uses `packageManager` or `devEngines.packageManager` from `package.json`, or an unambiguous root lockfile. It runs `npm ci`, `pnpm install --frozen-lockfile`, or `bun install --frozen-lockfile`. Each requires a committed lockfile (`bun.lock` or `bun.lockb` for bun) and a declared agnostic-ai dependency. For other package managers, bootstrap exits with an error that names their install command; run it before invoking the helper. Normal package scripts run, including hook-manager setup and native builds. `AGNOSTIC_AI_PROJECT_BOOTSTRAP=1` prevents a recursive helper call from starting another install; postinstall can still regenerate output. Repeating bootstrap with the correct binary performs no install.

Existing hook managers can call this helper without changing their own configuration format. A CLI capable of running bootstrap must already be installed, globally or locally. Versions released before `project` existed must first be updated through their existing package-manager command.

### Project readiness

Before starting a cached environment, check that the installed project binary and generated files are usable. Run these commands from the project root in a shell that stops on failure:

```bash
set -e
agnostic-ai project --check
agnostic-ai project -- doctor --scope project --check-references
```

Put the commands that start your environment after these checks. A failed check stops startup. The second command checks every configured target, including local Markdown links in generated skills, without approving hooks on this machine.

The entry-point helper must already be installed at a release that supports `project`. The selected project binary must also support `doctor --scope project`. v0.82.0 supports neither; update it through your package manager first. Keep the package pin, lockfile and `requires` aligned.

For a missing or mismatched project binary, run `agnostic-ai project --bootstrap`, then repeat the checks above. Bootstrap installs from the lockfile and generates output. If only ignored generated files are missing, run `agnostic-ai project` first. Move manual edits into the source before repairing drift.

Checkout hooks can warn and let Git continue. That behavior does not prove startup is ready. These checks cover project configuration; use your application's own checks for running services.
