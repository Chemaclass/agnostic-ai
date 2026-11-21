+++
title = "Troubleshooting"
description = "Fix configuration, sync, scoped-rule, and generated-file problems."
weight = 160

[extra]
group = "Reference"
+++

# Troubleshooting

Run `doctor` from the directory that holds `agnostic-ai.yaml` first:

```bash
agnostic-ai doctor
```

Follow its final **Next step**. Doctor changes nothing; [`doctor --fix`](@/docs/cli-reference/doctor.md#doctor) repairs. Rerun it afterward. If it cannot start, use the table.

| Symptom | Next step |
|---|---|
| `agnostic-ai` is not found | Check the [install destination and PATH](@/docs/installation.md) |
| The version did not change after upgrading | Run `agnostic-ai upgrade --check` to find an older binary that runs before the new one |
| Config is missing | Change to the project root, or run `agnostic-ai init` for a new project |
| A new clone or linked checkout (worktree) has no tool files | Run `agnostic-ai sync`; ignored outputs are absent from Git |
| An edited spec has no effect | Run `agnostic-ai list` to confirm it loads, then `agnostic-ai sync --dry-run` to see the planned output |
| Files match, but the agent ignores an instruction | Start a new session from the right directory and check the tool's [loading limits](@/docs/targets/_index.md) |
| A tool gets only some spec kinds | Check its [capabilities](@/docs/targets/_index.md#capability-matrix) and any unsupported-kind warnings |
| `sync --check` fails | Run `agnostic-ai sync`, review the result, and commit the generated files if the project tracks them |
| CI fails on every fresh checkout | Match the [CI recipe](@/docs/ci.md) to whether you commit generated files |
| Two targets write to the same path | Read [AAI-102](@/docs/errors.md#aai-102-targets-emit-to-the-same-output-path) and check `outputs` overrides |
| `sync` refuses to write a `.*ignore` file | Read [AAI-103](@/docs/errors.md#aai-103-hand-authored-ignore-file-would-lose-patterns), import the patterns, and check their order and negations |
| A scoped rule is skipped, conflicts, or appears missing | Check [scoped-rule diagnostics](#scoped-rules) |
| A generated skill links to a file the agent cannot open | Run `agnostic-ai doctor --check-references`; see [broken skill references](#broken-skill-references) |
| Watch mode misses changes on a mounted filesystem | Try `agnostic-ai sync --watch --watch-poll` |
| Claude Code reports that a hook timed out | Identify the [hook owner and slow command](#hook-timeouts) |

## Inspect the project

| Command | Use it to |
|---|---|
| [`validate`](@/docs/cli-reference/check.md#validate) | Check spec formats |
| [`lint`](@/docs/cli-reference/check.md#lint) | Check spec quality |
| [`sync --check`](@/docs/cli-reference/sync.md#sync) | Check generated files |
| [`status`](@/docs/cli-reference/check.md#status) | Summarize the project |
| [`why`](@/docs/cli-reference/inspect.md#why) | Trace a generated file to its source |
| [`graph`](@/docs/cli-reference/inspect.md#graph) | See which tools receive a spec |
| [`explain`](@/docs/cli-reference/inspect.md#explain) | Explain a code, such as `explain AAI-003` or `explain LINT011` |

<details>
<summary>Fix directory-specific rule problems</summary>

## Scoped rules

| Symptom | Next step |
|---|---|
| `unknown flag: --dir` or `unknown flag: --scope` | Install a newer version; see [setup](@/docs/scoped-context.md#start-with-one-directory). |
| A rule is skipped or a filter cannot be kept | Check [tool support](@/docs/scoped-context.md#native-support) and [pattern limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files). |
| Tools sharing a file conflict, or instructions differ | Use compatible tools with the same shared content, or separate worktrees. `--only` and `prefer-spec` do not bypass [scope conflicts](@/docs/scoped-context.md#shared-files-and-safe-updates). |
| A hand-written file has a line no spec holds | Move the quoted line into its spec, or set the file aside and sync. Follow [migration](@/docs/migration.md#keep-directory-specific-instructions). |
| Alternate instructions conflict | Import and preserve the alternate file, then move it off its native filename. Follow [migration](@/docs/migration.md#keep-directory-specific-instructions). |
| An `outputs` override is rejected | Remove the named override and keep headers enabled. |
| Cursor has no scoped `.mdc` file | It can share a nested `AGENTS.md` with Codex. Run `agnostic-ai graph --spec <name>` to see. |
| An old scope file remains | Run a full sync, then `sync --check`. Hand-authored files stay. |

</details>

<details>
<summary>Fix broken references inside a skill</summary>

## Broken skill references

A skill with a broken relative link still syncs. `agnostic-ai doctor --check-references` lists each broken link by source spec and destination, with affected targets on one line.

- A link that works from the project root, such as `apps/engine/src/lib.ts`, counts as valid even if the skill folder lacks the file.
- Not every tool resolves from the project root, so prefer a path the agent can open from where it runs.
- A link that leaves the project never counts.

| Cause | Fix |
|---|---|
| The linked file is in neither `.agnostic-ai/skills/<name>/` nor at that path from the project root | Add it, then run `agnostic-ai sync` |
| The link leaves the skill folder, such as `../shared/setup.md`, and no file sits at that path from the project root | Move the file into the skill folder and update the link. Sync copies only the skill's own folder |
| The tool flattens skills to one file and drops bundled files | Link to a URL, put the content in the skill, or accept the gap for that tool |
| You deleted a generated file by hand | Run `agnostic-ai sync` to restore it |
| A placeholder link in an example, such as `[Logs](url)`, can never resolve | List its destination under [`doctor.check-references.ignore`](@/docs/configuration-reference.md#doctorcheck-referencesignore) |

</details>

## A synced skill is disabled via skillOverrides

`Skill "deploy" is disabled via skillOverrides` means your Claude Code settings turned the skill off, usually in the `/skills` menu. Sync still writes it. `agnostic-ai doctor` lists every such skill. Re-enable them in `/skills` or with `agnostic-ai doctor --fix` in a terminal. See [Claude skill overrides](@/docs/cli-reference/doctor.md#claude-skill-overrides).

## Hook timeouts

`UserPromptSubmit hook [...] timed out after 5s` comes from the coding tool. It does not name the hook's owner or the slow step.

1. In Claude Code, open `/hooks` and find the event and command in the message. Check project settings, user settings, and enabled plugins. A `${CLAUDE_PLUGIN_ROOT}` command belongs to a plugin, not your specs. See the [hook reference](https://code.claude.com/docs/en/hooks).
2. For a hook generated from `.agnostic-ai/hooks/`, find its source with [`why`](@/docs/cli-reference/inspect.md#why) and inspect or test it with [hook diagnostics](@/docs/cli-reference/hook-diagnostics.md). Edit the source, then sync.
3. Time an ordinary command and the hook's runtime (`node --version`). If several unrelated hooks time out together, check CPU, memory, and runtime startup first.
4. Raise a timeout only when the command needs longer; a stuck command then takes longer to report.

The [Caveman built-in](@/docs/rtk-and-caveman.md#if-you-already-use-rtk-or-caveman) supplies a skill, not the separate Caveman plugin's mode-tracking hooks.

## Report a problem

Include the CLI version, OS, failing command, full error, and the smallest config that shows it. Remove credentials from MCP config and logs first. Open a [bug report](https://github.com/Chemaclass/agnostic-ai/issues/new?template=bug_report.yml).
