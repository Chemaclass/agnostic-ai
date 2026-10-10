+++
title = "Getting started"
description = "Install agnostic-ai, import existing tool files, preview the changes, and sync."
weight = 20

[extra]
group = "Start"
+++

# Getting started

Install agnostic-ai and bring your AI coding tool setup into one source directory.

<a id="install"></a>
<a id="scaffold"></a>
<a id="sync"></a>

## Quickstart

From your project root, with Node 18 or newer:

```bash
npm install -g agnostic-ai
agnostic-ai init --from all
agnostic-ai sync --plan
agnostic-ai sync
```

`init --from all` creates `agnostic-ai.yaml` and imports any existing tool files into `.agnostic-ai/`. Pick the tools you use when prompted. Without a terminal, init selects the tools it detects, or its default set when it finds none. `sync --plan` previews the changes. `sync` writes each tool's own files.

For other installers, see [Installation](@/docs/installation.md). To review an existing setup before generating files, see [Migration](@/docs/migration.md). Already use one tool and want another? Run `agnostic-ai use codex` (see [use](@/docs/cli-reference/start.md#use)).

## First rule

Add a rule to the setup:

```bash
agnostic-ai new rule conventional-commits
```

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

Run `agnostic-ai sync` again. With Claude Code and Cursor selected, the rule lands in:

| Output | Purpose |
|---|---|
| `.claude/rules/conventional-commits.md` | Claude Code rule |
| `.cursor/rules/conventional-commits.mdc` | Cursor rule |

Write shared project instructions in `.agnostic-ai/AGNOSTIC_AI.md`. Sync creates this file once and keeps your edits. Edit sources under `.agnostic-ai/`, then sync again. To change tools, edit `targets:` in `agnostic-ai.yaml` ([target selection](@/docs/configuration.md#targets)).

## Commit or ignore generated outputs

`init` asks. Without a terminal, or with `--all`, it ignores them.

- **Ignore (default):** Git holds only `.agnostic-ai/`, `agnostic-ai.yaml`, and `.gitignore`. Every teammate installs the tool and runs `agnostic-ai sync` after cloning. [Checkout hooks](@/docs/git-hooks.md#regenerate-on-checkout) can run it for them. CI [validates and generates](@/docs/ci.md#ignored-outputs) instead of comparing.
- **Commit (`init --gitignore=off`):** a clone works without the tool. Add the [CI drift gate](@/docs/ci.md#committed-outputs) to keep files current. To switch an existing project, see [gitignore](@/docs/configuration.md#gitignore).

## Daily use

Run `agnostic-ai sync --watch` while you edit specs. Run `agnostic-ai status` to see loaded specs, tools, and drift. Run `agnostic-ai sync --check` after a sync or in CI to catch drift.

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
- [MCP recipes](@/docs/spec-format/mcp-recipes.md): connect GitHub, Context7, Playwright, or the filesystem.
- [Configuration](@/docs/configuration.md): select tools and change output paths.
- [Git hooks](@/docs/git-hooks.md) and [CI gate](@/docs/ci.md): keep outputs current.
- [Troubleshooting](@/docs/troubleshooting.md): fix missing files or failing syncs.
