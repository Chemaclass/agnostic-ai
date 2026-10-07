+++
title = "Cline"
description = "What agnostic-ai writes for Cline: file paths, what Cline supports, and config options."
weight = 70

[extra]
group = "Reference"
target_id = "cline"
+++

# Cline (`cline`)

Cline gets `AGENTS.md`, rules, agents, skills, and hook scripts. Workflows are opt-in.

## Output

```
AGENTS.md                            # pointer body, plus the rules block when another tool adds it
.clinerules/<name>.md                # one per rule that AGENTS.md does not carry, or that needs `paths`
.cline/agents/<name>.yml             # frontmatter over a Markdown system prompt
.clinerules/hooks/<Event>            # one executable script per hook event, no extension
.cline/skills/<name>/SKILL.md        # one folder per skill (Cline's recommended skills path)
.clinerules/workflows/<name>.md      # one per agent, only when workflows-dir is set
```

- **Rules**: one file each under `.clinerules/`.
  - When Codex or another tool that copies rules into `AGENTS.md` writes that file, its `## Rules` block holds every rule without a scope. Cline loads that file by default, so an always-on rule gets no `.clinerules/` file and loads once.
  - Rules with `paths` keep their file. So does a rule whose text differs for Cline (a `::target cline` fence or a path variable).
  - Keep `AGENTS.md` on in Cline's Rules panel. Otherwise Cline misses the rules that have no file of their own.
  - Listing `AGENTS.md` under `sync.unmanaged` makes every rule keep its file. Sync then stops writing `AGENTS.md` for every tool that reads it, Codex included. See [target behavior](@/docs/target-behavior.md#entry-point-files).
- **Rules layouts**: Cline [searches both](https://docs.cline.bot/customization/cline-rules) `.clinerules/` and `.cline/rules/` in VS Code, Desktop, and the CLI, and [deprecates](https://docs.cline.bot/resources/deprecations) neither. `.clinerules/` is the default because the VS Code Rules panel creates rules there (`outputs.cline.rules-dir: .cline/rules` picks the other). Sync writes one and removes the files it wrote earlier in the other, so rules never load twice.
- **Rule activation**: a scoped rule gets `paths` limited to its folder, even with `alwaysApply: true`. A rule without a scope gets native `paths` from its file selector with `alwaysApply: false`, or with `globs` other than a catch-all such as `**/*` and no `alwaysApply`. With no usable selector, `sync` reports it. See [Cline conditions](https://docs.cline.bot/customization/cline-rules) and [scoped selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Agents**: always at `.cline/agents/`, whatever the rules folder is set to. All three Cline readers use `<workspace>/.cline/agents`.
  - `<name>.yml` has frontmatter (between `---` lines) with the required `name` and `description`. The spec body follows as the system prompt, with no added heading.
  - Cline loads only `.yml` and `.yaml` files that open with `---`, and rejects an empty `name` or `description`. A missing description falls back to the name.
  - The generated-file comment sits inside the prompt. Above the `---` line, the file would not load.
  - Set `tools`, `skills`, `providerId`, `modelId`, and `maxIterations` under `x-cline`.
- **Skills**: `.cline/skills/<name>/SKILL.md`, as [Cline's skills docs](https://docs.cline.bot/customization/skills) recommend. A flat file in the rules folder never loads as a skill. Frontmatter has `name` and `description`. Files next to the skill are copied unchanged.

{% <details summary="Old agent .md files"> %}
Older releases wrote `<name>.md` with no frontmatter, which Cline skipped. Sync removes the `.md` files it wrote. Hand-written files stay.
{% </details> %}

### Hooks

Cline finds a hook script by file name only, and it has two hook runtimes.

- **The VS Code extension** runs an executable named after the event with no extension, such as `.clinerules/hooks/PreToolUse` ([source](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/apps/vscode/src/core/hooks/hook-factory.ts#L1022-L1033)). It turns the SDK runtime off, so nothing runs twice ([source](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/apps/vscode/src/sdk/vscode-session-host.ts#L171-L178)).
- **The SDK runtime**, which the Cline CLI runs, lists ten names (`TaskStart`, `TaskResume`, `TaskCancel`, `TaskComplete`, `TaskError`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`, `SessionShutdown`). It scans `<workspace>/.clinerules/hooks` and `<workspace>/.cline/hooks` and runs every file with that name and an allowed extension (no extension counts).

So sync writes one file per event to `.clinerules/hooks/<Event>`, the only layout both run, once each. The next sync removes an older `.cline/hooks/<Event>.sh` that sync wrote. Sync overwrites a hand-written `.clinerules/hooks/<Event>` for an event a spec uses. List it under `sync.unmanaged` to keep your own. A single-file `.clinerules` blocks `.clinerules/hooks/`. Sync replaces it only when a rule spec holds its content. So run `agnostic-ai import cline` first, or set `outputs.cline.hooks-dir: .cline/hooks`, which only the Cline CLI reads.

Cline's docs barely cover this. The [hooks page](https://docs.cline.bot/customization/hooks) is a stub that points to SDK Plugins (a TypeScript API). Only the [config reference](https://docs.cline.bot/getting-started/config) lists `.cline/hooks/` as lifecycle hooks.

- Each script starts with `#!/usr/bin/env bash`, and the generated-file comment follows it. The extension runs the file through its shebang, and the SDK reads the shebang to pick bash.
- **`matcher` and `timeout` do nothing**, and sync says so. A hook runs on every occurrence of its event and must filter itself. Cline uses its own timeout. A [portable](@/docs/spec-format/hooks.md#portable-events) `match` kind is the exception: the script runs the command only when the event data names one of the kind's tools, such as `run_commands` (Cline CLI) or `execute_command` (VS Code extension) for `shell`.
- Two specs on one event share one script, in spec order, and share stdout. Each spec's command runs in its own subshell under `set -e`. So a `cd`, variable, or `exit 0` in one spec does not affect the next, and a failure other than exit 2 stops the script with its code. Cline reads stdout as control JSON (the last `HOOK_CONTROL<TAB><json>` line wins), so send anything else to stderr.
- **Exit 2 blocks.** Cline ignores the exit code: only stdout `{"cancel": true}` blocks ([source](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks/hook-file-hooks.ts#L415-L454)), and the VS Code extension reads no stdout after a non-zero exit. So the script turns a command's `exit 2`, as on Claude Code, into a last `HOOK_CONTROL<TAB>{"cancel": true, "errorMessage": ...}` line with the command's stderr, and exits 0. Any exit 2 counts, including a tool error such as `grep` on a missing file. A cancel stops the whole Cline run, not only that tool call.
- **On Windows the VS Code extension runs only `<Event>.ps1`** ([source](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/apps/vscode/src/core/hooks/hook-factory.ts#L988-L997)). Sync writes no `.ps1`, so synced hooks run there only in the Cline CLI.
- [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs a hook's script with bash, Cline's event data, and its 120 second timeout, before a session does.
- `PreCompact` has no runtime event yet. Its script is written and reported but never runs.
- Any other event gets a coverage note instead of a file.

### Workflows

With `outputs.cline.workflows-dir` set to `.clinerules/workflows`, each agent is also written as `<dir>/<name>.md`, which you run from chat as `/<name>.md`. An italic description, if any, comes before the body. The key is opt-in because a workflow repeats the agent in `.cline/agents/<name>.yml`.

Cline's workflows doc page is a 404, but its resolver searches `.clinerules/workflows` and `.cline/workflows`. The VS Code extension reads only `.clinerules/workflows` and keeps it out of the rules scan, so a workflow never doubles as a rule. With `.cline/workflows`, only the CLI and SDK see them, and sync says so.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.cline.rules-dir` | `.clinerules` | set to `.cline/rules` for the other layout Cline reads; both are searched when present |
| `outputs.cline.agents-dir` | `.cline/agents` | |
| `outputs.cline.skills-dir` | `.cline/skills` | set to `.agents/skills` to share one folder with amp, codex, windsurf and zed; Cline scans it too |
| `outputs.cline.hooks-dir` | `.clinerules/hooks` | the VS Code extension reads only `.clinerules/hooks`; the Cline CLI also reads `.cline/hooks` |
| `outputs.cline.workflows-dir` | empty | opt-in; set it to `.clinerules/workflows`, the only path both hosts read |

## Import

- **Rules**: from `.clinerules/` and `.cline/rules/`. Identical duplicates become one rule. Different files with the same destination conflict. The reserved `skills`, `workflows`, and `hooks` subfolders are skipped. Native `paths` arrays are kept in `x-cline.paths`, including brace globs and empty arrays that turn off activation. Rules still follow the [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification) sorting of older layouts.
- **Agents**: each `.cline/agents/<name>.yml` becomes a `<name>.md` spec, without the generated-file header. `.yaml` is read too, and `.md` last, so files from older syncs still import. `.yml` wins a clash. The `agent-<name>.md` prefix applies only to the old layout where rules and agents shared `.clinerules/`.
- **Skills**: from the paths Cline scans, in order: `.cline/skills/`, `.clinerules/skills/`, `.claude/skills/`, `.agents/skills/`. The first same-name skill wins. Bundled assets and executable file modes are kept. The rules import skips `.clinerules/skills/`.

Cline's source confirms `.agents/skills`, but [the skills page](https://docs.cline.bot/customization/skills) lists only three paths, so sync keeps writing to the documented `.cline/skills`. To share one copy across tools, turn on `sync.shared-skills` or set `outputs.cline.skills-dir: .agents/skills`.

{% <details summary="Single-file .clinerules"> %}
Cline still reads a single-file `.clinerules`. It imports as one rule, `clinerules.md`, with its `paths` kept, and the `.clinerules/skills/` lookup is skipped. Sync and `doctor --fix` then replace the file with a `.clinerules/` folder, as Cline does, but only when its content matches a rule spec. Otherwise sync, `--check`, `--dry-run`, and `doctor --fix` fail and leave it alone. Run `agnostic-ai import cline` first.
{% </details> %}

## Protected paths

Advisory. Cline takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the [Cline extension](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev) in VS Code.
2. Check the tree: `ls .clinerules/ .cline/agents/ .cline/skills/`, `grep "Generated by agnostic-ai" .clinerules/*.md`, `ls .cline/skills/*/SKILL.md >/dev/null`.
3. Open the project and the Cline panel:
   - Each `.clinerules/*.md` appears in the rules list with no "failed to parse" warnings.
   - Each `.cline/agents/<name>.yml` appears where Cline lists project agents (`cline config`, Agent Teams via `--team-name`, the hub's agent list).
   - Each `.cline/skills/<name>/` loads as a skill.
   - A file matching a `paths` rule triggers the "Conditional rules applied: workspace:&lt;name&gt;.md" notification. An unrelated file does not.
4. Hooks: `ls -l .clinerules/hooks/` shows one executable `<Event>` file per event. In the VS Code extension on macOS or Linux, or in the Cline CLI, triggering it (edit a file for `PostToolUse`, start a task for `TaskStart`) runs the script once.
5. With workflows on, each `/<name>.md` runs in the extension and the CLI, with the italic description as the preview.
