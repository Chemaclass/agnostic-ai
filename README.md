# agnostic-ai

> **One source of truth for AI agents, skills, rules, and hooks. Transpile to every AI CLI you use.**

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/chemaclass/agnostic-ai)](https://goreportcard.com/report/github.com/chemaclass/agnostic-ai)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Chemaclass/agnostic-ai?include_prereleases)](https://github.com/Chemaclass/agnostic-ai/releases)

Write prompts and project conventions **once**. agnostic-ai emits the right config for Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, Aider, Cline, Windsurf, Continue, Amp, Zed, Warp, OpenCode, and more.

Aligned with the [AGENTS.md](https://agents.md) open standard. Markdown body + YAML frontmatter, no proprietary extensions.

```
                                  ┌──► .claude/agents/*.md
                                  ├──► CLAUDE.md
                                  ├──► AGENTS.md            (Codex)
                                  ├──► .codex/agents/*.toml (Codex subagents)
   .agnostic-ai/agents/           ├──► GEMINI.md
   .agnostic-ai/skills/  sync     ├──► .cursor/rules/*.mdc
   .agnostic-ai/rules/   ───────► ├──► .github/copilot-instructions.md
   .agnostic-ai/hooks/            ├──► CONVENTIONS.md      (Aider)
                                  ├──► .clinerules/*.md
                                  ├──► .windsurf/rules/*.md
                                  └──► .continue/rules/*.md
```

## Supported targets

| Target         | Agents | Skills | Rules | Hooks | MCPs |
|----------------|:------:|:------:|:-----:|:-----:|:----:|
| Claude Code    |   ✅    |   ✅    |   ✅   |   ✅   |  ✅   |
| Codex CLI      |   ✅    |   ◐    |   ✅   |   -   |  -   |
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

Legend: ✅ separate files · ◐ merged into single doc · `-` not supported. Hooks are Claude-specific. MCP propagation lands in the schema each tool understands.

## Install

Prebuilt binary from the [latest release](https://github.com/Chemaclass/agnostic-ai/releases/latest), or:

```bash
go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest
```

More options (Homebrew, curl one-liner): [docs/user/getting-started.md](docs/user/getting-started.md).

## Quickstart

```bash
agnostic-ai init    # scaffold .agnostic-ai/{agents,skills,rules,hooks,mcps}/
agnostic-ai sync    # emit configs for every target
```

Walkthrough with a first rule: [docs/user/getting-started.md](docs/user/getting-started.md).

## Documentation

- [User docs](docs/user/): getting started, spec format, targets, config, CLI reference.
- [Contributor docs](docs/internal/): architecture, adding adapters, release process, roadmap.
- [Examples](docs/examples/).

---

> Write the rule once. Every AI tool obeys it. Outlive the tool.
