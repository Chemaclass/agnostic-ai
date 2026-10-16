+++
title = "Getting started"
description = "Write one project instruction and one rule, then generate and inspect the files for Claude Code and Codex."
weight = 20

[extra]
group = "Start"
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
+++

# Getting started

Share a project note and a commit convention between Claude Code and Codex. You will edit two source files, generate each tool's native files, and inspect what they receive.

You need a project folder, a terminal, and [agnostic-ai installed](@/docs/installation.md). To try the result, install and sign in to [Claude Code](@/docs/targets/claude.md) or [Codex](@/docs/targets/codex.md) separately. New to these concepts? Read [Start here](@/docs/why-agnostic-ai.md).

Already have `CLAUDE.md`, `AGENTS.md`, or other tool configuration? Follow [Import existing tool configuration](@/docs/migration.md) first. Already use agnostic-ai? Skip to [Daily use](#daily-use).

## Quickstart

From your project root:

```bash
agnostic-ai init
```

For this walkthrough, select `claude` and `codex` to compare their generated files. You need only one installed client to try the result. In your real project, select only the tools you use and follow the matching output rows. These are the CLI names for Claude Code and Codex. `init` creates `agnostic-ai.yaml` and `.agnostic-ai/`, and asks whether Git should [ignore the generated files](#commit-or-ignore-generated-outputs).

## Project instructions {#scaffold}

Write a short project note in `.agnostic-ai/AGNOSTIC_AI.md`:

```markdown
Before changing code, read CONTRIBUTING.md for the project's test command.
Run the relevant tests before finishing a change.
```

Adapt this to a file and command that exist in your project. These instructions become the starting notes in `CLAUDE.md` and `AGENTS.md`.

## First rule

Create a standing convention:

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

Use your team's prefixes. A rule is a request to the model; a Git check is needed if your team wants to enforce the format.

## Generate and inspect {#sync}

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

Review the plan before syncing. The check should exit 0, meaning the files match their sources. Open these outputs:

| Tool | Project note | Commit rule |
|---|---|---|
| Claude Code | `CLAUDE.md` | `.claude/rules/conventional-commits.md` |
| Codex | `AGENTS.md` | A Rules section in the same `AGENTS.md` |

Start a new Claude Code or Codex session from this project folder. Ask: “Which test command should you use here, and what commit format should you follow?” Check its answer against your sources. Matching files do not prove the model loaded or followed the text; the [tool's page](@/docs/targets/_index.md) explains its loading limits.

## Commit or ignore generated outputs

`init` asks you to choose:

- **Ignore (default):** commit `.agnostic-ai/`, `agnostic-ai.yaml`, and `.gitignore`. Teammates install agnostic-ai and run `sync` after cloning. Use the [ignored-output CI recipe](@/docs/ci.md#ignored-outputs).
- **Commit (`init --gitignore=off`):** also commit the generated files. A clone has them ready. Use the [committed-output CI check](@/docs/ci.md#committed-outputs).

To change this choice later, see [Git ignore settings](@/docs/configuration.md#gitignore).

## Daily use

After cloning, install the CLI using the project's documented method. For a project dependency, install its locked dependencies and use the local command, such as `pnpm exec agnostic-ai`.

Edit `.agnostic-ai/AGNOSTIC_AI.md` or a source spec, then run:

```bash
agnostic-ai sync
agnostic-ai sync --check
```

Review the output. If Git tracks generated files, commit them with the source change. Avoid editing generated files: sync can replace those edits.

<details>
<summary>Watch for edits, inspect status, or add another coding tool</summary>

Use `agnostic-ai sync --watch` while editing. `agnostic-ai status` lists loaded specs, selected tools, and out-of-date files. Run `agnostic-ai use codex` to add Codex to an existing setup, or change `targets:` in [configuration](@/docs/configuration.md#targets).

[Checkout hooks](@/docs/git-hooks.md#regenerate-on-checkout) restore ignored files in new checkouts. Automated environments can use [project readiness checks](@/docs/cli-reference/maintain.md#project-readiness).

</details>

## Next steps

Keep the setup small. Add [directory-specific instructions](@/docs/scoped-context.md) when one folder needs different conventions. For a repeated task or outside connection, use the [spec guide](@/docs/spec-format/_index.md). If a file is missing or a check fails, start with [Troubleshooting](@/docs/troubleshooting.md).
