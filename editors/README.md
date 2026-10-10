# Editor extensions

First-party editor integrations for agnostic-ai. Both extensions
**use the user's installed `agnostic-ai` binary**. They ship
no bundled binary, matching the v1 acceptance criteria from
[issue #44](https://github.com/Chemaclass/agnostic-ai/issues/44).

| Editor | Status | Path | Marketplace |
|--------|--------|------|-------------|
| VS Code | shipped (v0.2.0) | [`editors/vscode/`](vscode/) | publish via `npm run publish` from the directory; Personal Access Token required |
| JetBrains | shipped (v0.1.0) | [`editors/jetbrains/`](jetbrains/) | publish via `./gradlew publishPlugin`; `JETBRAINS_MARKETPLACE_TOKEN` required |

VS Code shows saved-file lint findings in Problems for configured Markdown and YAML sources and project configuration. It starts the CLI language server automatically and retains ordinary Markdown and YAML editing. See its [setup and limits](vscode/README.md#problems).

## Why one repo

Keeping the extensions next to the CLI lets a feature ship with its server and tests in one change. Marketplace publishes stay manual because the
review cadences differ from agnostic-ai's own release cadence.
