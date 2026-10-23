+++
title = "Use Composer with Claude Code and Codex"
description = "Keep one set of project instructions, rules, and skills for Composer's Claude and Codex providers."
weight = 75

[extra]
nav_title = "Composer providers"
group = "Workflows"
+++

# Use Composer with Claude Code and Codex

[Composer](https://getcomposer.dev/) runs coding sessions through Claude Code or Codex. Select both providers in agnostic-ai to share project instructions, a standing rule, and a reusable skill. Composer passes the project directory to the selected provider, which loads its native files.

This guide follows [Composer CLI 0.1.12 source](https://github.com/atemaguer/composer/tree/cli-v0.1.12), not unreleased main. Release downloads were unavailable during verification, so the commands below need checking with your installed build; an authenticated Composer session was not tested. Provider versions are installed separately and affect loading and feature support.

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

Review the imported `.agnostic-ai/` sources against the original files. The last provider listed wins for a shared spec; merge any differing instructions before continuing. Skip this step in a fresh project. See [import coverage and conflicts](@/docs/migration.md#2-review).

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

```bash
agnostic-ai lint
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

Review the plan before writing. Inspect the outputs for the note, rule, and skill:

| Provider | Files to inspect |
|---|---|
| Claude Code | `CLAUDE.md`, `.claude/rules/commit-subjects.md`, `.claude/skills/change-summary/SKILL.md` |
| Codex | `AGENTS.md`, `.agents/skills/change-summary/SKILL.md` |

In this fresh two-provider example, `CLAUDE.md` imports `AGENTS.md` with `@AGENTS.md`; follow that reference to find the shared note. Codex's root `AGENTS.md` also contains the rule. Imported setups can keep full Claude text.

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

For directory-specific instructions, check each provider's discovery rules: [Claude Code](@/docs/targets/claude.md#loading-and-limits) and [Codex](@/docs/targets/codex.md#loading-and-limits). Codex needs a new session started inside the relevant subtree to load its nested `AGENTS.md`.
