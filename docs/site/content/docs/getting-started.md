+++
title = "Set up your project"
description = "Share one project note across your coding tools, then change it once and update them together."
weight = 15

[extra]
nav_title = "Set up your project"
group = "Start"
scripts = ["assets/scripts/landing.js"]
next = "@/docs/scoped-context.md"
next_label = "Apply an instruction to one directory"

[extra.moved]
install = "@/docs/installation.md#installation"
shell-completion = "@/docs/cli-reference/maintain.md#completion"
add-a-single-spec = "@/docs/cli-reference/start.md#new"
import-an-existing-ai-cli-config = "@/docs/migration.md#import-existing-tool-configuration"
recommended-adoption-workflow = "@/docs/migration.md#import-existing-tool-configuration"
what-import-does-not-capture = "@/docs/migration.md#review-what-was-imported"
agnostic-aisync-state = "@/docs/cli-reference/sync.md#sync"
check-project-status = "@/docs/cli-reference/check.md#status"
roll-back-a-sync = "@/docs/cli-reference/maintain.md#revert"
watch-mode = "@/docs/cli-reference/sync.md#watch-mode"
auto-manage-gitignore = "@/docs/configuration.md#gitignore"
ci-gate = "@/docs/ci.md#ci"
upgrade = "@/docs/installation.md#upgrade"
inside-claude-code = "@/docs/installation.md#optional-extras"
with-your-coding-agent = "@/docs/agent-setup.md"
in-your-terminal = "@/docs/getting-started.md#install-agnostic-ai"
+++

# Set up your project {#getting-started}

This walkthrough writes one project note, generates the files Claude Code and Codex read, then changes the note and updates both. It takes a few minutes. Run each step from your project folder, with the tools you use.

Prefer to have your coding agent do the setup? [Paste this prompt](@/docs/agent-setup.md) instead, then come back for [the daily loop](#daily-use).

## Install agnostic-ai

{{ <install_picker /> }}

Check that the command works:

```bash
agnostic-ai --version
```

If the command is not found, open a new terminal or see [Installation](@/docs/installation.md#verify-the-install).

## Initialize your project {#quickstart}

```bash
agnostic-ai init
```

Select `claude` and `codex` for this example, or the tools you use. These are the names agnostic-ai uses for Claude Code and Codex. `init` creates `agnostic-ai.yaml`, the `.agnostic-ai/` source folder, and a block in `.gitignore`.

When asked about Git, keep **Ignore them** (the default). You commit the sources, and each teammate runs `sync` after cloning. See [Commit or ignore generated outputs](#commit-or-ignore-generated-outputs) for the other choice.

**Already have tool files?** In a terminal, `init` offers to import existing files such as `CLAUDE.md` and `AGENTS.md`. Accept to keep their instructions, and check the imported sources before you sync. Import tells you when a section needs a manual merge. To import only some files, see the [import reference](@/docs/migration.md#import-existing-tool-configuration). If `agnostic-ai.yaml` and `.agnostic-ai/` already exist, skip `init`.

## Write a project note {#scaffold}

Open `.agnostic-ai/AGNOSTIC_AI.md` in your editor, creating it if needed. Keep any imported instructions and add one note, for example:

```markdown
Run the project tests before finishing a change.
```

Use your project's real test command if you know it. This file is the only source you edit in this walkthrough.

## Generate and inspect {#sync}

Preview the files first:

```bash
agnostic-ai sync --plan
```

Sync can replace generated files. If the plan names a file it does not own or would overwrite, stop and check that its instructions were imported.

Then generate and check:

```bash
agnostic-ai sync
agnostic-ai sync --check
```

`sync --check` exits 0 when the generated files match their sources. In this Claude Code and Codex example, you get:

| Tool | Generated project notes |
|---|---|
| Claude Code | `CLAUDE.md`, which points to `AGENTS.md` |
| Codex | `AGENTS.md` |

Find "Run the project tests before finishing a change." in `AGENTS.md`. `CLAUDE.md` contains `@AGENTS.md`, so Claude Code reads the same note. With only Claude Code selected, the note goes straight into `CLAUDE.md`, and an imported setup can keep its full Claude text there. For other tools, open their generated files and follow any references.

To see it work, start a new Claude Code or Codex session in this folder and ask: "How should you check a change before finishing?" Matching files show the note was delivered, not that the model followed it; see each [tool's loading limits](@/docs/targets/_index.md).

## Change one note, update both tools {#daily-use}

Add a sentence to `.agnostic-ai/AGNOSTIC_AI.md`:

```markdown
Summarize which tests you ran.
```

Run:

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

Open `AGENTS.md` to find the new sentence. `CLAUDE.md` still points to it, so both tools read the update. This is the daily loop: edit the source, review the plan, sync. Never edit the generated files; sync replaces them.

After cloning, install agnostic-ai the way the project documents, then run `sync`. If the project lists agnostic-ai as a dependency, install its locked dependencies and use the local command, such as `pnpm exec agnostic-ai`.

<details>
<summary>Watch for edits, inspect status, or add another coding tool</summary>

Use `agnostic-ai sync --watch` while editing. `agnostic-ai status` lists loaded specs, selected tools, and out-of-date files. Run `agnostic-ai use codex` to add Codex to an existing setup, or change `targets:` in [configuration](@/docs/configuration.md#targets).

[Checkout hooks](@/docs/git-hooks.md#regenerate-on-checkout) restore ignored files in new checkouts. Automated environments can use [project readiness checks](@/docs/cli-reference/project.md#project-readiness).

</details>

## Commit or ignore generated outputs

Choose this during `init`:

- **Ignore (default):** commit `.agnostic-ai/`, `agnostic-ai.yaml`, `.gitignore`, and any other file sync lists to commit. Teammates install agnostic-ai and run `sync` after cloning. Use the [ignored-output CI recipe](@/docs/ci.md#ignored-outputs).
- **Commit (`init --gitignore=off`):** also commit the generated files with their source changes, so a clone has them ready. Use the [committed-output CI check](@/docs/ci.md#committed-outputs).

`git diff` does not show ignored files, so open them directly to review them. To change this choice later, see [Git ignore settings](@/docs/configuration.md#gitignore).

## Everyday commands

Run these from the folder that contains `agnostic-ai.yaml`:

| Command | What it does |
|---|---|
| `agnostic-ai status` | Lists selected tools and out-of-date files. |
| `agnostic-ai sync --plan` | Previews what sync will change. |
| `agnostic-ai sync` | Writes the tool files. |
| `agnostic-ai sync --check` | Confirms the tool files match their sources. |
| `agnostic-ai ui` | Opens the project or user configuration in your browser. See the [browser walkthrough](@/docs/inspect-configuration.md). |
| `agnostic-ai view --json` | Prints the same configuration overview as JSON. |
| `agnostic-ai doctor` | Diagnoses problems and suggests a next step. `doctor --fix` repairs generated files. |
| `agnostic-ai why AGENTS.md` | Traces a generated file back to its source. |
| `agnostic-ai explain <spec> --layers` | Shows which built-in, pack, project, and personal sources replace or extend a spec. |
| `agnostic-ai verify --evaluation smoke-v1 --record run.json` | Saves a check you can [compare after configuration changes](@/docs/cli-reference/check.md#record-and-compare-runs). |

If something looks wrong, start with `doctor` and then [Troubleshooting](@/docs/troubleshooting.md). The [CLI reference](@/docs/cli-reference/_index.md) covers every command, including import, compare, and upgrade.

<details>
<summary>Add a standing convention when you need one</summary>

## First rule

Create a rule for your team's commit convention:

```bash
agnostic-ai new rule conventional-commits
```

Replace `.agnostic-ai/rules/conventional-commits.md` with:

```markdown
---
name: conventional-commits
description: Use the team's commit message format.
alwaysApply: true
---

Start commit subjects with feat:, fix:, docs:, refactor:, test:, or chore:.
Keep the subject under 72 characters.
```

Use your team's prefixes. A rule asks the model to follow the convention; enforcing the format needs a Git check.

Run `agnostic-ai sync --plan`, review the changes, then run `agnostic-ai sync` and `agnostic-ai sync --check`. Claude Code receives `.claude/rules/conventional-commits.md`. Codex receives a Rules section in `AGENTS.md`.

</details>

## Next steps

Add more only when a task needs it:

- Repeat a task with several steps: write a [skill](@/docs/spec-format/skills.md), such as a release checklist.
- Run a command at a tool event: add a [hook](@/docs/spec-format/hooks.md).
- Keep a standing convention apart from the project note: add a [rule](@/docs/spec-format/rules.md).
- Give one folder different instructions: use [directory-specific instructions](@/docs/scoped-context.md).
- Keep personal specs for every project in `~/.agnostic-ai/`: see [global configuration](@/docs/configuration.md#global-configuration), written with `agnostic-ai sync --global`.
- Using Composer? [Set up its Claude Code and Codex providers](@/docs/composer.md) from the same sources.

[Why agnostic-ai?](@/docs/why-agnostic-ai.md#what-to-add-and-when) lists every kind of spec you can add.
