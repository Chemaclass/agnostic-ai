# agnostic-ai

> **One source of truth for AI agents, skills, rules, and hooks. Transpile to every AI CLI you use.**

Write your prompts and project conventions **once**. agnostic-ai emits the right config for Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, Aider, Cline, Windsurf, Continue, and more.

```
                                          ┌──► .claude/agents/*.md
                                          ├──► CLAUDE.md
                                          ├──► AGENTS.md          (Codex)
   agents/                                 ├──► GEMINI.md
   skills/      agnostic-ai sync           ├──► .cursor/rules/*.mdc
   rules/      ─────────────────►          ├──► .github/copilot-instructions.md
   hooks/                                  ├──► CONVENTIONS.md    (Aider)
                                           ├──► .clinerules/*.md
                                           ├──► .windsurf/rules/*.md
                                           └──► .continue/rules/*.md
```

## The problem

Each AI CLI wants its own config:

- Claude Code: `CLAUDE.md` + `.claude/agents/*.md`
- Cursor: `.cursor/rules/*.mdc` with custom frontmatter
- Copilot: `.github/copilot-instructions.md`
- Codex: `AGENTS.md`
- Aider: `CONVENTIONS.md`

Copy-paste leads to drift. Teammates don't know which file is canonical.

## What agnostic-ai does

One source. Many outputs. The CLI walks your `agents/`, `skills/`, `rules/`, `hooks/` directories and emits whatever each tool expects.

```bash
agnostic-ai init       # scaffold
agnostic-ai sync       # emit configs for every enabled target
```

Change one rule in `rules/conventional-commits.md`. Run `sync`. Every tool sees the update.

## Supported targets

| Target          | Agents | Skills | Rules | Hooks |
|-----------------|:------:|:------:|:-----:|:-----:|
| Claude Code     | ✅     | ✅     | ✅    | ✅    |
| Codex CLI       | merged | -      | ✅    | -     |
| Gemini CLI      | merged | -      | ✅    | -     |
| Cursor          | ✅     | -      | ✅    | -     |
| GitHub Copilot  | merged | -      | ✅    | -     |
| Aider           | merged | -      | ✅    | -     |
| Cline           | ✅     | -      | ✅    | -     |
| Windsurf        | ✅     | -      | ✅    | -     |
| Continue        | ✅     | -      | ✅    | -     |

Full breakdown: [docs/user/targets.md](docs/user/targets.md).

## Install

**Homebrew (mac/linux):**
```bash
brew install chemaclass/tap/agnostic-ai
```

**Direct binary:**
```bash
curl -fsSL https://github.com/Chemaclass/agnostic-ai/releases/latest/download/agnostic-ai-$(uname -s)-$(uname -m) \
  -o /usr/local/bin/agnostic-ai && chmod +x /usr/local/bin/agnostic-ai
```

**From source:**
```bash
go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest
```

## Quickstart

```bash
mkdir my-agents && cd my-agents
agnostic-ai init           # creates agents/, skills/, rules/, hooks/, agnostic.config.yaml
```

Add a rule (`rules/conventional-commits.md`):

```markdown
---
name: conventional-commits
description: Always use Conventional Commits.
alwaysApply: true
---

Use `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:` prefixes.
Subject line under 72 chars. Body explains why, not what.
```

Sync:

```bash
agnostic-ai sync
```

Resulting tree:

```
my-agents/
├── agnostic.config.yaml
├── rules/
│   └── conventional-commits.md
├── CLAUDE.md                                    # for Claude Code
├── AGENTS.md                                    # for Codex
├── GEMINI.md                                    # for Gemini CLI
├── CONVENTIONS.md                               # for Aider
├── .github/copilot-instructions.md              # for Copilot
├── .cursor/rules/conventional-commits.mdc       # for Cursor
├── .clinerules/conventional-commits.md          # for Cline
├── .windsurf/rules/conventional-commits.md      # for Windsurf
└── .continue/rules/conventional-commits.md      # for Continue
```

## Spec format

Markdown body + YAML frontmatter (hooks are pure YAML), shared across all four kinds.

Full reference: [docs/user/spec-format.md](docs/user/spec-format.md).

## Commands

```bash
agnostic-ai init                  # scaffold a project
agnostic-ai sync                  # emit all configured targets
agnostic-ai sync -t claude,cursor # only specific targets
agnostic-ai sync --dry-run        # preview without writing
agnostic-ai validate              # parse-check all specs
agnostic-ai list                  # show loaded specs
```

Full reference: [docs/user/cli-reference.md](docs/user/cli-reference.md).

## Documentation

- **Users:** [docs/user/](docs/user/): getting started, spec format, targets, configuration, CLI reference
- **Contributors:** [docs/internal/](docs/internal/): architecture, adding adapters, contributing, release process, decision log

## Project layout

Entry point in `cmd/agnostic-ai/`. Core packages under `internal/` (cli, config, spec, adapters). Examples in `examples/`, docs in `docs/`.

## Status

Pre-1.0. Spec format may change between minor versions; breaking changes go in `CHANGELOG.md`.

## Contributing

PRs welcome, especially new adapters. Read [docs/internal/contributing.md](docs/internal/contributing.md) and [docs/internal/adding-adapters.md](docs/internal/adding-adapters.md) first.

## License

MIT. See [LICENSE](LICENSE).
