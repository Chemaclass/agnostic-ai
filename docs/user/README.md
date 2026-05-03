# User docs

Read in this order. Each builds on the previous.

1. [Getting started](getting-started.md) — install, scaffold, first sync. ~5 min.
2. [Spec format](spec-format.md) — the four kinds (agent, skill, rule, hook) plus the `x-<target>` namespace for tool-specific extensions.
3. [Targets](targets.md) — capability matrix and per-target output paths.
4. [Configuration](configuration.md) — `agnostic.config.yaml` schema and precedence.
5. [CLI reference](cli-reference.md) — every command and flag, including `sync --check` and `doctor`.

## Mental model

```
   agents/  skills/  rules/  hooks/        ← single source of truth
        │       │      │      │
        └───────┴──┬───┴──────┘
                   │  agnostic-ai sync
                   ▼
   ┌──────────┬──────────┬──────────┬──────────┬──────────┐
   │ CLAUDE.md│ AGENTS.md│ GEMINI.md│  .cursor │   ...    │  ← per-target outputs
   │ .claude/ │          │          │  /rules/ │          │     (regenerated)
   └──────────┴──────────┴──────────┴──────────┴──────────┘
```

Write the rule, agent, skill, or hook **once**. Every supported AI CLI reads
the same contract.

## Standards this rides on

- **[`AGENTS.md`](https://agents.md/)** — open contract format. Originally
  OpenAI for Codex CLI, donated to the Linux Foundation in Dec 2025. Read
  natively by Codex, Cursor, Gemini CLI, GitHub Copilot, Aider, Cline,
  Windsurf, Continue, and others.
- **[`agentskills.io`](https://agentskills.io/specification)** — open skill
  spec announced by Anthropic in Dec 2025. Defines the minimal frontmatter
  (`name`, `description`) and progressive-disclosure loading.

The CLI emits to whichever conventions each target reads, with the open
standards as the anchor.

## CI gate

Use `sync --check` (or `doctor`) as a CI step to fail the build when
emitted files drift from source specs:

```yaml
- run: agnostic-ai sync --check
```
