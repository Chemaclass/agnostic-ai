+++
title = "Getting started"
description = "Install agnostic-ai and sync one rule to two AI coding tools."
weight = 20

[extra]
group = "Start"
+++

# Getting started


Create one rule and sync it to Claude Code and Cursor. The explicit targets keep the commands working in a non-interactive shell.

Already have `CLAUDE.md`, `AGENTS.md`, or tool-specific configuration? Follow [Migration](@/docs/migration.md) before syncing.

## Install

Follow [Installation](@/docs/installation.md), then confirm `agnostic-ai --version` works.

## Scaffold

From a project root with no tool configuration:

```bash
echo "claude,cursor" | agnostic-ai init
agnostic-ai new rule conventional-commits
```

`init` creates `agnostic-ai.yaml` and source folders under `.agnostic-ai/`. `new` writes `.agnostic-ai/rules/conventional-commits.md`.

For an interactive target picker, run `agnostic-ai init` without the pipe and pick only the tools you use. `init --demo` adds sample specs; `init --preset go`, `ts-react`, or `python` adds stack-specific starters. See [init options](@/docs/cli-reference.md#init).

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
agnostic-ai sync --dry-run
agnostic-ai sync
agnostic-ai sync --check
```

`--dry-run` shows the planned output. `sync` writes it. `--check` exits zero when the files match the specs.

Inspect the output:

| Output | Purpose |
|---|---|
| `.claude/rules/conventional-commits.md` | Claude Code rule |
| `.cursor/rules/conventional-commits.mdc` | Cursor rule |
| `CLAUDE.md` | Claude Code entry point that points back to the source specs |

Both rule files contain your commit convention. Edit the source and run `sync` again; never edit the generated copies.

To change tools later, edit `targets:` in `agnostic-ai.yaml`. See [target selection](@/docs/targets/_index.md#selecting-targets) for one-run filters and the first-sync picker.

## Commit or ignore generated outputs

`init` enables `gitignore.enabled` by default. Commit `.agnostic-ai/`, `agnostic-ai.yaml`, and `.gitignore`. The local `.agnostic-ai/.sync-state` cache and personal overrides stay ignored. Every fresh clone or worktree needs `agnostic-ai sync` to create its tool files.

To keep generated outputs in Git, set `gitignore.enabled: false` and remove their entries from the managed `.gitignore` block. `init --gitignore=false` sets this from the start on a new project. Commit the specs and generated files together, then use the [CI drift gate](@/docs/ci.md#committed-outputs).

If outputs are ignored, CI validates the specs and generates the files. It cannot compare a fresh checkout against files that were never committed. See [CI for ignored outputs](@/docs/ci.md#ignored-outputs).

## Daily use

```bash
agnostic-ai sync --watch
```

Keep it running while you edit specs. Run `agnostic-ai status` for a summary of loaded specs, selected tools, and drift.

## Next steps

- [Directory-specific instructions](@/docs/scoped-context.md): keep service conventions inside their subtree.
- [Spec format](@/docs/spec-format.md): add skills, agents, hooks, and MCP servers.
- [Configuration](@/docs/configuration.md): select tools and change output paths.
- [Git hooks](@/docs/git-hooks.md): generate output on a fresh checkout.
- [Troubleshooting](@/docs/troubleshooting.md): fix missing files or failing syncs.

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

## More workflows

- [Shell completion](@/docs/installation.md#shell-completion)
- [Add a single spec](@/docs/cli-reference.md#new)
- [Import an existing AI CLI config](@/docs/migration.md)
- [Check project status](@/docs/cli-reference.md#status)
- [Roll back a sync](@/docs/migration.md#back-up-and-restore)
- [Watch mode](@/docs/cli-reference.md#sync)
- [Auto-manage .gitignore](@/docs/configuration.md#gitignore)
- [CI gate](@/docs/ci.md)
