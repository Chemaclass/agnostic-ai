+++
title = "Use Composer with Claude Code and Codex"
description = "Keep one set of project instructions, rules, and skills for Composer's Claude and Codex providers."
weight = 76

[extra]
nav_title = "Composer providers"
group = "Workflows"
+++

# Use Composer with Claude Code and Codex

[Composer](https://getcomposer.dev/) runs sessions through Claude Code or Codex. Select both targets to share instructions, rules, and skills; the provider loads its native files from the project directory.

## Prepare the project

[Install agnostic-ai](@/docs/installation.md). Install Composer through its [official project](https://github.com/atemaguer/composer#readme), then install and sign in to the providers you use: [Claude Code](@/docs/targets/claude.md#first-rule) and [Codex](@/docs/targets/codex.md#first-rule).

Work from your project root. Create `agnostic-ai.yaml`, or add both targets to your existing list:

```yaml
targets: [claude, codex]
```

### Keep existing provider files

Before the first sync, import any existing Claude Code or Codex configuration:

```bash
agnostic-ai import claude codex --dry-run --diff
agnostic-ai import claude codex
```

Compare the imported sources with the originals and resolve differences before syncing. Skip in a fresh project. See [how imports merge](@/docs/migration.md#2-review).

## Write the shared sources

```bash
mkdir -p .agnostic-ai/rules .agnostic-ai/skills
```

Add a project note to `.agnostic-ai/AGNOSTIC_AI.md`, keeping imported text:

```markdown
When summarizing a change, name the files you inspected.
```

Save this rule as `.agnostic-ai/rules/commit-subjects.md`:

```markdown
---
name: commit-subjects
description: Use the team's commit subject prefixes.
alwaysApply: true
---

Start suggested commit subjects with fix:, feat:, docs:, or chore:.
```

Save this skill as `.agnostic-ai/skills/change-summary.md`:

```markdown
---
name: change-summary
description: Summarize a supplied change. Use when asked for a change summary.
---

1. State the effect a user sees.
2. Suggest a commit subject.
3. List the checks performed, or say that no checks were run.
```

For more fields, use the [rules](@/docs/spec-format/rules.md) and [skills](@/docs/spec-format/skills.md) references.

## Generate and inspect

The first sync can replace imported files. `--backup` keeps the originals as `.bak` files.

```bash
agnostic-ai lint
agnostic-ai sync --plan --backup
```

Review the plan, then generate and check:

```bash
agnostic-ai sync --backup
agnostic-ai sync --check
```

| Provider | Files to inspect |
|---|---|
| Claude Code | `CLAUDE.md`, `.claude/rules/commit-subjects.md`, `.claude/skills/change-summary/SKILL.md` |
| Codex | `AGENTS.md`, `.agents/skills/change-summary/SKILL.md` |

The note is in both `CLAUDE.md` and `AGENTS.md`; Codex gets the rule inside `AGENTS.md`. Later: edit `.agnostic-ai/` and sync. Home-level files use [user configuration](@/docs/global-configuration.md). Settings, hooks, agents, and MCP servers keep their [Claude Code](@/docs/targets/claude-reference.md) or [Codex](@/docs/targets/codex-reference.md) mappings.

## Check each provider in Composer

From the project root, start one session per provider:

```bash
composer run --provider claude --permission-mode default 'Use the change-summary skill for this supplied change: a broken documentation link was fixed. Read the project instructions first. Do not edit files.'
composer run --provider codex --permission-mode default 'Use the change-summary skill for this supplied change: a broken documentation link was fixed. Read the project instructions first. Do not edit files.'
```

Each answer should have a `fix:` or `docs:` subject, the skill's three steps, and inspected filenames that match the generated files above. `sync --check` proves the files match their sources; this check proves the provider loads and follows them. Keep `--permission-mode default` (see the details below).

<details>
<summary>Composer 0.1.12 behavior and what was checked</summary>

- Checked against the [Composer CLI 0.1.12 source](https://github.com/atemaguer/composer/tree/cli-v0.1.12). Release downloads were unavailable, so packaged runtime behavior and signed-in sessions were not tested.
- Composer's [run command](https://github.com/atemaguer/composer/blob/c4f04d94839a666f21548616b220fa8eb17a503f/apps/cli/src/index.ts#L552) defaults to Full access in 0.1.12, and its permission mode can override provider configuration. Keep `--permission-mode default`.
- A noninteractive run stops when approval is required. To answer approvals, use `composer tui` and review the provider and permission mode there.
- Composer's [skill catalog](https://github.com/atemaguer/composer/blob/c4f04d94839a666f21548616b220fa8eb17a503f/packages/composer-runtime/src/capabilities.ts#L24) scans global provider skill folders and the Codex plugin cache, not project skill folders.
- [Selecting a skill in the TUI](https://github.com/atemaguer/composer/blob/c4f04d94839a666f21548616b220fa8eb17a503f/apps/cli/src/tui/components/dialogs/DialogActions.tsx#L87) shows a notice instead of running it. Ask the provider to use the project skill, as above.

</details>
