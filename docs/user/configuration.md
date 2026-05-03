# Configuration

`agnostic.config.yaml` lives at the project root.

## Schema

```yaml
version: 1

sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks

targets:
  - claude
  - codex
  - gemini
  - cursor

outputs:
  claude:
    dir: .claude
    rules-file: CLAUDE.md
  codex:
    file: AGENTS.md
  gemini:
    file: GEMINI.md
  cursor:
    rules-dir: .cursor/rules

on-unsupported: warn   # warn | error | silent
```

## Fields

| Field | Description |
|-------|-------------|
| `version` | Schema version. Always `1`. |
| `sources` | Per-kind source directories, relative to the config file. |
| `targets` | Adapter names to emit. Unknown targets are reported and skipped. |
| `outputs` | Per-target output paths. See [targets](targets.md). |
| `on-unsupported` | `warn` (default), `error`, or `silent`. |
