+++
title = "Project CLI and readiness"
description = "Select the installed project CLI, repair dependencies, and check startup readiness."
weight = 56

[extra]
nav_title = "Project readiness"
group = "Reference"
+++

# Project CLI and readiness

## project

Run the project's installed agnostic-ai binary after checking its version against the project's `requires` and any exact stable version in `package.json`. It prefers `node_modules/.bin/agnostic-ai` over `PATH`; when the project declares an agnostic-ai dependency, a global binary never stands in for the local one. On Windows it runs the package's Node launcher directly.

```bash
agnostic-ai project                     # sync --keep-edits --quiet
agnostic-ai project --check             # sync --check, no install or writes
agnostic-ai project --check --against index
agnostic-ai project --bootstrap         # repair dependencies once, then sync
agnostic-ai project -- hook memory --target codex
```

- It finds the nearest project config at or above the working directory; a built-in hook can use the host's project-directory variable instead.
- A mismatch names the installed and required versions and the `agnostic-ai project --bootstrap` recovery. Conflicting exact declarations stop before any install. npm ranges, tags, aliases, prereleases, and file dependencies are not read as requirements; state those in `requires`.
- With `--against index` or `HEAD`, the version contract comes from the staged files or commit, and the binary from the working checkout.
- Version and command checks share ten seconds. A failed check shows up to 64 KiB of the command's error output, marking cut-off text, timeouts, and cancellation. Warnings from a successful command do not fail it.
- A binary without `project` (such as v0.82.0) gets an instruction to upgrade the owning package through its package manager.

| Flag | Description |
|------|-------------|
| `--check` | Check the version and generated output without installing or writing. |
| `--against index\|HEAD` | With `--check`, use the existing check for staged or committed output. |
| `--bootstrap` | If the binary is missing or mismatched, install once from the npm, pnpm, or bun lockfile, then sync preserving manual edits. |

Bootstrap picks the package manager from `packageManager` or `devEngines.packageManager` in `package.json`, or an unambiguous root lockfile, and runs `npm ci`, `pnpm install --frozen-lockfile`, or `bun install --frozen-lockfile`. It needs a declared agnostic-ai dependency and a committed lockfile (`bun.lock` or `bun.lockb` for bun) that installs an allowed version; it never changes `requires` or the manifest. Other package managers get an error naming their install command. Package scripts run as usual. `AGNOSTIC_AI_PROJECT_BOOTSTRAP=1` stops a nested helper call from installing again; postinstall can still regenerate output. With the right binary in place, it installs nothing. A CLI that supports `project` must already be installed, globally or locally; hook managers can then call it without changing their own config.

### Project readiness

Before starting a cached environment, check the installed project binary and generated files from the project root, in a shell that stops on failure:

```bash
set -e
agnostic-ai project --check
agnostic-ai project -- doctor --scope project --check-references
```

Start your environment after these lines, so a failed check stops it. The second command checks every configured target, including local Markdown links in generated skills, without approving hooks on this machine. The project binary must support `doctor --scope project` (v0.82.0 does not). Keep the package pin, lockfile, and `requires` aligned.

- Missing or mismatched project binary: run `agnostic-ai project --bootstrap`, then repeat the checks.
- Only ignored generated files missing: run `agnostic-ai project`.
- Drift: move manual edits into the source first.

A checkout hook that warns and lets Git continue does not prove startup is ready. These checks cover project configuration, not running services.
