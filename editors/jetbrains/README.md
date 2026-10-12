# agnostic-ai for JetBrains

Write specs, preview generated configuration, and sync coding tools from IntelliJ IDEA, WebStorm, PyCharm, GoLand, and other JetBrains editors.

This plugin wraps the [agnostic-ai](https://github.com/Chemaclass/agnostic-ai)
CLI. Install `agnostic-ai` on `PATH` before using the plugin. It runs that program to generate and check the same files as the CLI.

Open a project containing `agnostic-ai.yaml`, edit a source spec, and use **Render to <target>** above the spec to preview it. Choose **Tools ▸ agnostic-ai ▸ Sync** to update generated files.

## Features

| Editor feature | What you get |
|---------|--------------|
| YAML schema | `agnostic-ai.yaml`, or the legacy `agnostic.config.yaml`, validates and autocompletes against the published JSON Schema (no `# yaml-language-server:` line needed). |
| Tools menu | `Tools ▸ agnostic-ai ▸ Sync / Sync — check / Doctor — auto-fix / Status`. Each runs in a background task and reports via IDE notification. |
| Render banner | Above each spec in `<base>/{agents,skills,rules,hooks,mcps}/`: an editor banner with one **Render to <target>** link per configured target. Output streams to a notification. |
| Status bar | Polls `sync --check --json` and shows the number of generated files that differ from the specs. Click runs `Sync — check`. |

## Requirements

- IntelliJ IDEA 2024.2+ (or any other JetBrains IDE on the same
  platform build).
- The `agnostic-ai` binary on `PATH`. Install from
  [the project README](https://github.com/Chemaclass/agnostic-ai#install).
- JDK 21+ (for local development; the wrapper auto-downloads if
  missing using Gradle's JDK download support).

## Settings

`Settings ▸ Tools ▸ agnostic-ai`:

- **Binary path**: choose a specific install when multiple coexist. Empty
  means resolve `agnostic-ai` on `PATH`.
- **Drift poll (seconds)**: how often the status bar refreshes
  (5 to 600 seconds, default 30).
- **Show 'Render to <target>' banner above each spec**: toggle the
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

The Gradle wrapper bootstraps Gradle 9.0.0 on first run and checks the download against its pinned SHA-256 checksum. CI validates the committed wrapper JAR against Gradle's published checksums before running it.

## Publish

```bash
export JETBRAINS_MARKETPLACE_TOKEN=<token>
./gradlew publishPlugin
```

The publisher id matches `pluginGroup` in `gradle.properties`. Generate
the token from your JetBrains Marketplace account: Profile ▸ Marketplace ▸
Tokens. Publishing is intentionally a manual step rather than tied to
agnostic-ai release tags. Marketplace review follows its own schedule.

## Limits

- Status bar polls on a schedule; it does not watch
  `agnostic-ai.yaml` for changes (planned).
- "Render" shows output in a notification (last 40 lines).
  A dedicated tool window is planned after the preview design is settled.
- No preview when hovering over a spec. Use the **Render to <target>** links above it.
