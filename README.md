# agnostic-ai

> **One source of truth for AI agents, skills, rules, and hooks. Transpile to every AI CLI you use.**

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/chemaclass/agnostic-ai)](https://goreportcard.com/report/github.com/chemaclass/agnostic-ai)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Chemaclass/agnostic-ai?include_prereleases)](https://github.com/Chemaclass/agnostic-ai/releases)

Write prompts and project conventions **once**. agnostic-ai emits the right config for Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, Aider, Cline, Windsurf, Continue, and more.

```
                                          ┌──► .claude/agents/*.md
                                          ├──► CLAUDE.md
                                          ├──► AGENTS.md          (Codex)
   agents/                                ├──► GEMINI.md
   skills/      agnostic-ai sync          ├──► .cursor/rules/*.mdc
   rules/      ─────────────────►         ├──► .github/copilot-instructions.md
   hooks/                                 ├──► CONVENTIONS.md    (Aider)
                                          ├──► .clinerules/*.md
                                          ├──► .windsurf/rules/*.md
                                          └──► .continue/rules/*.md
```

## Why

Each AI CLI wants its own config (`CLAUDE.md`, `.cursor/rules/*.mdc`, `AGENTS.md`, `CONVENTIONS.md`, ...). Copy-paste drifts. agnostic-ai keeps one source under `agents/`, `skills/`, `rules/`, `hooks/` and emits whatever each tool expects. Edit once, run `sync`, every tool updates.

## Supported targets

| Target         | Agents | Skills | Rules | Hooks |
|----------------|:------:|:------:|:-----:|:-----:|
| Claude Code    | ✅     | ✅     | ✅    | ✅    |
| Codex CLI      | ◐      | ◐      | ✅    | —     |
| Gemini CLI     | ◐      | ◐      | ✅    | —     |
| Cursor         | ✅     | ✅     | ✅    | —     |
| GitHub Copilot | ◐      | ◐      | ✅    | —     |
| Aider          | ◐      | ◐      | ✅    | —     |
| Cline          | ✅     | ✅     | ✅    | —     |
| Windsurf       | ✅     | ✅     | ✅    | —     |
| Continue       | ✅     | ✅     | ✅    | —     |

Legend: ✅ separate files · ◐ merged into single doc · — not supported.

Hooks are Claude-specific (lifecycle events with shell commands). Other targets have no equivalent concept.

Details: [docs/user/targets.md](docs/user/targets.md). Adding a target: [docs/internal/adding-adapters.md](docs/internal/adding-adapters.md).

## Install

**Prebuilt binary** (no Go required):

Grab the archive for your OS/arch from the [latest release](https://github.com/Chemaclass/agnostic-ai/releases/latest), extract `agnostic-ai`, and put it on your `PATH`.

**Go install:**

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

## Build from source

Requires Go 1.23+.

```bash
git clone https://github.com/Chemaclass/agnostic-ai
cd agnostic-ai
make build    # ./agnostic-ai
make test     # unit + integration
```

## Documentation

- Users: [docs/user/](docs/user/): getting started, spec format, targets, config, CLI reference.
- Contributors: [docs/internal/](docs/internal/): architecture, adapters, release process.
- Examples: [docs/examples/](docs/examples/).

## Status

Pre-1.0. Spec format may change between minor versions. See [CHANGELOG.md](CHANGELOG.md).

## Contributing

PRs welcome, especially new adapters. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT. See [LICENSE](LICENSE).
