<div align="center">

# agnostic-ai

### **One spec. Every AI CLI. Zero drift.**

Write your agents, skills, rules, and hooks **once**. Ship them to Claude Code, Codex, Gemini, Cursor, Copilot, and 9 more, in their native format.

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/chemaclass/agnostic-ai)](https://goreportcard.com/report/github.com/chemaclass/agnostic-ai)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Chemaclass/agnostic-ai?include_prereleases)](https://github.com/Chemaclass/agnostic-ai/releases)

</div>

---

## Why

You write `CLAUDE.md`. Then `.cursor/rules`. Then `GEMINI.md`. Then `AGENTS.md`. Same content, four formats. Switch tools next quarter? Rewrite everything.

agnostic-ai keeps **one source of truth** in plain Markdown + YAML frontmatter, aligned with the [AGENTS.md](https://agents.md) open standard. Run `sync` and every tool gets the config it expects.

## How it works

<img width="1472" height="536" alt="image" src="https://github.com/user-attachments/assets/c065913b-67d9-4759-b584-56dca79a6a14" />


## See it work

Drop one file in `.agnostic-ai/rules/`:

```markdown
---
name: conventional-commits
description: Always use Conventional Commits format.
globs: "**/*"
alwaysApply: true
---

Use Conventional Commits for all commit messages.
Subject under 72 chars. Body explains why, not what.
```

Run `agnostic-ai sync`. Every target now has it, in its own native shape:

<table>
<tr>
<td>

**`CLAUDE.md`**
```markdown
## conventional-commits

Use Conventional Commits...
Subject under 72 chars...
```

</td>
<td>

**`.cursor/rules/conventional-commits.mdc`**
```markdown
---
description: Always use Conventional Commits.
globs: "**/*"
alwaysApply: true
---
Use Conventional Commits...
```

</td>
</tr>
<tr>
<td>

**`AGENTS.md`** (Codex)
```markdown
## Conventions
<!-- source: rules/conventional-commits.md -->
### conventional-commits
Use Conventional Commits...
```

</td>
<td>

**`.github/copilot-instructions.md`**
```markdown
### conventional-commits
Use Conventional Commits...
Subject under 72 chars...
```

</td>
</tr>
</table>

Same content. Each tool happy. No copy-paste, no drift.

## Supported targets

| Target         | Agents | Skills | Rules | Hooks | MCPs |
|----------------|:------:|:------:|:-----:|:-----:|:----:|
| Claude Code    |   ✅    |   ✅    |   ✅   |   ✅   |  ✅   |
| Codex CLI      |   ✅    |   ✅    |   ✅   |   -   |  -   |
| Gemini CLI     |   ◐    |   ◐    |   ✅   |   -   |  -   |
| Cursor         |   ✅    |   ✅    |   ✅   |   -   |  ✅   |
| GitHub Copilot |   ◐    |   ◐    |   ✅   |   -   |  ✅   |
| Aider          |   ◐    |   ◐    |   ✅   |   -   |  -   |
| Cline          |   ✅    |   ✅    |   ✅   |   -   |  -   |
| Windsurf       |   ✅    |   ✅    |   ✅   |   -   |  -   |
| Continue       |   ✅    |   ✅    |   ✅   |   -   |  -   |
| Amp            |   ◐    |   ◐    |   ✅   |   -   |  -   |
| Zed            |   ◐    |   ◐    |   ✅   |   -   |  -   |
| Warp           |   ◐    |   ◐    |   ✅   |   -   |  -   |
| OpenCode       |   ◐    |   ◐    |   ✅   |   -   |  -   |

Legend: ✅ separate files · ◐ merged into single doc · `-` not supported. Hooks are Claude-specific. MCPs propagate in each tool's native schema.

## Install

```bash
go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest
```

Or grab a prebuilt binary from the [latest release](https://github.com/Chemaclass/agnostic-ai/releases/latest). Homebrew and curl one-liner: [getting started](docs/user/getting-started.md).

## Quickstart

```bash
agnostic-ai init                  # scaffold .agnostic-ai/{agents,skills,rules,hooks,mcps}/
agnostic-ai init --demo           # same, plus one example spec per folder to learn from
agnostic-ai sync                  # emit native config for every target
agnostic-ai sync --dry-run        # preview without writing
agnostic-ai revert                # undo last sync
```

Already have `.cursor/rules` or `AGENTS.md`? Pull them in:

```bash
agnostic-ai import cursor         # also: codex, claude, cline, windsurf, continue
```

## What you get

- **Stateless adapters.** Same input, same output. Diffable, reviewable, regeneratable.
- **Provenance markers.** Every merged section carries `<!-- source: path -->` so you can trace any line back home.
- **Nested scoping.** `rules/backend/auth.md` routes to `backend/AGENTS.md`, `backend/.cursor/rules/`, etc.
- **Auto `.gitignore`.** Opt-in block listing every generated path. Re-runs are no-ops.
- **Backups & revert.** `--backup` writes `.bak` before overwrite. `revert` puts everything back.

## Documentation

- **[Getting started](docs/user/getting-started.md)** · first rule, first sync, in 2 minutes.
- **[Spec format](docs/user/spec-format.md)** · frontmatter reference for agents, skills, rules, hooks, MCPs.
- **[Targets](docs/user/targets.md)** · what each adapter emits and where.
- **[Configuration](docs/user/configuration.md)** · `agnostic.config.yaml` reference.
- **[CLI reference](docs/user/cli-reference.md)** · every flag, every command.
- **[Examples](docs/examples/)** · drop-in templates.
- **[Contributing](docs/internal/)** · architecture, adding adapters, release process.

---

<div align="center">

**Write the rule once. Every AI tool obeys it. Outlive the tool.**

</div>
