+++
title = "Use Composer with Claude Code and Codex"
description = "Keep one set of project instructions, rules, and skills for Composer's Claude and Codex providers."
weight = 72

[extra]
nav_title = "Composer providers"
group = "Workflows"
+++

# Use Composer with Claude Code and Codex

[Composer](https://getcomposer.dev/) runs coding sessions through Claude Code or Codex. Select both providers in agnostic-ai to share project instructions, a standing rule, and a reusable skill. Composer passes the project directory to the selected provider, which loads its native files.

Verified against [Composer CLI 0.1.12 source](https://github.com/atemaguer/composer/tree/cli-v0.1.12). Release downloads were unavailable, so packaged runtime behavior and authenticated sessions were not tested.

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

Review the imported `.agnostic-ai/` sources against the original files and resolve differences before syncing. Skip this step in a fresh project. See [how imports from several tools merge instructions](@/docs/migration.md#2-review).

## Write the shared sources

Create the source folders:

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

The first sync can replace imported files after you reconcile their instructions. Use `--backup` to keep the originals as `.bak` files.

```bash
agnostic-ai lint
agnostic-ai sync --plan --backup
```

Review the plan, then generate and check:

```bash
agnostic-ai sync --backup
agnostic-ai sync --check
```

Inspect the outputs for the note, rule, and skill:

| Provider | Files to inspect |
|---|---|
| Claude Code | `CLAUDE.md`, `.claude/rules/commit-subjects.md`, `.claude/skills/change-summary/SKILL.md` |
| Codex | `AGENTS.md`, `.agents/skills/change-summary/SKILL.md` |

Find the shared note in both `CLAUDE.md` and `AGENTS.md`. Codex's `AGENTS.md` also contains the rule; Claude Code gets it in `.claude/rules/commit-subjects.md`.

For later changes, edit `.agnostic-ai/` and sync again. Home instructions and skills use [global configuration](@/docs/global-configuration.md). Additional settings, hooks, agents, and MCP servers keep their [Claude Code](@/docs/targets/claude-reference.md) or [Codex](@/docs/targets/codex-reference.md) mappings.

## Check each provider in Composer

Run these from the project root after syncing. Each starts a new session with an explicit provider:

```bash
composer run --provider claude --permission-mode default 'Use the change-summary skill for this supplied change: a broken documentation link was fixed. Read the project instructions first. Do not edit files.'
composer run --provider codex --permission-mode default 'Use the change-summary skill for this supplied change: a broken documentation link was fixed. Read the project instructions first. Do not edit files.'
```

Check for a `fix:` or `docs:` subject, the skill's three steps, and the inspected filenames. Compare those paths with the generated files above. `sync --check` proves that files match their sources; the session check tests whether the installed provider loads and follows them.

Composer's [run command](https://github.com/atemaguer/composer/blob/c4f04d94839a666f21548616b220fa8eb17a503f/apps/cli/src/index.ts#L552) defaults to Full access in 0.1.12. Its runtime permission mode can override provider configuration. Keep `--permission-mode default` for these checks. A noninteractive run stops if approval is required; use `composer tui` and review the provider and permission mode there to answer approvals.

Composer's [skill catalog](https://github.com/atemaguer/composer/blob/c4f04d94839a666f21548616b220fa8eb17a503f/packages/composer-runtime/src/capabilities.ts#L24) scans global provider skill folders and the Codex plugin cache, not project skill folders. [Selecting a skill in the TUI](https://github.com/atemaguer/composer/blob/c4f04d94839a666f21548616b220fa8eb17a503f/apps/cli/src/tui/components/dialogs/DialogActions.tsx#L87) displays a notice rather than invoking it. Ask the active provider to use the generated project skill, as above.
