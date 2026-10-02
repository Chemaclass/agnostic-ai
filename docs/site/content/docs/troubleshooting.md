+++
title = "Troubleshooting"
description = "Diagnose configuration, sync, scope, and generated-output problems."
weight = 160

[extra]
group = "Reference"
+++

# Troubleshooting


Run commands from the directory containing `agnostic-ai.yaml`.

| Symptom | Next step |
|---|---|
| `agnostic-ai` is not found | Check the [install destination and PATH](@/docs/installation.md) |
| The version did not change after upgrading | Run `agnostic-ai upgrade --check` to find PATH shadowing |
| Config is missing | Change to the project root, or run `agnostic-ai init` for a new project |
| A new clone or worktree has no tool files | Run `agnostic-ai sync`; ignored outputs are absent from Git |
| An edited spec has no effect | Run `agnostic-ai list` to confirm it loads, then `agnostic-ai sync --dry-run` to see the planned output |
| A tool receives only some spec kinds | Check its [capabilities](@/docs/targets/_index.md#capability-matrix) and any unsupported-kind warnings |
| `sync --check` reports drift | Run `agnostic-ai sync`, review the result, and commit outputs if the project tracks them |
| CI fails on every fresh checkout | Match the [CI recipe](@/docs/ci.md) to whether generated outputs are committed |
| Two targets emit to the same path | Read [AAI-102](@/docs/errors.md#aai-102-targets-emit-to-the-same-output-path) and inspect output overrides |
| `sync` refuses to write a `.*ignore` file | Read [AAI-103](@/docs/errors.md#aai-103-hand-authored-ignore-file-cannot-be-safely-overwritten), import the patterns, and review their order and negations |
| A scoped rule is skipped, conflicts, or appears missing | Check [scoped-rule diagnostics](#scoped-rules) |
| A generated skill links to a file the agent cannot open | Run `agnostic-ai doctor --check-references`; see [broken skill references](#broken-skill-references) |
| Watch mode misses changes on a mounted filesystem | Try `agnostic-ai sync --watch --watch-poll` |

## Inspect the project

```bash
agnostic-ai status
agnostic-ai validate
agnostic-ai doctor
```

- `status` summarizes the project and reports drift. It always exits zero.
- `validate` checks source specs.
- `doctor` diagnoses configuration and output problems. It can exit non-zero.

See the [CLI reference](@/docs/cli-reference/_index.md) for flags and exit codes. To trace a generated file to its source, use [why](@/docs/trace.md). To see which targets receive a spec, use [graph](@/docs/graph.md). To explain a diagnostic code, run `agnostic-ai explain` with it, such as `agnostic-ai explain AAI-003`.

## Scoped rules

| Symptom | Next step |
|---|---|
| `unknown flag: --scope` | Install a build that has the feature; see [setup](@/docs/scoped-context.md#start-with-one-directory). |
| A rule is skipped or a filter cannot be preserved | Check [target support](@/docs/scoped-context.md#native-support) and [selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files). |
| Shared readers conflict or instructions differ | Use compatible targets with identical shared content, or separate worktrees. `--only` and `prefer-spec` do not bypass [scope conflicts](@/docs/scoped-context.md#shared-files-and-safe-updates). |
| A hand-authored file has a line no spec holds | Move the quoted line into its spec, or set the file aside and sync. Follow [migration](@/docs/migration.md#keep-directory-specific-instructions). |
| Alternate instructions conflict | Import and preserve the alternate file, then move it off its native filename. Follow [migration](@/docs/migration.md#keep-directory-specific-instructions). |
| An output override is rejected | Remove the named override and keep provenance headers enabled. |
| Cursor has no scoped `.mdc` file | It can share a nested `AGENTS.md` with Codex. Run `agnostic-ai graph --spec <name>`. |
| An old scope file remains | Run a full sync, then `sync --check`. Hand-authored files stay. |

## Broken skill references

A skill can sync cleanly while a relative link in it points at nothing. `agnostic-ai doctor --check-references` lists each broken link by source spec and destination, with every affected target on one line.

A link that resolves from the project root, such as `apps/engine/src/lib.ts`, counts as valid even when the skill folder lacks the file. That proves the file exists, not that every tool resolves links from the project root. Prefer a path the agent can open from where it runs. A link that leaves the project, such as `../shared/setup.md`, never counts.

| Cause | Fix |
|---|---|
| The linked file is in neither `.agnostic-ai/skills/<name>/` nor at that path from the project root | Add it, then run `agnostic-ai sync` |
| The link leaves the skill folder, such as `../shared/setup.md`, and no file sits at that path from the project root | Move the file into the skill folder and update the link. Sync copies only the skill's own folder |
| The target flattens skills to one file and drops bundled files | Link to a URL, inline the content, or accept the gap for that target |
| A generated reference was deleted by hand | Run `agnostic-ai sync` to restore it |
| A placeholder link in an example template, such as `[Logs](url)`, can never resolve | List its destination under [`doctor.check-references.ignore`](@/docs/configuration.md#doctorcheck-referencesignore) |

## Report a problem

Include the CLI version, OS, failing command, full error, and the smallest config or spec that reproduces it. Remove credentials from MCP configuration and logs first. Open a [bug report](https://github.com/Chemaclass/agnostic-ai/issues/new?template=bug_report.yml).
