# User docs

Read in order. Each builds on the previous.

1. [Getting started](getting-started.md): install, scaffold, first sync. ~5 min.
2. [Spec format](spec-format.md): the ten kinds (agent, skill, rule, hook, MCP, command, settings, review, environment, ignore), per-directory scope, and the `x-<target>` namespace for tool-specific extensions.
3. [Targets](targets.md): capability matrix and per-target output paths (25 tools).
4. [Configuration](configuration.md): `agnostic-ai.yaml` schema, precedence, and the optional auto-managed `.gitignore` block.
5. [CLI reference](cli-reference.md): every command and flag, including `sync --watch`, `sync --check`, `sync --backup`, `init --demo`, `init --all`, `import <source>`, `revert`, `doctor`, `status`, and `--json` output on `sync`/`revert`/`doctor`/`status`.
6. [CI](ci.md): drift detection in pull requests with `sync --check`.
7. [Git hooks](git-hooks.md): pre-commit recipes for pre-commit, lefthook, husky.
8. [Packs](packs.md): install shared spec packs (`agnostic-ai packs add`).
9. [Why not symlinks](alternatives-why-not-symlinks.md): how agnostic-ai compares to symlinks, manual copies, and shared-file approaches, and when a simpler option suffices.

## Mental model

```
   agents/  skills/  rules/  hooks/  mcps/   ← single source of truth
        │       │      │      │       │
        └───────┴──┬───┴──────┴───────┘
                   │  agnostic-ai sync
                   ▼
   ┌──────────┬──────────┬──────────┬──────────┬──────────┐
   │ CLAUDE.md│ AGENTS.md│ GEMINI.md│  .cursor │   ...    │  ← per-target outputs
   │ .claude/ │          │          │  /rules/ │          │     (regenerated)
   │ .mcp.json│          │          │ mcp.json │          │
   └──────────┴──────────┴──────────┴──────────┴──────────┘
```

Write the rule, agent, skill, hook, or MCP server **once**. Every supported AI CLI reads the same contract.

## Standards this rides on

- **[`AGENTS.md`](https://agents.md/)**: open contract format. Originally OpenAI for Codex CLI, donated to the Linux Foundation in Dec 2025. Read natively by Codex, Cursor, Gemini CLI, GitHub Copilot, Aider, Cline, Windsurf, Continue, and others.
- **[`agentskills.io`](https://agentskills.io/specification)**: open skill spec from Anthropic, Dec 2025. Defines the minimal frontmatter (`name`, `description`) and progressive-disclosure loading.

The CLI emits to whichever conventions each target reads, anchored on these open standards.

## CI gate

See [CI](ci.md) for the workflow snippet and gating recipes.
