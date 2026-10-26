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

Follow its final **Next step**. Doctor is read-only by default; [`doctor --fix`](@/docs/cli-reference/check.md#doctor) applies its repairs, after which you rerun `doctor`. When a finding needs more detail or `doctor` cannot start, find the symptom below.

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

After `doctor`, use the command its finding names:

- [`validate`](@/docs/cli-reference/check.md#validate) checks spec formats, [`lint`](@/docs/cli-reference/check.md#lint) spec quality, and [`sync --check`](@/docs/cli-reference/sync.md#sync) generated files.
- [`status`](@/docs/cli-reference/check.md#status) summarizes the project.
- [why](@/docs/trace.md) traces a generated file to its source, and [graph](@/docs/graph.md) shows which tools receive a spec.
- [`explain`](@/docs/cli-reference/inspect.md#explain) explains a diagnostic code, such as `agnostic-ai explain AAI-003` or `agnostic-ai explain LINT011`.

<details>
<summary>Fix directory-specific rule problems</summary>

## Scoped rules

| Symptom | Next step |
|---|---|
| `unknown flag: --scope` | Install a newer version; see [setup](@/docs/scoped-context.md#start-with-one-directory). |
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

A skill with a broken relative link still syncs. `agnostic-ai doctor --check-references` lists each broken link by source spec and destination, with every affected target on one line.

A link that works from the project root, such as `apps/engine/src/lib.ts`, counts as valid even when the skill folder lacks the file. Not every tool resolves links from the project root, so prefer a path the agent can open from where it runs. A link that leaves the project never counts.

| Cause | Fix |
|---|---|
| The linked file is in neither `.agnostic-ai/skills/<name>/` nor at that path from the project root | Add it, then run `agnostic-ai sync` |
| The link leaves the skill folder, such as `../shared/setup.md`, and no file sits at that path from the project root | Move the file into the skill folder and update the link. Sync copies only the skill's own folder |
| The tool flattens skills to one file and drops bundled files | Link to a URL, put the content in the skill, or accept the gap for that tool |
| You deleted a generated file by hand | Run `agnostic-ai sync` to restore it |
| A placeholder link in an example, such as `[Logs](url)`, can never resolve | List its destination under [`doctor.check-references.ignore`](@/docs/configuration.md#doctorcheck-referencesignore) |

</details>

## A synced skill is disabled via skillOverrides

A message such as `Skill "deploy" is disabled via skillOverrides` means your Claude Code settings switched the skill off, usually from the `/skills` menu. Sync still writes it. Run `agnostic-ai doctor` to list every synced skill your Claude settings turn off, file by file, then re-enable them in `/skills` or with `agnostic-ai doctor --fix` in a terminal. See [Claude skill overrides](@/docs/cli-reference/check.md#claude-skill-overrides).

## Hook timeouts

A message such as `UserPromptSubmit hook [...] timed out after 5s` comes from the coding tool. It does not say who installed the hook or why its command was slow.

In Claude Code, open `/hooks` and find the event and command named in the message. Check project settings, user settings, and enabled plugins. A command containing `${CLAUDE_PLUGIN_ROOT}` belongs to a plugin; update or diagnose that plugin, not your agnostic-ai specs. The [Claude Code hook reference](https://code.claude.com/docs/en/hooks) explains hook locations and timeouts.

For a hook generated from `.agnostic-ai/hooks/`, use [`why`](@/docs/trace.md) to find its source and [hook diagnostics](@/docs/cli-reference/hook-diagnostics.md) to inspect or test it. Make changes in the source, then sync.

Check whether an ordinary terminal command and the hook's runtime, such as `node --version`, are also slow. If several unrelated hooks time out together, check CPU load, memory pressure, and runtime startup first. Raise a timeout only once you know the command needs longer, because a stuck command then takes longer to report.

The [Caveman built-in](@/docs/rtk-and-caveman.md#if-you-already-use-rtk-or-caveman) supplies a skill, not the separate Caveman plugin's mode-tracking hooks.

## Report a problem

Include the CLI version, OS, failing command, full error, and the smallest config or spec that shows the problem. Remove credentials from MCP configuration and logs first. Open a [bug report](https://github.com/Chemaclass/agnostic-ai/issues/new?template=bug_report.yml).
