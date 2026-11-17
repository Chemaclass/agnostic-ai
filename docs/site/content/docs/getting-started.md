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
auto-manage-gitignore = "@/docs/sync-settings.md#gitignore"
ci-gate = "@/docs/ci.md#ci"
upgrade = "@/docs/installation.md#upgrade"
inside-claude-code = "@/docs/installation.md#optional-extras"
with-your-coding-agent = "@/docs/agent-setup.md"
in-your-terminal = "@/docs/getting-started.md#install-agnostic-ai"
+++

# Set up your project {#getting-started}

Write one project note, generate the files Claude Code and Codex read, then change the note once for both. Run each step from your project folder. Prefer to let your coding agent do it? [Paste this prompt](@/docs/agent-setup.md), then come back for [the daily loop](#daily-use).

## Install agnostic-ai

{{ <install_picker /> }}

```bash
agnostic-ai --version
```

If the command is not found, open a new terminal or see [Installation](@/docs/installation.md#verify-the-install).

## Initialize your project {#quickstart}

```bash
agnostic-ai init
```

Select `claude` and `codex` (agnostic-ai's names for Claude Code and Codex), or your own tools. `init` creates `agnostic-ai.yaml`, the project note `.agnostic-ai/AGNOSTIC_AI.md`, and a `.gitignore` block. Keep the default Git answer, **Ignore them**; see [commit or ignore](#commit-or-ignore-generated-outputs).

**Already have tool files?** In a terminal, `init` offers to import files such as `CLAUDE.md` and `AGENTS.md`. Accept, then check the imported sources before you sync. To import only some, see [import](@/docs/migration.md#import-existing-tool-configuration). If `agnostic-ai.yaml` and `.agnostic-ai/` already exist, skip `init`.

## Write a project note {#scaffold}

Replace the placeholder comment in `.agnostic-ai/AGNOSTIC_AI.md` (or add below imported text):

```markdown
Run the project tests before finishing a change.
```

## Generate and inspect {#sync}

```bash
agnostic-ai sync --plan
```

Stop if the plan names a file agnostic-ai does not own or would overwrite. Then:

```bash
agnostic-ai sync
agnostic-ai sync --check
```

`sync --check` exits 0 when the files match their sources.

| Tool | Generated file |
|---|---|
| Claude Code | `CLAUDE.md`, which contains `@AGENTS.md` |
| Codex | `AGENTS.md`, which holds the note |

Start a new session and ask "How should you check a change before finishing?" The files show the note was delivered, not that the model followed it; see each [tool's loading limits](@/docs/targets/_index.md).

## Change one note, update both tools {#daily-use}

Add a line to `.agnostic-ai/AGNOSTIC_AI.md`:

```markdown
Summarize which tests you ran.
```

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

Both tools get the line. For every change: edit the source, review the plan, sync. Sync replaces edits to generated files.

After cloning, install agnostic-ai as the project documents, then run `sync`. If it is a project dependency, use the local command, such as `pnpm exec agnostic-ai`.

## Commit or ignore generated outputs

| Choice | Commit | Teammates and CI |
|---|---|---|
| **Ignore** (default) | `.agnostic-ai/`, `agnostic-ai.yaml`, `.gitignore`, and files sync lists | Run `sync` after cloning; use the [ignored-output CI recipe](@/docs/ci.md#ignored-outputs) |
| **Commit** (`init --gitignore=off`) | The generated files too | Clones have them ready; use the [committed-output CI check](@/docs/ci.md#committed-outputs) |

`git diff` hides ignored files, so open them to review. To switch later, see [Git ignore settings](@/docs/sync-settings.md#gitignore).

## Everyday commands

<details>
<summary>Commands to run from the folder with agnostic-ai.yaml</summary>

| Command | What it does |
|---|---|
| `agnostic-ai status` | Lists selected tools and out-of-date files. |
| `agnostic-ai sync --plan` | Previews changes. |
| `agnostic-ai sync` | Writes the tool files. |
| `agnostic-ai sync --check` | Confirms the files match their sources. |
| `agnostic-ai sync --watch` | Syncs while you edit. |
| `agnostic-ai use codex` | Adds Codex to an existing setup. Or edit [`targets:`](@/docs/configuration.md#targets). |
| `agnostic-ai ui` | Opens the configuration in your browser; see the [walkthrough](@/docs/inspect-configuration.md). |
| `agnostic-ai view --json` | Prints the same overview as JSON. |
| `agnostic-ai doctor` | Diagnoses problems. `doctor --fix` repairs generated files. |
| `agnostic-ai why AGENTS.md` | Traces a generated file to its source. |
| `agnostic-ai explain <spec> --layers` | Shows which built-in, pack, project, and personal sources replace or extend a spec. |
| `agnostic-ai verify --evaluation smoke-v1 --record run.json` | Saves a check you can [compare later](@/docs/cli-reference/check.md#record-and-compare-runs). |

- [Checkout hooks](@/docs/git-hooks.md#regenerate-on-checkout) restore ignored files in new checkouts.
- Automated environments can use [project readiness checks](@/docs/cli-reference/project.md#project-readiness).
- If something looks wrong, run `doctor`, then see [Troubleshooting](@/docs/troubleshooting.md).
- The [CLI reference](@/docs/cli-reference/_index.md) covers every command.

</details>

<details>
<summary>Add a standing convention</summary>

## First rule

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

- Use your team's prefixes.
- A rule asks the model to follow the convention. Enforcing the format needs a Git check.
- Run `sync --plan`, `sync`, `sync --check`.
- Claude Code gets `.claude/rules/conventional-commits.md`. Codex gets a Rules section in `AGENTS.md`.

</details>

## Next steps

- Several-step task: write a [skill](@/docs/spec-format/skills.md).
- Command at a tool event: add a [hook](@/docs/spec-format/hooks.md).
- Standing convention: add a [rule](@/docs/spec-format/rules.md).
- One folder, different instructions: [directory-specific instructions](@/docs/scoped-context.md).
- Personal specs for every project: [global configuration](@/docs/configuration-reference.md#global-configuration), via `agnostic-ai sync --global`.
- Composer: [set up its Claude Code and Codex providers](@/docs/composer.md).

[Why agnostic-ai?](@/docs/why-agnostic-ai.md#what-to-add-and-when) lists every spec kind.
