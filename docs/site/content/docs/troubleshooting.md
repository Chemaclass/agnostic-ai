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

`status` summarizes the project and reports drift without a failing exit code. `validate` checks source specs. `doctor` diagnoses configuration and output problems and can exit non-zero. See [CLI reference](@/docs/cli-reference.md) for flags and exit behavior.

Use [why](@/docs/why.md) to trace a generated file to its source, or [graph](@/docs/graph.md) to see which targets receive a spec. For a diagnostic code, run `agnostic-ai explain AAI-003` with the code you were given.

## Scoped rules

| Symptom | Next step |
|---|---|
| `unknown flag: --scope` | Install a build that has the feature; see [setup](@/docs/scoped-context.md#start-with-one-directory). |
| A rule is skipped or a filter cannot be preserved | Check [target support](@/docs/scoped-context.md#native-support) and [selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files). |
| Shared readers conflict or instructions differ | Use compatible targets with identical shared content, or separate worktrees. `--only` and `prefer-spec` do not bypass [scope conflicts](@/docs/scoped-context.md#shared-files-and-safe-updates). |
| Hand-authored or alternate instructions conflict | Import and preserve the original, then move its native filename. Follow [migration](@/docs/migration.md#keep-directory-specific-instructions). |
| An output override is rejected | Remove the named override and keep provenance headers enabled. |
| Cursor has no scoped `.mdc` file | It can share a nested `AGENTS.md` with Codex. Run `agnostic-ai graph --spec <name>`. |
| An old scope file remains | Run a full sync, then `sync --check`. Hand-authored files stay. |

## Broken skill references

A skill can sync cleanly while a relative link in it points at nothing. `agnostic-ai doctor --check-references` lists each broken link with the target, the generated document and line, the missing destination, and the source spec.

| Cause | Fix |
|---|---|
| The linked file is missing from the skill folder under `.agnostic-ai/skills/<name>/` | Add it, then run `agnostic-ai sync` |
| The link leaves the skill folder, such as `../shared/setup.md` | Move the file into the skill folder and update the link. Sync copies only the skill's own folder |
| The target flattens skills to one file and drops bundled files | Link to a URL, inline the content, or accept the gap for that target |
| A generated reference was deleted by hand | Run `agnostic-ai sync` to restore it |

## Report a problem

Include the CLI version, OS, failing command, full error, and the smallest config or spec that reproduces it. Strip credentials from MCP configuration and logs first. Open a [bug report](https://github.com/Chemaclass/agnostic-ai/issues/new?template=bug_report.yml).
