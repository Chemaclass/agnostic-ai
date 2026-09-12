# agnostic-ai

**One spec. Every AI CLI.**

Write shared instructions, rules, skills, agents, hooks, and MCP configuration once. `agnostic-ai sync` writes the native files for the coding tools your team uses.

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Chemaclass/agnostic-ai?include_prereleases)](https://github.com/Chemaclass/agnostic-ai/releases)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/Chemaclass/agnostic-ai)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Quickstart

Install on macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
```

On Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 | iex
```

In your project directory:

```bash
agnostic-ai init --demo
agnostic-ai sync
```

Choose your tools during setup. The demo creates sample specs under `.agnostic-ai/`; sync generates their native configuration. Edit the specs and sync again. Generated outputs are ignored by Git by default.

Already have tool configuration? Start with [importing an existing project](docs/user/migration.md) to preserve your instructions.

[Step-by-step tutorial](docs/user/getting-started.md) · [More install options](docs/user/installation.md) · [Try the playground](https://chemaclass.github.io/agnostic-ai/playground/)

## How it works

```text
.agnostic-ai/                 agnostic-ai sync        Native tool files
  AGNOSTIC_AI.md          ───────────────────────►      CLAUDE.md, AGENTS.md, ...
  rules/                                               .cursor/rules/, ...
  skills/                                              .claude/skills/, ...
  agents/, hooks/, mcps/, ...
```

Specs use Markdown with YAML frontmatter, or YAML for structured configuration. For example, `.agnostic-ai/rules/conventional-commits.md`:

```markdown
---
name: conventional-commits
description: Use Conventional Commits.
alwaysApply: true
---

Use feat:, fix:, docs:, refactor:, test:, or chore: prefixes.
Keep the subject under 72 characters.
```

Sync writes this rule to each selected tool's rules directory or includes it in the tool's instructions file. Edit the source spec, since generated files are overwritten on the next sync.

Create directory-specific instructions with `agnostic-ai new rule payments-context --scope services/payments`. Sync preserves native scope across [supported tools](docs/user/scoped-context.md), without adding those instructions to root context.

## Supported targets

Supports Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, and [all 25 targets](docs/user/targets.md#capability-matrix). Support varies by spec kind. The target reference lists each tool's capabilities, output paths, and opt-in settings.

Gemini hook specs emit the native nested command format. Import preserves handler groups and millisecond timeouts. See [hook rendering](docs/user/spec-format.md#per-target-rendering).

Use `targets:` in `agnostic-ai.yaml` to select the tools you need. For instructions shared across your own projects, see [global configuration](docs/user/configuration.md#global-configuration).

## Find your next step

| I want to... | Read |
|---|---|
| Sync my first rule | [Getting started](docs/user/getting-started.md) |
| Bring existing tool config into one source | [Migration](docs/user/migration.md) |
| Write a skill, agent, hook, or MCP spec | [Spec format](docs/user/spec-format.md) |
| Change targets or output paths | [Configuration](docs/user/configuration.md) |
| Automate sync for a team | [CI](docs/user/ci.md) and [Git hooks](docs/user/git-hooks.md) |
| Share specs across repositories | [Packs](docs/user/packs.md) |
| Diagnose missing or stale output | [Troubleshooting](docs/user/troubleshooting.md) |
| Work on agnostic-ai | [Contributing](CONTRIBUTING.md) |

[All documentation](docs/README.md) · [CLI reference](docs/user/cli-reference.md) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [Changelog](CHANGELOG.md)
