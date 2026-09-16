# agnostic-ai

**One spec. Every AI CLI.**

Write shared instructions, rules, skills, agents, hooks, and MCP configuration once. `agnostic-ai sync` writes the native files for the coding tools your team uses.

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Chemaclass/agnostic-ai?include_prereleases)](https://github.com/Chemaclass/agnostic-ai/releases)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/Chemaclass/agnostic-ai)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Set up with a coding agent

Paste this into Claude Code, Codex CLI, Cursor, or another coding agent:

```text
Set up agnostic-ai in this repository. Follow https://agnostic-ai.org/agent-setup.txt exactly. Preserve existing AI tool behavior, import native configuration before syncing, and finish with agnostic-ai sync --check. Summarize the targets selected and every file changed.
```

The [agent setup guide](https://agnostic-ai.org/docs/agent-setup/) explains the safety contract and each command. It keeps existing native configuration intact by importing before the first sync.

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

Already have tool configuration? Start with [importing an existing project](https://agnostic-ai.org/docs/migration/) to preserve your instructions.

Run `agnostic-ai update` to upgrade to the latest release. Use `--check` to see the detected install method without changing anything.

Model and CLI changes need more than a file diff. Configure your own test command, then let agnostic-ai pass it a stable identity for each harness:

```yaml
verify:
  command: [./scripts/verify-harness]
```

Run `agnostic-ai verify --target codex`. It first rejects stale generated files, then sends versioned JSON to the verifier through stdin. Your script owns the task, scoring, and pass or fail decision. See the [verification gate](https://agnostic-ai.org/docs/cli-reference/#verify).

Sync protects hand-authored ignore files with a conservative check of pattern order, negations, and whitespace. Run `agnostic-ai import <target>` to copy those patterns into specs, then review any conflicting patterns before syncing. See [ignore overwrite behavior](https://agnostic-ai.org/docs/spec-format/#overwrite-behaviour).

[Step-by-step tutorial](https://agnostic-ai.org/docs/getting-started/) · [More install options](https://agnostic-ai.org/docs/installation/) · [Try the playground](https://agnostic-ai.org/playground/)

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

Create directory-specific instructions with `agnostic-ai new rule payments-context --scope services/payments`. Sync preserves native scope across [supported tools](https://agnostic-ai.org/docs/scoped-context/), without adding those instructions to root context.

## Supported targets

Supports Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, and [all 25 targets](https://agnostic-ai.org/docs/targets/#capability-matrix). Support varies by spec kind. The target reference lists each tool's capabilities, output paths, and opt-in settings.

Native output includes Claude and Cursor prompt hooks, Goose hook plugins and review instructions, shared Goose/OpenHands project agents, Amp orb setup and supervised services, and ignore files for ten targets. Settings can share a default model across Claude, Codex, Copilot, OpenCode, Junie, Qoder, and Kilo, with Qoder also receiving shared permissions. Directory-scoped skills stay scoped on Codex, Cursor, Warp, and OpenCode. VS Code MCP sync retains `inputs`, `sandbox`, and other sibling settings.

The [AI tooling updates](https://agnostic-ai.org/updates/) archive filters complete release briefings by target and search terms. Filter state stays in the URL for bookmarks and sharing, while the full archive remains readable without JavaScript.

Gemini hook specs emit the native nested command format. Import preserves handler groups and millisecond timeouts. See [hook rendering](https://agnostic-ai.org/docs/spec-format/#per-target-rendering).

Use `targets:` in `agnostic-ai.yaml` to select the tools you need. For instructions shared across your own projects, see [global configuration](https://agnostic-ai.org/docs/configuration/#global-configuration).

## Find your next step

| I want to... | Read |
|---|---|
| Let a coding agent install and configure agnostic-ai | [Agent setup](https://agnostic-ai.org/docs/agent-setup/) |
| Sync my first rule | [Getting started](https://agnostic-ai.org/docs/getting-started/) |
| Bring existing tool config into one source | [Migration](https://agnostic-ai.org/docs/migration/) |
| Write a skill, agent, hook, or MCP spec | [Spec format](https://agnostic-ai.org/docs/spec-format/) |
| Set up or review agent context in any project | [Agent context skill](.agnostic-ai/skills/agent-context/SKILL.md) |
| Change targets or output paths | [Configuration](https://agnostic-ai.org/docs/configuration/) |
| Automate sync for a team | [CI](https://agnostic-ai.org/docs/ci/) and [Git hooks](https://agnostic-ai.org/docs/git-hooks/) |
| Share specs across repositories | [Packs](https://agnostic-ai.org/docs/packs/) |
| Diagnose missing or stale output | [Troubleshooting](https://agnostic-ai.org/docs/troubleshooting/) |
| Track upstream target changes and proposed support | [AI tooling updates](https://agnostic-ai.org/updates/) |
| Work on agnostic-ai | [Contributing](CONTRIBUTING.md) |

To use the agent context skill across projects, copy the
`.agnostic-ai/skills/agent-context/` directory to
`~/.agnostic-ai/skills/agent-context/` (or the equivalent under
`$AGNOSTIC_AI_HOME`), then run `agnostic-ai sync --global`. The skill works
without the project-local specialist agents, which global sync does not emit.

[All documentation](https://agnostic-ai.org/docs/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [Changelog](CHANGELOG.md)
