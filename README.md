# agnostic-ai

**One source to rule them all.** Write agents, skills, rules, hooks, and MCP configuration once. `agnostic-ai sync` turns those specs into native files for the AI coding tools you use.

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/agnostic-ai?logo=npm&label=npm)](https://www.npmjs.com/package/agnostic-ai)
[![Homebrew](https://img.shields.io/badge/Homebrew-Chemaclass%2Ftap-FBB040?logo=homebrew&logoColor=111)](https://github.com/Chemaclass/homebrew-tap/blob/master/Casks/agnostic-ai.rb)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)

AI tools store instructions in different files. Keeping those files by hand makes them drift. agnostic-ai keeps the editable source in plain Markdown and YAML in your repository. It needs no account or service.

## Set up with a coding agent

Paste this into Claude Code, Codex, Cursor, or another coding agent:

```text
Set up agnostic-ai in this repository. Follow https://agnostic-ai.org/agent-setup.txt exactly. Preserve existing AI tool behavior, import native configuration before syncing, and finish with agnostic-ai sync --check. Summarize the targets selected and every file changed.
```

The [agent setup guide](https://agnostic-ai.org/docs/agent-setup/) explains each step.

## Set up manually

Install with Homebrew on macOS or Linux, or npm on a machine with Node 18 or newer:

```bash
brew install --cask Chemaclass/tap/agnostic-ai
# or
npm install -g agnostic-ai
```

See [all install options](https://agnostic-ai.org/docs/installation/) for Windows, Go, and direct downloads. If the project already has native AI tool files, follow the [migration guide](https://agnostic-ai.org/docs/migration/) before the first sync.

```bash
agnostic-ai init
agnostic-ai new rule team-conventions
# edit .agnostic-ai/rules/team-conventions.md
agnostic-ai sync
agnostic-ai sync --check
```

`init` selects your tools. `new` creates your first rule under `.agnostic-ai/`; replace its TODO text before syncing. Generated files such as `CLAUDE.md`, `AGENTS.md`, and `.cursor/rules/` are outputs. Keep the specs as your source of truth.

Use `agnostic-ai sync --global` for [user-level configuration](https://agnostic-ai.org/docs/configuration/#global-configuration). Global hooks and skills honor each spec's target filters. Skill metadata renders per target, while shared directories stay neutral. Agents with `readonly: true` use Cursor's read-only mode or Codex's read-only sandbox.

Keep [personal overrides](https://agnostic-ai.org/docs/local-overrides/) in `.agnostic-ai.local/` for one project, ignored by default, or in `~/.agnostic-ai/local/` for every project. `agnostic-ai list` and `list --global` show which layer supplies each spec.

## Daily commands

```bash
agnostic-ai import claude codex --dry-run --diff  # preview existing tool config
agnostic-ai compare claude cursor                # see what each tool keeps or drops
agnostic-ai why AGENTS.md                        # trace an output to its source
agnostic-ai sync --check                         # find local drift
```

Support spans [Claude Code, Codex, Cursor, Gemini CLI, Copilot, and more](https://agnostic-ai.org/docs/targets/#capability-matrix). Each tool supports a different set of spec kinds. The [spec format](https://agnostic-ai.org/docs/spec-format/) and [target reference](https://agnostic-ai.org/docs/targets/) show the exact paths and fields.

## Develop agnostic-ai

```bash
make tools      # install pinned development tools once
make build
make preflight  # format, lint, and Go tests
```

This repository keeps its own agent setup in `.agnostic-ai/`. Edit those source specs, then run `./agnostic-ai sync`. Most native output is ignored by Git; `.openhands/setup.sh` is tracked for bootstrap. See [CONTRIBUTING.md](CONTRIBUTING.md) for checks by change type and the [architecture guide](docs/internal/architecture.md) for the Go packages.

[Getting started](https://agnostic-ai.org/docs/getting-started/) · [Playground](https://agnostic-ai.org/playground/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Changelog](CHANGELOG.md)
