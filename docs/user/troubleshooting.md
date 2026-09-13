# Troubleshooting

[User docs](README.md) · [Error codes](errors.md)

Run commands from the directory containing `agnostic-ai.yaml`.

| Symptom | Next step |
|---|---|
| `agnostic-ai` is not found | Check the [install destination and PATH](installation.md) |
| The version did not change after upgrading | Run `agnostic-ai upgrade --check` to find PATH shadowing |
| Config is missing | Change to the project root, or run `agnostic-ai init` for a new project |
| A new clone or worktree has no tool files | Run `agnostic-ai sync`; ignored outputs are absent from Git |
| An edited spec has no effect | Run `agnostic-ai list` to confirm it loads, then `agnostic-ai sync --dry-run` to inspect planned output |
| A tool receives only some spec kinds | Check its [capabilities](targets.md#capability-matrix) and any unsupported-kind warnings |
| `sync --check` reports drift | Run `agnostic-ai sync`, review the result, and commit outputs if the project tracks them |
| CI fails on every fresh checkout | Match the [CI recipe](ci.md) to whether generated outputs are committed |
| Two targets emit to the same path | Read [AAI-102](errors.md#aai-102-targets-emit-to-the-same-output-path) and inspect output overrides |
| `sync` refuses to write a `.*ignore` file | Read [AAI-103](errors.md#aai-103-hand-authored-ignore-file-cannot-be-safely-overwritten), import the patterns, and review their order and negations |
| A scoped rule is skipped, conflicts, or appears missing | Check [scoped-rule diagnostics](#scoped-rules) |
| Watch mode misses changes on a mounted filesystem | Try `agnostic-ai sync --watch --watch-poll` |

## Inspect the project

```bash
agnostic-ai status
agnostic-ai validate
agnostic-ai doctor
```

`status` summarizes the project and reports drift without a failing exit code. `validate` checks source specs. `doctor` diagnoses configuration and output problems and can exit non-zero. See [CLI reference](cli-reference.md) for flags and exit behavior.

Use [why](why.md) to trace a generated file to its source, or [graph](graph.md) to see which targets receive a spec. For a diagnostic code, run `agnostic-ai explain AAI-003`, replacing the code with the one reported.

## Scoped rules

| Symptom | Next step |
|---|---|
| `unknown flag: --scope` | Install a build containing the feature; see [setup](scoped-context.md#start-with-one-directory). |
| A rule is skipped or a filter cannot be preserved | Check [target support](scoped-context.md#native-support) and [selector limits](scoped-context.md#narrow-a-rule-to-certain-files). |
| Shared readers conflict or instructions differ | Use compatible targets and identical shared content, or separate worktrees. `--only` and `prefer-spec` do not bypass [scope conflicts](scoped-context.md#shared-files-and-safe-updates). |
| Hand-authored or alternate instructions conflict | Import and preserve the original, then move its native filename. Follow [migration](migration.md#keep-directory-specific-instructions). |
| An output override is rejected | Remove the named override and keep provenance headers enabled. |
| Cursor has no scoped `.mdc` file | It can share nested `AGENTS.md` with Codex. Run `agnostic-ai graph --spec <name>`. |
| An old scope file remains | Run a full sync, then `sync --check`. Hand-authored files are preserved. |

## Report a problem

Include the CLI version, OS, failing command, full error, and the smallest config/spec that reproduces the failure. Remove credentials from MCP configuration and logs before sharing. Open a [bug report](https://github.com/Chemaclass/agnostic-ai/issues/new?template=bug_report.yml).
