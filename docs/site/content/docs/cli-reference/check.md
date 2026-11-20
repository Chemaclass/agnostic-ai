+++
title = "Check specs and output"
description = "Check specs and generated files: validate, lint, verify, and status."
weight = 30

[extra]
nav_title = "Check output"
group = "Reference"

[extra.moved]
doctor = "@/docs/cli-reference/doctor.md#doctor"
claude-skill-overrides = "@/docs/cli-reference/doctor.md#claude-skill-overrides"
rtk-approval-diagnostics = "@/docs/cli-reference/doctor.md#rtk-approval-diagnostics"
+++

# Check specs and output

## Doctor {#doctor .legacy-anchor}

## Claude skill overrides {#claude-skill-overrides .legacy-anchor}

## RTK approval diagnostics {#rtk-approval-diagnostics .legacy-anchor}

Pick the command for the problem:

| Problem | Run |
|---------|-----|
| Anything is wrong, and you do not know what | [`doctor`](@/docs/cli-reference/doctor.md#doctor): checks project and local setup, then suggests a next step. Read-only unless you pass `--fix`. |
| A spec has an invalid format | [`validate`](#validate) |
| Spec quality | [`lint`](#lint) |
| Generated files differ from the sources | [`sync --check`](@/docs/cli-reference/sync.md#sync) |
| A summary of the project | [`status`](#status) |
| Your own tasks, run against each tool | [`verify`](#verify) |

`validate` and `lint` skip built-in hooks whose target selectors match no selected tool. User-authored hooks get the usual checks.

## validate

Load all specs and report parse errors. On success it prints `loaded 12 entries. ok.`. With no specs it prints `loaded 0 entries. ok.` and stderr suggests `init` or `import`.

| Flag | Description |
|------|-------------|
| `--fix` | Rewrite source spec files to repair issues it can fix. |
| `--global` | Validate the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Works outside a project. |

{% <details summary="Details: what validate checks, and global mode"> %}
| Check | Reports |
|-------|------------|
| Hook events | A hook with neither `on:` nor `event:`, or an event no target the hook emits to supports, after its `x-<target>` event. The report lists the supported events. |
| Unused spec kinds | Hook, MCP, command, settings, review, environment, or ignore specs that no enabled target reads. The line names the targets that would. `new` names them too when it writes such a spec. |
| Duplicate names | Two specs of one kind with the same `name` in one layer. Only the last one loaded reaches any tool, so validate fails and `sync` prints a warning naming both files. A project spec that overrides a pack, or a `local/` spec that extends a project spec, is not a duplicate. |
| Case-only output clashes | On a filesystem where letter case does not count, such as macOS or Windows: two outputs whose paths differ only in case, such as the files of rules `b` and `B`, which land on one file. Validate fails with [AAI-102](@/docs/errors.md#aai-102-targets-emit-to-the-same-output-path) and names both paths, as `sync` would. To find them it renders every target, which makes it slower when spec names or scopes differ only in case, or when `agnostic-ai.yaml` sets an output path. |
| Declared sources | A `sources.<kind>` path in `agnostic-ai.yaml` with no directory. It prints a `note:` on stderr, loads the kind empty, and passes. |
| Tool-specific instruction blocks | A `::target` or `::targets` name in `.agnostic-ai/AGNOSTIC_AI.md` that is neither a built-in target nor an external adapter listed in `targets`. Also a tool that reads no entry-point file (`cursor`, or any target with `outputs.<target>.rules-file`). |
| Global rules | With `--global`: a rule with scope, path, glob, or target conditions, which `sync --global` rejects. |
| Global settings | With `--global`: a settings `effort` that a tool cannot take, such as `max` for Claude. |

- With `--global`, hook events are checked against the tools `sync --global` writes hooks for, narrowed by a `targets` list in the home config.
- For event names, see [portable hook events](@/docs/spec-format/hook-reference.md#portable-events) and each [target page](@/docs/targets/_index.md). Use `on:` to share an event across tools.
{% </details> %}

## lint

Check spec quality after parsing. Exits 1 on errors. Each code is on [lint codes](@/docs/cli-reference/lint-codes.md); `agnostic-ai explain LINT011` prints one code's cause, fix, and config key.

```bash
agnostic-ai lint --strict
```

Fix the named source, rerun lint, then sync if generated output changes.

| Flag | Description |
|------|-------------|
| `--strict` | Exit 1 on warnings too, for CI. |
| `--suggest-capabilities` | Suggest neutral names for tool aliases (LINT037). Off by default. |
| `--global` | Lint the specs in `$AGNOSTIC_AI_HOME` and its `local/` overrides. Works outside a project. |
| `--json` | Print `{version, command, findings}` on stdout. Works with `--global`. |
| `--files <path>...` | Report only findings on these files, or on specs below a directory. |

{% <details summary="Details: --global, --json, and --files"> %}
- `--global` also reports LINT010 and LINT014. Budgets come from the `lint` key in the home config. `$AGNOSTIC_AI_HOME` defaults to `~/.agnostic-ai`.
- Each `--json` finding has `code`, `severity` (`error` or `warn`), `path`, and `message`, like the `lint` list in `doctor --json`.
- `--files -` reads paths from stdin, one per line. A missing path, or no path, fails. Not with `--global`.
- The [spec guard hook](@/docs/spec-format/hook-reference.md#spec-guard) uses `--files` after each edit.
{% </details> %}

## verify

Run your own checks against each selected AI tool. Omit `--target` to check every configured target.

```bash
agnostic-ai verify --target codex
```

`verify` first runs `sync --check`. A missing or stale generated file stops it before any verifier runs or any result is written.

The command in [`verify.command`](@/docs/configuration.md#verify) runs once per target, without a shell. It gets the JSON below on stdin. Stdout and stderr pass through. A non-zero exit stops the run and becomes the `agnostic-ai` exit code.

| Field | Meaning |
|------|---------|
| `version` | Input format version, currently `1`. |
| `target` | Tool being verified, such as `"codex"`. |
| `configured_model` | Optional. Model in the generated config, not the one used at runtime. |
| `cli` | Optional. Detected CLI command, resolved path, and `--version` output. Omitted when the binary is missing. |
| `harness_fingerprint` | SHA-256 checksum of the tool's specs and generated files. It identifies the setup. |

### Record and compare runs

Recording is optional. Save two runs, then compare them. Use one evaluation identifier for both. It must name the same task suite, dataset revision, and evaluation rules.

```bash
agnostic-ai verify --target codex --evaluation smoke-v1 --record before.json
# Change an instruction, then regenerate the tool configuration.
agnostic-ai sync
agnostic-ai verify --target codex --evaluation smoke-v1 --record after.json
agnostic-ai verify compare before.json after.json [--json]
```

| Flag | Effect |
|------|--------|
| `--record <path>` | Save one run at this path. Parent directories must exist. Pick a private destination. |
| `--evaluation <identifier>` | Name the task suite and dataset revision. Required with `--record`. |
| `--overwrite` | Allow replacing an existing result file. Requires `--record`. |

Each target in a comparison gets one result:

| Result | When |
|--------|------|
| `regression` | The first run passed and the second failed. |
| `recovery` | The first run failed and the second passed. |
| `unchanged` | Both runs have the same outcome. |
| `missing_target` | The target is in only one file. |
| `non_comparable` | The verifier identity is unknown or changed, the evaluation identifiers differ, or the target was skipped. |
| `execution_error` | The verifier failed to launch. |

`verify compare` exits 0 when it can read both files, regressions included. It exits 1 on an invalid or unsupported file.

{% <details summary="Details: comparison output, file format, identity, and privacy"> %}
- A changed harness fingerprint names the configuration change, not a failure.
- The elapsed difference is the second duration minus the first (nanoseconds in JSON). One comparison does not show a statistical improvement.
- The files use schema `version: 1` with `started_at` (UTC RFC 3339), `evaluation`, `verifier_identity`, and `targets`. Each target holds the input fields above, `elapsed_ns`, `outcome`, and an optional `exit_code`.
- The outcome is `passed` (exit 0), `failed` (non-zero exit), `execution_error` (launch, cancellation, or context preparation failed), or `skipped` (not attempted after an earlier failure; no context or exit code, duration `0`).
- The CLI and model fields do not establish the model used at runtime.
- `verifier_identity` hashes the resolved executable and the configured arguments. For a known interpreter, a direct script argument adds its code. Options before the script, module loading, and inline code leave the identity unknown. Other file arguments do not change it.
- Change the evaluation identifier when tasks, datasets, remote dependencies, or rules change without changing the executable, arguments, or script.
- Raw arguments, verifier output, and environment values are not saved. Credentials in CLI and model metadata are redacted, and an evaluation identifier holding a detected credential is rejected.
- Files are written in one step with mode `0600`, which needs a filesystem with hard links for a new file.
- If saving fails, a failed evaluation keeps its exit code and a passing one exits 1. The save error is reported separately.
- Pass rates, token use, and cost are not inferred.
{% </details> %}

## status

Show project configuration and sync state. It exits 0 even on drift; in CI, use `sync --check` or `doctor`.

```bash
agnostic-ai status [--json]
```

```
Project: my-project
Layers:  project (.agnostic-ai/)
Specs:   3 rules, 1 agent, 2 skills
Targets: claude, cursor, copilot
Last sync: 2026-05-07 14:22 (5 files changed)
Drift:   in sync
```

{% <details summary="Details: --json keys"> %}
| JSON key | Type | Description |
|-----|------|-------------|
| `project` | string | Project directory base name. |
| `layers` | array | Active spec layers, each with `name` and `path`. |
| `specs` | object | Counts: `agents`, `skills`, `rules`, `hooks`, `mcps`. |
| `targets` | array | Targets in `agnostic-ai.yaml`. |
| `last_sync` | string or null | RFC 3339 time of the last successful `sync`. Falls back to the newest generated file's modified time. `null` (`unknown` in text) with no files. |
| `files_changed_last_sync` | number or null | Files written by the last sync. `null` when `last_sync` uses the modified-time fallback. |
| `drift_files` | number | Generated files that differ from what `sync` would write. |
{% </details> %}
