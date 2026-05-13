# Editor extensions

First-party editor integrations for agnostic-ai. Both extensions
**shell out to the user's installed `agnostic-ai` binary** — they ship
no bundled binary, matching the v1 acceptance criteria from
[issue #44](https://github.com/Chemaclass/agnostic-ai/issues/44).

| Editor | Status | Path | Marketplace |
|--------|--------|------|-------------|
| VS Code | shipped (v0.1.0) | [`editors/vscode/`](vscode/) | publish via `npm run publish` from the directory; Personal Access Token required |
| JetBrains | shipped (v0.1.0) | [`editors/jetbrains/`](jetbrains/) | publish via `./gradlew publishPlugin`; `JETBRAINS_MARKETPLACE_TOKEN` required |

## Why one repo

Each extension is small (the VS Code extension fits in a single TS
file), and keeping them next to the CLI lets feature work land in one
PR instead of three. Marketplace publishes stay manual because the
review cadences differ from agnostic-ai's own release cadence.
