# Changelog

All notable changes to the agnostic-ai JetBrains plugin are documented
in this file.

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
