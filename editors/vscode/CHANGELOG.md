# Changelog

All notable changes to the agnostic-ai VS Code extension are documented
in this file.

## 0.1.0 — 2026-05-13

Initial release.

- Schema-backed editing for `agnostic.config.yaml` via the published
  JSON Schema.
- Command palette entries: `sync`, `sync --check`, `doctor --fix`,
  `status`, and `render current spec to a target`.
- Codelens above each spec: one `Render to <target>` action per
  configured target. Output streams to the **agnostic-ai** output
  channel.
- Status bar item polling `sync --check --json` for the current drift
  count. Click to run `sync --check` in a terminal.
- Settings: `agnostic-ai.binaryPath`, `agnostic-ai.driftPollSeconds`,
  `agnostic-ai.codeLens.enabled`.
- No bundled binary; shells out to whichever `agnostic-ai` is on
  `PATH`.
