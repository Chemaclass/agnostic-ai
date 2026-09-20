# agnostic-ai

**One spec. Every AI CLI.**

Write shared instructions, rules, skills, agents, hooks, and MCP configuration once. `agnostic-ai sync` writes the native files for the coding tools your team uses.

## Why agnostic-ai

Every AI coding tool ships its own config file. Adopt three and your instructions live in three places. They drift. Switching tools means rewriting work you already did. That is lock-in, delivered one config file at a time.

agnostic-ai makes that configuration yours. One source in your repository, plain Markdown and YAML. Every tool reads a generated copy. Adding a tool costs nothing. Dropping one costs nothing.

- **You own the source.** Plain files in your repo. No account, no database, no service.
- **Generated files are outputs.** Never a second source of truth. `sync` overwrites them, `sync --check` proves it.
- **No tool is privileged.** Adding a target never changes what the others get.
- **A sync layer, not a platform.** agnostic-ai should be easy to stop using.

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Chemaclass/agnostic-ai?include_prereleases)](https://github.com/Chemaclass/agnostic-ai/releases)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/Chemaclass/agnostic-ai)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Get started

### With a coding agent

Paste this into Claude Code, Codex CLI, Cursor, or another coding agent:

```text
Set up agnostic-ai in this repository. Follow https://agnostic-ai.org/agent-setup.txt exactly. Preserve existing AI tool behavior, import native configuration before syncing, and finish with agnostic-ai sync --check. Summarize the targets selected and every file changed.
```

The [agent setup guide](https://agnostic-ai.org/docs/agent-setup/) explains the safety contract and each command.

### Manually

Install the CLI with whichever route fits the machine:

| Platform | Route | Command |
|---|---|---|
| macOS and Linux | Homebrew | `brew install --cask Chemaclass/tap/agnostic-ai` |
| Any platform with Node 18 or newer | npm | `npm install -g agnostic-ai` |
| macOS and Linux without Homebrew | install script | `curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh \| bash` |

Windows, Go, and manual download are in [all install options](https://agnostic-ai.org/docs/installation/). Then run:

```console
agnostic-ai init --demo
agnostic-ai sync
```

Choose your tools during setup. The demo creates sample specs under `.agnostic-ai/`, and `sync` writes their native configuration. Already have tool configuration? Follow the [migration guide](https://agnostic-ai.org/docs/migration/) before the first sync.

[Follow the tutorial](https://agnostic-ai.org/docs/getting-started/) · [Try the playground](https://agnostic-ai.org/playground/)

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

Sync writes the rule to each selected tool's native location. Edit the source spec, then sync again. The [spec format](https://agnostic-ai.org/docs/spec-format/) covers rules, skills, agents, hooks, MCP servers, commands, settings, reviews, environments, and ignore files.

Three of those ten do more than carry instructions. A [review](https://agnostic-ai.org/docs/spec-format/#reviews) spec is guidance for a code-review bot, which Cursor Bugbot and Goose read. An [environment](https://agnostic-ai.org/docs/spec-format/#environments) spec says how an agent boots your dev environment, for Cursor, Amp, and OpenHands. An [ignore](https://agnostic-ai.org/docs/spec-format/#ignore) spec holds gitignore-syntax patterns an agent must not read or index, and writes a native exclusion file for ten targets.

## Supported targets

agnostic-ai supports Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, and [20 more targets](https://agnostic-ai.org/docs/targets/#capability-matrix). Support varies by spec kind. The target reference lists every capability, native path, and opt-in setting.

The [AI tooling updates](https://agnostic-ai.org/updates/) explain important upstream CLI and model changes, their developer impact, and the current agnostic-ai support state.

## Find your next step

| I want to... | Read |
|---|---|
| Let a coding agent install and configure agnostic-ai | [Agent setup](https://agnostic-ai.org/docs/agent-setup/) |
| Install or upgrade the CLI | [Installation](https://agnostic-ai.org/docs/installation/) |
| Sync my first rule | [Getting started](https://agnostic-ai.org/docs/getting-started/) |
| Bring existing tool config into one source | [Migration](https://agnostic-ai.org/docs/migration/) |
| Write a skill, agent, hook, or MCP spec | [Spec format](https://agnostic-ai.org/docs/spec-format/) |
| Keep an agent out of build output and secrets | [Ignore specs](https://agnostic-ai.org/docs/spec-format/#ignore) |
| Change targets or output paths | [Configuration](https://agnostic-ai.org/docs/configuration/) |
| Automate sync for a team | [CI](https://agnostic-ai.org/docs/ci/) and [Git hooks](https://agnostic-ai.org/docs/git-hooks/) |
| Run a project-owned harness test | [Verification gate](https://agnostic-ai.org/docs/cli-reference/#verify) |
| Add directory-specific instructions | [Scoped context](https://agnostic-ai.org/docs/scoped-context/) |
| Share specs across repositories | [Packs](https://agnostic-ai.org/docs/packs/) |
| Diagnose missing or stale output | [Troubleshooting](https://agnostic-ai.org/docs/troubleshooting/) |
| Track upstream target changes and proposed support | [AI tooling updates](https://agnostic-ai.org/updates/) |
| Work on agnostic-ai | [Contributing](CONTRIBUTING.md) |

[All documentation](https://agnostic-ai.org/docs/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [Changelog](CHANGELOG.md)
