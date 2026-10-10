# agnostic-ai for VS Code

> Author your AI specs once. Ship to every CLI. Live preview, drift
> detection, and one-click sync from inside VS Code.

This extension wraps the [agnostic-ai](https://github.com/Chemaclass/agnostic-ai)
CLI. Install `agnostic-ai` on `PATH`, or set `agnostic-ai.binaryPath` to its location.

## Features

| Surface | What you get |
|---------|--------------|
| Problems | Lint findings for saved Markdown and YAML specs and project configuration. Open a source or save it to check; fix and save to clear. |
| YAML schema | `agnostic-ai.yaml` (or the legacy `agnostic.config.yaml`) validates and autocompletes against the published JSON Schema (no `# yaml-language-server:` line needed). |
| Command palette | Run sync, check generated files, fix setup with Doctor, show status, or preview a spec. The first four open a terminal in the project root; previews use the output panel. |
| Codelens | Above each spec in `<base>/agents/`, `<base>/skills/`, `<base>/rules/`, `<base>/hooks/`, `<base>/mcps/`: one **Render to <target>** button per configured target. Output streams to the agnostic-ai output channel. |
| Status bar | Shows in sync, a drift count, or a failed check. Hover to read the result or failure reason. Click to run `sync --check` in a terminal. |
| Open canonical source | From a generated file (`.claude/rules/x.md`, `AGENTS.md`, ...), run `agnostic-ai: Open canonical source` from the command palette or the editor context menu. One source opens directly. A merged file lists every contributing spec by name and path. |

### Problems

The extension starts `agnostic-ai lsp` from the first configured workspace folder, using `agnostic-ai.binaryPath`. It checks saved project configuration and every configured source directory, including custom and absolute paths. Files keep their Markdown or YAML language mode. Unsaved edits are checked after you save.

Changing the selected project or binary stops the old service before starting another. Closing the workspace or disabling the extension stops the service and clears its diagnostics. Startup failures name the command and show how to fix the install or binary path.

### Drift status

The status bar checks generated files on save and every `driftPollSeconds`. It runs one check at a time. Saves and poll ticks during a check queue one follow-up check. A result from before the latest save does not replace the status.

- **In sync:** the check succeeded and found no drift.
- **N drifted:** the check found files that differ from the specs. Click to run the check in a terminal for repair instructions.
- **Check failed:** the check reported an error, failed without a drift result, or returned invalid output. Hover to read the reason, then click to check in a terminal.
- **Not found:** install the CLI on `PATH` or set `agnostic-ai.binaryPath`.

A later successful check clears the previous failure reason.

Closing the workspace or disabling the extension stops its background check. Changing the selected project or the configured binary cancels the old check before starting another.

### Open canonical source

Editing a generated file loses the change on the next sync. This
command takes you to the spec you should edit instead.

It asks the CLI with `agnostic-ai why <file> --format json`, run from
the project that owns the file: the nearest directory holding
`agnostic-ai.yaml` or `agnostic.config.yaml`, inside that file's
workspace folder. Source paths come from that reply, so configured
`sources:` directories and files ignored by Git both work. The reply is
validated before any path opens. The command never runs `sync`; when
the file is untracked, the project never synced, a source is missing,
or the CLI is too old, it says what to run instead.

## Requirements

- VS Code 1.85 or newer.
- The `agnostic-ai` binary on `PATH`. Install from
  [the project README](https://github.com/Chemaclass/agnostic-ai#install)
  (Homebrew, Go install, or release binary).

## Settings

- `agnostic-ai.binaryPath` (string, default `agnostic-ai`); point at a
  specific install if multiple coexist.
- `agnostic-ai.driftPollSeconds` (number, default `30`); how often the
  status bar refreshes the drift count.
- `agnostic-ai.codeLens.enabled` (boolean, default `true`); toggle the
  per-spec render codelens.

## Develop

Use Node.js 22 or newer. The packaging tool requires Node 22; the extension still supports VS Code 1.85 or newer.

```bash
cd editors/vscode
npm ci
npm run compile         # one-shot tsc
npm run watch           # incremental compile while iterating
npm test                # compile, then run unit tests
AGNOSTIC_AI_TEST_BINARY=/path/to/agnostic-ai npm run test:integration
```

Tests cover project and source navigation helpers, plus the production status presentation in `src/drift.ts`. The navigation fixtures in
`test/fixtures/why/` are real `agnostic-ai why --format json` output
from a synced project with configured source paths containing spaces.
Recapture them when the `why` JSON envelope changes.

Press `F5` to launch a development host. Open a folder containing
`agnostic-ai.yaml` or `agnostic.config.yaml`; the extension activates
automatically (`activationEvents: workspaceContains:<name>` for each).
When both exist, `agnostic-ai.yaml` wins, as it does for the CLI.

## Publish

```bash
npm ci
npm run package          # produces agnostic-ai-<version>.vsix
npm run publish          # requires a Personal Access Token from
                         # https://dev.azure.com/<your-org>/_usersSettings/tokens
```

The publisher id is `Chemaclass`; talk to the maintainer for token
access. Publishing is intentionally a manual step rather than tied to
the agnostic-ai release tag. Marketplace releases follow their own
review schedules.

## Limits in v1

- No live hover preview yet (codelens covers the iteration loop).
