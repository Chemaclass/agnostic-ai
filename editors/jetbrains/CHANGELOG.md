# Changelog

All notable changes to the agnostic-ai JetBrains plugin are documented
in this file.

## Unreleased

- Read `agnostic-ai.yaml` as well as the legacy `agnostic.config.yaml`,
  preferring the new name like the CLI. Projects on the new name get root
  detection, configured targets, schema validation, and the drift status bar.
  `agnostic.config.yml`, which the CLI never read, no longer gets the schema.

## 0.1.0 — 2026-05-13

Initial release.

- Schema-backed editing for `agnostic.config.yaml` via the published
  JSON Schema.
- Tools menu entries: `Sync`, `Sync — check for drift`,
  `Doctor — auto-fix`, `Status`. Each runs in a background task and
  reports via IDE notification.
- Editor banner above each spec in the configured source folders with
  one "Render to <target>" link per configured target. Output streams
  to a notification.
- Status bar widget polling `sync --check --json` for the current drift
  count. Click runs `Sync — check`.
- Settings page at `Settings ▸ Tools ▸ agnostic-ai` for the binary
  path, drift poll interval, and the line-marker toggle.
- No bundled binary; shells out to whichever `agnostic-ai` is on
  `PATH`.
