# agnostic-ai for JetBrains

> Author your AI specs once. Ship to every CLI. Live preview, drift
> detection, and one-click sync from inside IntelliJ IDEA, WebStorm,
> PyCharm, GoLand, and the rest of the JetBrains family.

This plugin wraps the [agnostic-ai](https://github.com/Chemaclass/agnostic-ai)
CLI. It does **not** ship a bundled binary — it shells out to the
`agnostic-ai` you already have on `PATH`. Same outputs as the CLI; same
review surface as `agnostic-ai sync --check`.

## Features

| Surface | What you get |
|---------|--------------|
| YAML schema | `agnostic.config.yaml` validates and autocompletes against the published JSON Schema (no `# yaml-language-server:` line needed). |
| Tools menu | `Tools ▸ agnostic-ai ▸ Sync / Sync — check / Doctor — auto-fix / Status`. Each runs in a background task and reports via IDE notification. |
| Render banner | Above each spec in `<base>/{agents,skills,rules,hooks,mcps}/`: an editor banner with one **Render to <target>** link per configured target. Output streams to a notification. |
| Status bar | Polls `sync --check --json` and shows the current drift count. Click runs `Sync — check`. |

## Requirements

- IntelliJ IDEA 2024.2+ (or any other JetBrains IDE on the same
  platform build).
- The `agnostic-ai` binary on `PATH`. Install from
  [the project README](https://github.com/Chemaclass/agnostic-ai#install).

## Settings

`Settings ▸ Tools ▸ agnostic-ai`:

- **Binary path** — pin a specific install when multiple coexist. Empty
  means resolve `agnostic-ai` on `PATH`.
- **Drift poll (seconds)** — how often the status bar refreshes
  (5–600s, default 30).
- **Show 'Render to <target>' banner above each spec** — toggle the
  editor banner.

## Build & run locally

```bash
cd editors/jetbrains
./gradlew runIde            # launches a sandboxed IDE with the plugin installed
./gradlew test              # JVM unit tests (no IDE needed)
./gradlew buildPlugin       # produces build/distributions/agnostic-ai-<version>.zip
./gradlew verifyPlugin      # runs the JetBrains plugin verifier against
                            # recommended IDE versions
```

The Gradle wrapper bootstraps Gradle 8.10.2 on first run.

## Publish

```bash
export JETBRAINS_MARKETPLACE_TOKEN=<token>
./gradlew publishPlugin
```

The publisher id matches `pluginGroup` in `gradle.properties`. Generate
the token from your JetBrains Marketplace account: Profile ▸ Marketplace ▸
Tokens. Publishing is intentionally a manual step rather than tied to
agnostic-ai release tags — Marketplace review has its own SLA.

## Limits in v0.1.0

- Status bar polls on a schedule; it does not reactively watch
  `agnostic.config.yaml` for changes (planned).
- "Render" surfaces output as a notification snippet (last 40 lines).
  A dedicated tool window is planned once the surface settles.
- No live hover preview yet (line markers cover the iteration loop).
