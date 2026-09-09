# Getting started

[User docs](README.md) · [Install the CLI](installation.md)

Create one rule and sync it to Claude Code and Cursor. This example uses explicit targets so the commands also work in a non-interactive shell.

Already have `CLAUDE.md`, `AGENTS.md`, or tool-specific configuration? Follow [Migration](migration.md) before syncing.

## Install

Follow [Installation](installation.md), then confirm `agnostic-ai --version` works.

## Scaffold

From the root of a project without existing tool configuration:

```bash
echo "claude,cursor" | agnostic-ai init
agnostic-ai new rule conventional-commits
```

`init` creates `agnostic-ai.yaml` and source folders under `.agnostic-ai/`. `new` writes `.agnostic-ai/rules/conventional-commits.md`.

For an interactive target picker, run `agnostic-ai init` without the pipe. Choose only the tools you use. `init --demo` adds sample specs; `init --preset go`, `ts-react`, or `python` adds stack-specific starters. See [init options](cli-reference.md#init).

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

The preview shows planned output. Sync writes it. The check exits successfully when files match the specs.

For this example, inspect:

| Output | Purpose |
|---|---|
| `.claude/rules/conventional-commits.md` | Claude Code rule |
| `.cursor/rules/conventional-commits.mdc` | Cursor rule |
| `CLAUDE.md` | Claude Code entry point that points back to the source specs |

Both rule files contain your commit convention. Edit the source file and run `sync` again to update them. Do not edit the generated copies.

To change tools later, edit `targets:` in `agnostic-ai.yaml`. See [target selection](targets.md#selecting-targets) for one-run filters and the first-sync picker.

## Commit or ignore generated outputs

`init` enables `gitignore.enabled` by default. Commit `.agnostic-ai/`, `agnostic-ai.yaml`, and `.gitignore`. The local `.agnostic-ai/.sync-state` cache and personal overrides stay ignored. Every fresh clone or worktree needs `agnostic-ai sync` to create its tool files.

To keep generated outputs in Git, set `gitignore.enabled: false` and remove their entries from the managed `.gitignore` block. For a new project, `init --gitignore=false` chooses this from the start. Commit the specs and generated files together, then use the [CI drift gate](ci.md#committed-outputs).

If outputs are ignored, CI should validate specs and generate files. It cannot compare a fresh checkout against files that were never committed. See [CI for ignored outputs](ci.md#ignored-outputs).

## Daily use

```bash
agnostic-ai sync --watch
```

Keep this running while editing specs; Ctrl+C stops it. Run `agnostic-ai status` for a summary of loaded specs, selected tools, and drift.

## Next steps

- [Spec format](spec-format.md): add skills, agents, hooks, and MCP servers.
- [Configuration](configuration.md): select tools and customize paths.
- [Git hooks](git-hooks.md): generate output when opening a fresh checkout.
- [Troubleshooting](troubleshooting.md): resolve missing files or sync failures.

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

These links keep previous guide sections easy to find:

- [Shell completion](installation.md#shell-completion)
- [Add a single spec](cli-reference.md#new)
- [Import an existing AI CLI config](migration.md)
- [Check project status](cli-reference.md#status)
- [Roll back a sync](migration.md#back-up-and-restore)
- [Watch mode](cli-reference.md#sync)
- [Auto-manage .gitignore](configuration.md#gitignore)
- [CI gate](ci.md)
