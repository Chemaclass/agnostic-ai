+++
title = "Getting started"
description = "Install agnostic-ai and sync one rule to two AI coding tools."
weight = 20

[extra]
group = "Start"
+++

# Getting started

Write one rule and sync it to Claude Code and Cursor.

Already use one tool and want another? Run `agnostic-ai use codex` (see [use](@/docs/cli-reference/start.md#use)). Already have `CLAUDE.md`, `AGENTS.md`, or other tool files? Follow [Migration](@/docs/migration.md) first.

## Install

Follow [Installation](@/docs/installation.md), then check that `agnostic-ai --version` works.

## Scaffold

From a project root with no tool configuration:

```bash
echo "claude,cursor" | agnostic-ai init
agnostic-ai new rule conventional-commits
```

`init` creates `agnostic-ai.yaml`. `new` creates `.agnostic-ai/rules/conventional-commits.md`. Run `init` without the pipe to pick tools interactively. See [init options](@/docs/cli-reference/start.md#init) for `--demo` and stack presets.

## First rule

Replace `.agnostic-ai/rules/conventional-commits.md` with:

```markdown
---
name: conventional-commits
description: Use Conventional Commits.
alwaysApply: true
---

Use feat:, fix:, docs:, refactor:, test:, or chore: prefixes.
Keep the subject under 72 characters.
```

## Sync

```bash
agnostic-ai sync --dry-run   # preview
agnostic-ai sync             # write
agnostic-ai sync --check     # exit 0 when outputs match the specs
```

Sync writes:

| Output | Purpose |
|---|---|
| `.claude/rules/conventional-commits.md` | Claude Code rule |
| `.cursor/rules/conventional-commits.mdc` | Cursor rule |
| `.agnostic-ai/AGNOSTIC_AI.md` | Shared project instructions (you edit this) |
| `CLAUDE.md` | Claude Code entry point, copied from `AGNOSTIC_AI.md` |

Edit sources under `.agnostic-ai/`, then sync again. Never edit the generated copies. To change tools, edit `targets:` in `agnostic-ai.yaml` ([target selection](@/docs/configuration.md#targets)).

## Commit or ignore generated outputs

`init` asks. Without a terminal, or with `--all`, it ignores them.

- **Ignore (default):** Git holds only `.agnostic-ai/`, `agnostic-ai.yaml`, and `.gitignore`. Every teammate installs the tool and runs `agnostic-ai sync` after cloning. [Checkout hooks](@/docs/git-hooks.md#regenerate-on-checkout) can run it for them. CI [validates and generates](@/docs/ci.md#ignored-outputs) instead of comparing.
- **Commit (`init --gitignore=off`):** a clone works without the tool. Add the [CI drift gate](@/docs/ci.md#committed-outputs) to keep files current. To switch an existing project, see [gitignore](@/docs/configuration.md#gitignore).

## Daily use

Run `agnostic-ai sync --watch` while you edit specs. Run `agnostic-ai status` for loaded specs, tools, and drift.

<a id="shell-completion"></a>
<a id="add-a-single-spec"></a>
<a id="import-an-existing-ai-cli-config"></a>
<a id="recommended-adoption-workflow"></a>
<a id="what-import-does-not-capture"></a>
<a id="agnostic-aisync-state"></a>
<a id="check-project-status"></a>
<a id="roll-back-a-sync"></a>
<a id="watch-mode"></a>
<a id="auto-manage-gitignore"></a>
<a id="ci-gate"></a>
<a id="upgrade"></a>
<a id="inside-claude-code"></a>

## Next steps

- [Directory-specific instructions](@/docs/scoped-context.md): keep service conventions inside their subtree.
- [Spec format](@/docs/spec-format/_index.md): add skills, agents, hooks, and MCP servers.
- [Configuration](@/docs/configuration.md): select tools and change output paths.
- [Git hooks](@/docs/git-hooks.md) and [CI gate](@/docs/ci.md): keep outputs current.
- [Troubleshooting](@/docs/troubleshooting.md): fix missing files or failing syncs.
