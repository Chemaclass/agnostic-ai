+++
title = "Set up your project"
description = "Share one project note across your coding tools, then change it once and update them together."
weight = 20

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

Claude Code and Codex read project notes to learn how to work in your codebase. Keep those notes in one place with agnostic-ai. Edit once, then sync the files your tools read.

Rather have your coding agent do it? [Paste one prompt](@/docs/agent-setup.md) instead, then come back for [the daily loop](#daily-use).

The walkthrough below writes one project note, generates files for Claude Code and Codex, then edits the note and updates both. Follow along with the tools you use, from your project folder.

## Install agnostic-ai

{{ <install_picker /> }}

Verify the command:

```bash
agnostic-ai --version
```

Command not found? Open a new terminal or see [Installation](@/docs/installation.md#verify-the-install).

## Initialize your project {#quickstart}

From your project root:

```bash
agnostic-ai init
```

Select `claude` and `codex` for this example, or select the tools you actually use. These are the CLI names for Claude Code and Codex. `init` creates `agnostic-ai.yaml` and updates `.gitignore`; the next step puts your note in `.agnostic-ai/`.

When asked about Git, keep **Ignore them** (the default) for this walkthrough. Commit the sources; teammates run `sync` after cloning. Choose **Commit them** if you want Git to track generated files so a clone has them ready. See [Commit or ignore generated outputs](#commit-or-ignore-generated-outputs) after your first sync.

**Already have tool files?** In a terminal, `init` offers to import existing configuration such as `CLAUDE.md` and `AGENTS.md`. Accept to keep those instructions, then inspect the imported sources before syncing. Import can report sections that need a manual merge. For a selective import, use the [import reference](@/docs/migration.md#import-existing-tool-configuration). If `agnostic-ai.yaml` and `.agnostic-ai/` already exist, keep them and skip `init`.

## Write a project note {#scaffold}

Create the source folder if needed:

```bash
mkdir -p .agnostic-ai
```

Create or edit `.agnostic-ai/AGNOSTIC_AI.md`. Keep any imported instructions and add a useful note, for example:

```markdown
Run the project tests before finishing a change.
```

Adapt this to your project’s test command if you know it. This is the only instruction source you need to edit for this walkthrough.

## Generate and inspect {#sync}

Preview the files before writing them:

```bash
agnostic-ai sync --plan
```

Check the paths and reported changes. Sync can replace generated files, so confirm that existing instructions have been imported. If it reports a file it does not own or would overwrite, stop and review that path before proceeding.

Then generate and check:

```bash
agnostic-ai sync
agnostic-ai sync --check
```

The check should exit 0, meaning the generated files match their sources. In this fresh, note-only Claude Code and Codex example, the outputs are:

| Tool | Generated project notes |
|---|---|
| Claude Code | `CLAUDE.md`, which points to `AGENTS.md` |
| Codex | `AGENTS.md` |

Find “Run the project tests before finishing a change.” in `AGENTS.md`. Here, `CLAUDE.md` contains `@AGENTS.md` so Claude Code reads the same notes. A Claude-only setup keeps the note directly in `CLAUDE.md`; imported setups can also keep full Claude text. Inspect your selected tools’ native files and follow references when present. Keep editing the source in `.agnostic-ai/AGNOSTIC_AI.md`.

Start a new Claude Code or Codex session from this project folder. Ask: “How should you check a change before finishing?” Compare the answer with your note. Matching files do not prove the model loaded or followed the text; see the [tool's loading limits](@/docs/targets/_index.md).

## Change one note, update both tools {#daily-use}

Add another sentence to the same `.agnostic-ai/AGNOSTIC_AI.md`:

```markdown
Summarize which tests you ran.
```

Run:

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

In this fresh two-tool example, open `AGENTS.md` again to find the new sentence. `CLAUDE.md` still points to it, so both tools use the updated instructions. For another setup, check the native files you inspected above. This is the maintenance loop: edit one source, review the plan, and sync your selected tools.

After cloning, install agnostic-ai using the project's documented method, then run `sync`. For a project dependency, install its locked dependencies and use the local command, such as `pnpm exec agnostic-ai`.

<details>
<summary>Watch for edits, inspect status, or add another coding tool</summary>

Use `agnostic-ai sync --watch` while editing. `agnostic-ai status` lists loaded specs, selected tools, and out-of-date files. Run `agnostic-ai use codex` to add Codex to an existing setup, or change `targets:` in [configuration](@/docs/configuration.md#targets).

[Checkout hooks](@/docs/git-hooks.md#regenerate-on-checkout) restore ignored files in new checkouts. Automated environments can use [project readiness checks](@/docs/cli-reference/maintain.md#project-readiness).

</details>

## Commit or ignore generated outputs

Keep source changes in Git with the policy you chose during `init`:

- **Ignore (default):** commit `.agnostic-ai/`, `agnostic-ai.yaml`, and `.gitignore`. Teammates install agnostic-ai and run `sync` after cloning. Use the [ignored-output CI recipe](@/docs/ci.md#ignored-outputs).
- **Commit (`init --gitignore=off`):** also commit generated files with their source changes. A clone has them ready. Use the [committed-output CI check](@/docs/ci.md#committed-outputs).

Inspect ignored output files directly because `git diff` does not show them. To change this choice later, see [Git ignore settings](@/docs/configuration.md#gitignore).

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

Use your team's prefixes. A rule asks the model to follow the convention; a Git check is needed to enforce the format.

Run `agnostic-ai sync --plan`, review the changes, then run `agnostic-ai sync` and `agnostic-ai sync --check`. Claude Code receives `.claude/rules/conventional-commits.md`. Codex receives a Rules section in `AGENTS.md`.

</details>

## Next steps

Add more only when a task needs it:

- Repeat a task with several steps: write a [skill](@/docs/spec-format/skills.md), such as a release checklist.
- Run a command automatically at a supported tool event: add a [hook](@/docs/spec-format/hooks.md).
- Keep a standing convention separate from the project note: add a [rule](@/docs/spec-format/rules.md).
- Give one folder different instructions: use [directory-specific instructions](@/docs/scoped-context.md).

The [setup overview](@/docs/why-agnostic-ai.md) explains the other kinds of configuration. If a file is missing or a check fails, start with [Troubleshooting](@/docs/troubleshooting.md).
