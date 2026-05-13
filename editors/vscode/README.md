# agnostic-ai for VS Code

> Author your AI specs once. Ship to every CLI. Live preview, drift
> detection, and one-click sync from inside VS Code.

This extension wraps the [agnostic-ai](https://github.com/Chemaclass/agnostic-ai)
CLI. It does **not** ship a bundled binary — it shells out to the
`agnostic-ai` you already have on `PATH`. Same outputs as the CLI; same
review surface as `agnostic-ai sync --check`.

## Features

| Surface | What you get |
|---------|--------------|
| YAML schema | `agnostic.config.yaml` validates and autocompletes against the published JSON Schema (no `# yaml-language-server:` line needed). |
| Command palette | `agnostic-ai: Sync`, `Sync — check for drift`, `Doctor — auto-fix`, `Status`, `Render current spec to a target`. Each command opens a terminal in the project root. |
| Codelens | Above each spec in `<base>/agents/`, `<base>/skills/`, `<base>/rules/`, `<base>/hooks/`, `<base>/mcps/`: one **Render to <target>** button per configured target. Output streams to the agnostic-ai output channel. |
| Status bar | Polls `sync --check --json` and shows the current drift count. Click to run sync --check in a terminal. |

## Requirements

- VS Code 1.85 or newer.
- The `agnostic-ai` binary on `PATH`. Install from
  [the project README](https://github.com/Chemaclass/agnostic-ai#install)
  (Homebrew, Go install, or release binary).

## Settings

- `agnostic-ai.binaryPath` (string, default `agnostic-ai`) — point at a
  specific install if multiple coexist.
- `agnostic-ai.driftPollSeconds` (number, default `30`) — how often the
  status bar refreshes the drift count.
- `agnostic-ai.codeLens.enabled` (boolean, default `true`) — toggle the
  per-spec render codelens.

## Develop

```bash
cd editors/vscode
npm install
npm run compile         # one-shot tsc
npm run watch           # incremental compile while iterating
```

Press `F5` to launch a development host. Open a folder containing
`agnostic.config.yaml`; the extension activates automatically
(`activationEvents: workspaceContains:agnostic.config.yaml`).

## Publish

```bash
npm install
npm run package          # produces agnostic-ai-<version>.vsix
npm run publish          # requires a Personal Access Token from
                         # https://dev.azure.com/<your-org>/_usersSettings/tokens
```

The publisher id is `Chemaclass`; talk to the maintainer for token
access. Publishing is intentionally a manual step rather than tied to
the agnostic-ai release tag — extension marketplaces have their own
review SLAs.

## Limits in v1

- No live hover preview yet (codelens covers the iteration loop).
- No JetBrains plugin yet; tracked separately.
- Drift status updates on save and every `driftPollSeconds`. Watching
  `agnostic.config.yaml` for changes is on the follow-up list.
