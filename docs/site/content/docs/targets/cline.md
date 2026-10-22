+++
title = "Cline"
description = "What agnostic-ai writes for Cline: file paths, what Cline supports, and config options."
weight = 70

[extra]
group = "Reference"
target_id = "cline"

[extra.moved]
set-up-this-tool = "@/docs/targets/cline.md#first-rule"
+++

# Cline (`cline`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Cline gets `AGENTS.md`, rules, agents, skills, and hook scripts. Workflows are opt-in.

## First rule

Install the client using the [official setup guide](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev), then open your project in it.

Select `cline` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `.clinerules/conventional-commits.md`, or shared `AGENTS.md` when another selected target already puts the rule there.

When another target includes this rule in `AGENTS.md`, Cline uses that shared copy instead. Keep `AGENTS.md` enabled in the Rules panel.

For existing files, check [what `import cline` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and find the commit-prefix rule in `.clinerules/conventional-commits.md` or the shared `AGENTS.md`. Open the project in Cline and confirm the rule is enabled in its Rules panel. Ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
AGENTS.md                            # pointer body, plus the rules block when another tool adds it
.clinerules/<name>.md                # one per rule that AGENTS.md does not carry, or that needs `paths`
.cline/agents/<name>.yml             # frontmatter over a Markdown system prompt
.clinerules/hooks/<Event>            # one executable script per hook event, no extension
.cline/skills/<name>/SKILL.md        # one folder per skill (Cline's recommended skills path)
.clinerules/workflows/<name>.md      # one per agent, only when workflows-dir is set
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>


- **Rules**: one file each under `.clinerules/`.
  - When Codex or another tool that copies rules into `AGENTS.md` writes that file, its `## Rules` block holds every rule without a scope. Cline loads that file by default, so an always-on rule gets no `.clinerules/` file and loads once.
  - Rules with `paths` keep their file. So does a rule whose text differs for Cline (a `::target cline` fence or a path variable).
  - Keep `AGENTS.md` on in Cline's Rules panel. Otherwise Cline misses the rules that have no file of their own.
  - Listing `AGENTS.md` under `sync.unmanaged` makes every rule keep its file. Check [the effect on other targets sharing that file](@/docs/target-behavior.md#inlined-rules).
- **Rules layouts**: Cline [searches both](https://docs.cline.bot/customization/cline-rules) `.clinerules/` and `.cline/rules/` in VS Code, Desktop, and the CLI, and [deprecates](https://docs.cline.bot/resources/deprecations) neither. `.clinerules/` is the default because the VS Code Rules panel creates rules there (`outputs.cline.rules-dir: .cline/rules` picks the other). Sync writes one and removes the files it wrote earlier in the other, so rules never load twice.
- **Rule activation**: a scoped rule gets `paths` limited to its folder, even with `alwaysApply: true`. A rule without a scope gets native `paths` from its file selector with `alwaysApply: false`, or with `globs` other than a catch-all such as `**/*` and no `alwaysApply`. With no usable selector, `sync` reports it. See [Cline conditions](https://docs.cline.bot/customization/cline-rules) and [file-pattern limits for directory rules](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Agents**: always at `.cline/agents/`, whatever the rules folder is set to. All three Cline readers use `<workspace>/.cline/agents`.
  - `<name>.yml` has frontmatter (between `---` lines) with the required `name` and `description`. The spec body follows as the system prompt, with no added heading.
  - Cline loads only `.yml` and `.yaml` files that open with `---`, and rejects an empty `name` or `description`. A missing description falls back to the name.
  - The generated-file comment sits inside the prompt. Above the `---` line, the file would not load.
  - Set `tools`, `skills`, `providerId`, `modelId`, and `maxIterations` under `x-cline`.
- **Skills**: `.cline/skills/<name>/SKILL.md`, as [Cline's skills docs](https://docs.cline.bot/customization/skills) recommend. A flat file in the rules folder never loads as a skill. Frontmatter has `name` and `description`. Files next to the skill are copied unchanged.
  - Cline documents no field that keeps a skill user-only: any enabled skill runs from the chat input as `/<name>`, and the model can also pick it ([Cline skills](https://docs.cline.bot/customization/skills)). Its `disabled` field hides the skill from the person too. So `disable-model-invocation` is dropped with a note, and commands are skipped.

<details>
<summary>Old agent .md files</summary>

Older releases wrote `<name>.md` with no frontmatter, which Cline skipped. Sync removes the `.md` files it wrote. Hand-written files stay.

</details>

### Hooks

Cline finds a hook script by file name only, and it has two hook runtimes.

- **The VS Code extension** runs an executable named after the event with no extension, such as `.clinerules/hooks/PreToolUse` ([source](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/apps/vscode/src/core/hooks/hook-factory.ts#L1022-L1033)). It turns the SDK runtime off, so nothing runs twice ([source](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/apps/vscode/src/sdk/vscode-session-host.ts#L171-L178)).
- **The SDK runtime**, which the Cline CLI runs, lists ten names (`TaskStart`, `TaskResume`, `TaskCancel`, `TaskComplete`, `TaskError`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`, `SessionShutdown`). It scans `<workspace>/.clinerules/hooks` and `<workspace>/.cline/hooks` and runs every file with that name and an allowed extension (no extension counts).

So sync writes one file per event to `.clinerules/hooks/<Event>`, the only layout both run, once each. The next sync removes an older `.cline/hooks/<Event>.sh` that sync wrote. Sync overwrites a hand-written `.clinerules/hooks/<Event>` for an event a spec uses. List it under `sync.unmanaged` to keep your own. A single-file `.clinerules` blocks `.clinerules/hooks/`. Sync replaces it only when a rule spec holds its content. So run `agnostic-ai import cline` first, or set `outputs.cline.hooks-dir: .cline/hooks`, which only the Cline CLI reads.

Cline's docs barely cover this. The [hooks page](https://docs.cline.bot/customization/hooks) is a stub that points to SDK Plugins (a TypeScript API). Only the [config reference](https://docs.cline.bot/getting-started/config) lists hooks, under `.cline/hooks/` and in the older `.clinerules/hooks/` folder.

- Each script starts with `#!/usr/bin/env bash`, and the generated-file comment follows it. The extension runs the file through its shebang, and the SDK reads the shebang to pick bash.
- **`matcher` and `timeout` do nothing**, and sync says so. A hook runs on every occurrence of its event and must filter itself. Cline uses its own timeout. A [portable](@/docs/spec-format/hook-reference.md#portable-events) `match` kind is the exception: the script runs the command only when the event data names one of the kind's tools, such as `run_commands` (Cline CLI) or `execute_command` (VS Code extension) for `shell`. Tool names in target overrides are matched as literal JSON values, including quotes, backslashes, and newlines. They cannot add shell commands.
- Two specs on one event share one script, in spec order, and share stdout. Each spec's command runs in its own subshell under `set -e`. So a `cd`, variable, or `exit 0` in one spec does not affect the next, and a failure other than exit 2 stops the script with its code. Cline reads stdout as control JSON (the last `HOOK_CONTROL<TAB><json>` line wins), so send anything else to stderr.
- **Exit 2 blocks.** Cline ignores the exit code: only stdout `{"cancel": true}` blocks ([source](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks/hook-file-hooks.ts#L415-L454)), The current VS Code extension honors valid reply JSON even after a nonzero exit. So the script turns a command's `exit 2`, as on Claude Code, into a last `HOOK_CONTROL<TAB>{"cancel": true, "errorMessage": ...}` line with the command's stderr, and exits 0. Any exit 2 counts, including a tool error such as `grep` on a missing file. A cancel stops the whole Cline run, not only that tool call.
- **On Windows the VS Code extension runs only `<Event>.ps1`** ([source](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/apps/vscode/src/core/hooks/hook-factory.ts#L988-L997)). For a TaskStart hook with `x-cline.runtime: vscode` and `commandWindows`, a Windows sync also writes its native `.ps1` companion. Unix sync omits it so the SDK does not run a second PowerShell hook. Normal ledger cleanup removes a previously owned companion when syncing on Unix.
- [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) runs a hook's script with bash, Cline's event data, and its 120 second timeout, before a session does.
- `PreCompact` has no runtime event yet. Its script is written and reported but never runs.
- Any other event gets a sync note instead of a file.

### Workflows

With `outputs.cline.workflows-dir` set to `.clinerules/workflows`, each agent is also written as `<dir>/<name>.md`, which you run from chat as `/<name>.md`. An italic description, if any, comes before the body. The key is opt-in because a workflow repeats the agent in `.cline/agents/<name>.yml`.

Cline's workflows doc page now redirects to the [rules page](https://docs.cline.bot/customization/cline-rules). The [config reference](https://docs.cline.bot/getting-started/config) lists `.cline/workflows/` and the older `.clinerules/workflows/`, and Cline's resolver searches both. The VS Code extension reads only `.clinerules/workflows` and keeps it out of the rules scan, so a workflow never doubles as a rule. With `.cline/workflows`, only the CLI and SDK see them, and sync says so.

</details>

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.cline.rules-dir` | `.clinerules` | set to `.cline/rules` for the other layout Cline reads; both are searched when present |
| `outputs.cline.agents-dir` | `.cline/agents` | |
| `outputs.cline.skills-dir` | `.cline/skills` | set to `.agents/skills` to share one folder with amp, codex, windsurf and zed; Cline scans it too |
| `outputs.cline.hooks-dir` | `.clinerules/hooks` | the VS Code extension reads only `.clinerules/hooks`; the Cline CLI also reads `.cline/hooks` |
| `outputs.cline.workflows-dir` | empty | opt-in; set it to `.clinerules/workflows`, the only path both hosts read |

## Global hooks

`agnostic-ai sync --global --only cline` writes one executable `~/.cline/hooks/<Event>` for each event in the global hook specs. `CLINE_DIR` moves that root. Commands for the same event share one script. Shared command bodies and portable wrappers live in `~/.cline/hook-scripts/`, outside the hook discovery folder.

These user hooks run in the Cline SDK and CLI. The VS Code extension reads global hooks from the system Documents folder's `Cline/Hooks/`, which sync does not write. The [config reference](https://docs.cline.bot/getting-started/config) lists both locations; the [SDK path resolver](https://github.com/cline/cline/blob/main/sdk/packages/shared/src/storage/paths.ts) and [extension loader](https://github.com/cline/cline/blob/main/apps/vscode/src/core/storage/disk.ts) show the reader difference.

Global sync preserves unrelated event files and stops on a conflicting hand-written event file. Removing a global spec removes its unchanged generated script. `import --global cline` does not import hook scripts.

## Import

- **Rules**: from `.clinerules/` and `.cline/rules/`. Identical duplicates become one rule. Different files with the same destination conflict. The reserved `skills`, `workflows`, and `hooks` subfolders are skipped. Native `paths` arrays are kept in `x-cline.paths`, including brace globs and empty arrays that turn off activation. Rules still follow the [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification) sorting of older layouts.
- **Agents**: each `.cline/agents/<name>.yml` becomes a `<name>.md` spec, without the generated-file header. `.yaml` is read too, and `.md` last, so files from older syncs still import. `.yml` wins a clash. The `agent-<name>.md` prefix applies only to the old layout where rules and agents shared `.clinerules/`.
- **Skills**: from the paths Cline scans, in order: `.cline/skills/`, `.clinerules/skills/`, `.claude/skills/`, `.agents/skills/`. The first same-name skill wins. Bundled assets and executable file modes are kept. The rules import skips `.clinerules/skills/`.

The [config reference](https://docs.cline.bot/getting-started/config) lists `.agents/skills/`, but [the skills page](https://docs.cline.bot/customization/skills) lists only three paths, so sync keeps writing to the documented `.cline/skills`. To share one copy across tools, turn on `sync.shared-skills` or set `outputs.cline.skills-dir: .agents/skills`.

<details>
<summary>Single-file .clinerules</summary>

Cline still reads a single-file `.clinerules`. It imports as one rule, `clinerules.md`, with its `paths` kept, and the `.clinerules/skills/` lookup is skipped. Sync and `doctor --fix` then replace the file with a `.clinerules/` folder, as Cline does, but only when its content matches a rule spec. Otherwise sync, `--check`, `--dry-run`, and `doctor --fix` fail and leave it alone. Run `agnostic-ai import cline` first.

</details>

## Protected paths

Not enforced. Cline takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

## VS Code memory startup

The `memory` builtin adds a project TaskStart hook that returns the existing memory indexes as `contextModification`. Cline VS Code waits up to 30 seconds, uses the workspace root as the working directory, and adds that text when the hook does not cancel. This needs Cline for VS Code 4.1.20 or later; earlier versions drop the text. The shared memory rule remains available when hooks are disabled. The default SDK and CLI TaskStart hooks remain detached and add no context.

`x-cline.runtime: vscode` marks this project surface and prevents global SDK hook emission. The marker appears only in portable specs, not a vendor configuration file. On Windows, supply `commandWindows` for TaskStart to create the `.ps1` file the extension discovers. Existing hooks without this marker keep their outputs.

On Unix, `agnostic-ai hook run memory-cline-task-start --target cline --runtime vscode --format json` models the VS Code TaskStart payload, executable-file discovery, timeout, and reply. It runs the synced event file, including any siblings, and excludes a shared script from expectation checks. Windows diagnostics report not run because the extension's PowerShell installation resolver is not yet modeled. Default diagnostics continue to model the SDK.

Source: [VS Code hook adapter](https://github.com/cline/cline/blob/26cea2fa8bc481e942cac2f7cfb412c7c9d139f2/apps/vscode/src/sdk/hooks-adapter.ts), [hook discovery](https://github.com/cline/cline/blob/26cea2fa8bc481e942cac2f7cfb412c7c9d139f2/apps/vscode/src/core/hooks/hook-factory.ts).
