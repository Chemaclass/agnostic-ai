+++
title = "Check specs and output"
description = "Check specs and generated files: validate, lint, verify, doctor, and status."
weight = 30

[extra]
nav_title = "Check output"
group = "Reference"
+++

# Check specs and output

Start troubleshooting with [`doctor`](#doctor): it combines project and local setup checks and ends with a next step. It is read-only unless you pass `--fix`.

Use [`validate`](#validate) for invalid spec formats, [`lint`](#lint) for spec quality, and [`sync --check`](@/docs/cli-reference/sync.md#sync) for generated files that differ from the sources. [`status`](#status) summarizes the project. [`verify`](#verify) runs your own tasks to check each AI tool's behavior.

`validate` and `lint` skip built-in hooks whose target selectors match no selected tool. User-authored hooks get the usual checks.

## validate

Load all specs and report parse errors. On success it prints `loaded 12 entries. ok.`. With no specs it prints `loaded 0 entries. ok.` and stderr suggests `init` or `import`.

| Check | Reports |
|-------|------------|
| Hook events | A hook with neither `on:` nor `event:`, or an event no configured target supports. The report lists the supported events. |
| Unused spec kinds | Hook or MCP specs that no enabled target reads. The line names the targets that would. |
| Declared sources | A `sources.<kind>` path in `agnostic-ai.yaml` with no directory. It prints a `note:` on stderr, loads the kind empty, and passes. |
| Tool-specific instruction blocks | A `::target` or `::targets` name in `.agnostic-ai/AGNOSTIC_AI.md` that is neither a built-in target nor an external adapter listed in `targets`. Also a tool that reads no entry-point file (`cursor`, or any target with `outputs.<target>.rules-file`). |
| Global rules | With `--global`: a rule with scope, path, glob, or target conditions, which `sync --global` rejects. |
| Global settings | With `--global`: a settings `effort` that a tool cannot take, such as `max` for Claude. |

| Flag | Description |
|------|-------------|
| `--fix` | Rewrite source spec files to repair issues it can fix. |
| `--global` | Validate the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Hook events are checked against the tools `sync --global` writes hooks for, narrowed by a `targets` list in the home config. Works outside a project. |

For event names, see [portable hook events](@/docs/spec-format/hook-reference.md#portable-events) and each [target page](@/docs/targets/_index.md). Use `on:` to share an event across tools.

## lint

Check spec quality after parsing. Exits 1 on errors. Each code, and the context budgets, are on [lint codes](@/docs/cli-reference/lint-codes.md); `agnostic-ai explain LINT011` prints one code's cause, fix, and config key.

```bash
agnostic-ai lint --strict
```

| Flag | Description |
|------|-------------|
| `--strict` | Exit 1 on warnings too, for CI. |
| `--suggest-capabilities` | Suggest neutral names for tool aliases (LINT037). Off by default. |
| `--global` | Lint the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Also reports LINT010 and LINT014. Budgets come from the `lint` key in the home config. Works outside a project. |
| `--json` | Print `{version, command, findings}` on stdout. Each finding has `code`, `severity` (`error` or `warn`), `path`, and `message`, like the `lint` list in `doctor --json`. Works with `--global`. |
| `--files <path>...` | Report only findings on these files, or on specs below a directory. `-` reads paths from stdin, one per line. A missing path, or no path, fails. Not with `--global`. The [spec guard hook](@/docs/spec-format/hook-reference.md#spec-guard) uses it after each edit. |

Fix the named source, rerun lint, then sync if generated output changes.

## verify

Run your own checks against each selected AI tool. Omit `--target` to check every configured target. It first runs `sync --check`; a missing or stale generated file stops it before any verifier runs or any result is written.

```bash
agnostic-ai verify --target codex
```

The command in [`verify.command`](@/docs/configuration.md#verify) runs once per target, without a shell, and gets this JSON on stdin. Stdout and stderr pass through. A non-zero exit stops the run and becomes the `agnostic-ai` exit code.

| Field | Meaning |
|------|---------|
| `version` | Input format version, currently `1`. |
| `target` | Tool being verified, such as `"codex"`. |
| `configured_model` | Optional. Model in the generated config, not the one used at runtime. |
| `cli` | Optional. Detected CLI command, resolved path, and `--version` output. Omitted when the binary is missing. |
| `harness_fingerprint` | SHA-256 checksum of the tool's specs and generated files. It identifies the setup. |

### Record and compare runs

Recording is optional. Pick a private destination and an evaluation identifier that names the same task suite, dataset revision, and evaluation rules for both runs:

```bash
agnostic-ai verify --target codex --evaluation smoke-v1 --record before.json
# Change an instruction, then regenerate the tool configuration.
agnostic-ai sync
agnostic-ai verify --target codex --evaluation smoke-v1 --record after.json
agnostic-ai verify compare before.json after.json [--json]
```

| Flag | Effect |
|------|--------|
| `--record <path>` | Save one run at this path. Parent directories must exist. |
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

A changed harness fingerprint names the configuration change, not a failure. The elapsed difference is the second duration minus the first (nanoseconds in JSON); one comparison does not show a statistical improvement. `verify compare` exits 0 when it can read both files, regressions included, and 1 on an invalid or unsupported file.

The files use schema `version: 1` with `started_at` (UTC RFC 3339), `evaluation`, `verifier_identity`, and `targets`. Each target holds the input fields above, `elapsed_ns`, `outcome`, and an optional `exit_code`. The outcome is `passed` (exit 0), `failed` (non-zero exit), `execution_error` (launch, cancellation, or context preparation failed), or `skipped` (not attempted after an earlier failure; no context or exit code, duration `0`). The CLI and model fields do not establish the model used at runtime.

`verifier_identity` hashes the resolved executable and the configured arguments. For a known interpreter, a direct script argument adds its code; options before the script, module loading, and inline code leave the identity unknown. Other file arguments do not change it. Change the evaluation identifier when tasks, datasets, remote dependencies, or rules change without changing the executable, arguments, or script.

Raw arguments, verifier output, and environment values are not saved; credentials in CLI and model metadata are redacted, and an evaluation identifier holding a detected credential is rejected. Files are written in one step with mode `0600`, which needs a filesystem with hard links for a new file. If saving fails, a failed evaluation keeps its exit code and a passing one exits 1; the save error is reported separately. Pass rates, token use, and cost are not inferred.

## doctor

```bash
agnostic-ai doctor
```

Read the findings and the final **Next step** before a repair or a focused check. The [troubleshooting guide](@/docs/troubleshooting.md) covers symptoms.

Doctor reports four kinds of generated file:

- **missing**: never synced
- **stale**: out of date with the specs
- **edited**: changed since the last sync
- **orphaned**: no longer generated, kept because sync cannot prove it wrote the file

It exits non-zero on any drift, [lint](#lint) error, or untrusted, modified, or unreadable Codex hook. Hooks you disabled on purpose do not fail. It also names any [spec migration](@/docs/cli-reference/upgrades.md#migrate) that applies.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--scope <all\|project>` | `all` (default) includes local Codex hook trust and your own Claude settings files. `project` skips them and keeps all project checks. |
| `--fix` | Write missing files and overwrite stale or edited generated files. Remove each nested `CLAUDE.md` whose text is already in a scoped rule. In a terminal, ask before removing each kept orphan and each group of [Claude skill overrides](#claude-skill-overrides), with No as the default. Without a terminal, keep them and explain why. Exit non-zero while any orphan remains. |
| `--backup` | With `--fix`, copy each file to `<path>.bak` before overwriting it, removing a confirmed orphan, or removing confirmed skill overrides. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default. |
| `--check-references` | Flag relative Markdown links in generated skills whose file is missing. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`, plus the lists below. |

On a fresh CI runner, run `agnostic-ai doctor --scope project --check-references`. Codex hook trust then shows `SKIPPED` in text, and JSON adds `hook_trust_check: {"status": "skipped", "reason": "project scope skips local runtime trust"}` with an empty `hook_trust`. `--scope project --fix` repairs drift without reading or approving trust. `--json` stays read-only.

`--check-references` reads each Markdown file a selected tool writes for its skills. A link is valid when it resolves from the document's directory or from the project root. It skips code, URLs, absolute paths, and `#fragment`-only links, and checks only the file in a `file#fragment` link. [`doctor.check-references.ignore`](@/docs/configuration.md#doctorcheck-referencesignore) exempts destinations that can never resolve. Any broken link fails doctor. Findings group by source spec and link:

```
Skill references:
  ✗ .agnostic-ai/skills/deploy/SKILL.md:8 links to missing references/setup.md
      targets: claude, codex
```

| `--json` list | Fields |
|------|--------|
| `references` | With `--check-references`: `target`, `source` (omitted when unknown), `path`, `line`, `destination`. |
| `lint` | `code`, `severity` (`error` or `warn`), `path`, `message`. |
| `hook_trust` | `path`, `event`, `group`, `handler`, `hook`, `status` (`untrusted`, `modified`, `disabled`, or `unknown`). An `unknown` entry adds `problem` with the failed check. |
| `packaging_ignore` | `path`, and either `uncovered` (generated paths) or `problem` (why coverage could not be checked). |
| `skill_overrides` | With `claude` selected, one item per `skillOverrides` entry, highest file first: `path` (absolute outside the project), `scope` (`local`, `shared`, or `user`), `skill` (the key as written), `value` (the string, or the JSON text of another value), `synced` (false for a skill sync does not write), and `effective` (false when a higher file sets the skill). An unreadable file has one item with `path`, `scope`, and `problem`. |

After the file report, doctor prints:

| Block | What it shows | Counts as drift |
|-------|---------------|-----------|
| **Spec health** | The findings `agnostic-ai lint` reports. An error fails doctor; a warning does not. | No |
| **Coverage notes** | With [`coverage.accept`](@/docs/configuration.md#coverageaccept) set, how many notes it accepts (`coverage_accepted` in `--json`). `sync -vv` lists them. | No |
| **Packaging ignores** | Generated paths that an existing root `.npmignore`, `.vscodeignore`, or `.dockerignore` does not cover, with the ignore file. | No, reported only |
| **Tracked despite ignored** | A generated path that git tracks and ignores, with the `git rm --cached` fix. | No |
| **Codex hook trust** | With Codex selected: inactive handlers in the project hooks file and user `hooks.json`, with `/hooks` as the next step. Reads trust from `CODEX_HOME/config.toml` (default `~/.codex/config.toml`). Untrusted, modified, or unreadable status fails; disabled does not. | Never grants trust |
| **MCP** | Whether each stdio `command:` is on PATH, with install hints. Then each `${NAME}` a server reads in `env`, `headers`, `url`, or `args` that is unset in this shell, by name only. A reference with a default is skipped. | No |
| **Claude hook specs** | With `claude` enabled: a hook spec with several commands that `.claude/settings.json` already runs in one group with different settings, as an older `import claude` wrote it, so each command runs twice. Delete the spec, run `sync`, then run `import claude`. | No |
| **Nested CLAUDE.md** | With `claude` enabled: a hand-written `<dir>/CLAUDE.md` whose text equals the body of a rule scoped to `<dir>`, as `import claude` leaves it. Claude Code would load it beside the synced rule. | Yes, `--fix` removes it |
| **Script divergence** | Scripts under `.agnostic-ai/scripts/<tool>/` with the same name but different bodies across tools, with the suggested path `.agnostic-ai/scripts/<basename>`. | Yes, no automatic fix |
| **Unmanaged config** | Markdown and TOML config files with no generated-file marker, grouped by the `import` source that adopts each. | No |
| **User-owned** | [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) entries, left out of Unmanaged config. | Never |
| **Global names** | A project skill or agent that shares its name with one in `~/.agnostic-ai/`, and which wins per tool. A name in [`sync.allow-global-names`](@/docs/configuration.md#syncallow-global-names) shows as allowed. See [shared names](@/docs/global-configuration.md#global-shared-names). | Never |
| **Instructions** | A hint when `AGNOSTIC_AI.md` still holds the long default text. | Never |
| **Claude skill overrides** | With `claude` selected: synced skills that `skillOverrides` in a Claude settings file turns off, hides from Claude, or lists without a description. See [below](#claude-skill-overrides). | Never |

<details>
<summary>What the packaging check covers</summary>

The check handles literal paths, `*`, `?`, character classes, and `**` segments. Braces, extended globs, escaped patterns, and unsupported classes give a coverage-check warning, not an uncovered claim.

It reads only the root ignore file. It ignores `package.json` allowlists, built-in package exclusions, nested ignore files, and Dockerfile-specific ignore files. Verify with `npm pack --dry-run`, `vsce ls`, or your Docker build context.

</details>

### Claude skill overrides

Claude Code's `/skills` menu saves each skill you switch off in your git-ignored `.claude/settings.local.json`. The skill still syncs, but Claude Code refuses to run it. Claude Code also reads `skillOverrides` from the shared `.claude/settings.json` and your user `~/.claude/settings.json` (under `CLAUDE_CONFIG_DIR` when set). Doctor lists the synced skills each file turns off, hides, or lists without a description, highest file first: local, shared, user.

```text
Claude skill overrides:
  ! .claude/settings.local.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: deploy
    → review them in Claude Code with /skills, run `agnostic-ai doctor --fix` in a terminal, or delete the entries from .claude/settings.local.json
  ! .claude/settings.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: lint
    → team policy: set skillOverrides in .agnostic-ai/overlays/claude.settings.json and run `agnostic-ai sync`
  ! ~/.claude/settings.json hides 1 of 3 synced skills from Claude (user-invocable-only): deploy
    → applies to every project on this machine; delete the entries from ~/.claude/settings.json
  ~ ~/.claude/settings.json also sets 4 entries for skills sync does not write
```

An entry is reported only when no higher file sets the same skill: a user file that turns `deploy` off is not reported when the local file turns it back `on`. Entries for skills sync does not write, such as your personal ones, are only counted. Skills whose generated files are under `sync.unmanaged` do not count as synced. `--json` lists every entry, with `effective` and `synced` as above.

The shared file is team policy, so its line points to the [overlay](@/docs/spec-format/overlays.md) sync writes it from, or is a `~` note when the overlay sets the same value.

Below the git root, Claude Code also reads the local file at the root (or the main checkout in a worktree), and its value wins for a key both set. Doctor lists it first, labelled `(git root)` or `(main checkout)`. Claude Code and doctor skip the root file on Windows and when the root is your home folder. Claude Code also skips it when you do not own the root or its `.git` or `.claude` entry; doctor does not check ownership.

`--scope project` reads only the shared file. An empty settings file counts as no settings. Managed (enterprise) settings and `--settings` are not read. Claude Code reads settings as strict JSON, and so does doctor:

| Case | Doctor reports | `--json` |
|------|----------------|----------|
| The file is not strict JSON (a comment or trailing comma), so Claude Code skips it | The file and the parse error | One item with `problem` |
| A value other than `on`, `off`, `user-invocable-only`, or `name-only`, which Claude Code rejects and may skip the whole file for | The value's JSON text. The file's other entries get a caution line and overrule nothing in a lower file. | `rejected: true`, and `uncertain: true` on the other entries |
| Two keys name one skill (`Deploy` and `deploy`) with different values, and Claude Code does not say which applies | Both keys. The file still overrules lower files. | `conflict: true` on both |
| Two keys name one skill with the same value | One key | The other has `effective: false` |

`doctor --fix` in a terminal asks once per state whether to re-enable those skills. A yes removes only their entries (both keys of a duplicate) from the project's `.claude/settings.local.json`, the only file it edits. When a lower file sets the same skill, the question names the value that then applies (or may apply, if that value is rejected). It skips a linked, hard-linked, read-only, or other user's file, one that sets `skillOverrides` twice, and one under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged). The file is replaced in one step, so a failed write leaves it as it was, and a file that changed while doctor ran is left alone.

Subcommands run one check:

| Subcommand | Check |
|------------|-------|
| `doctor config` | Validate `agnostic-ai.yaml` with the checks every command runs when it loads the project. |
| `doctor install` | Which AI CLIs are on PATH. |
| `doctor mcp` | Find each MCP server's command and list its unset `${NAME}` references. |
| `doctor rtk --command '<command>'` | Preview an RTK rewrite and compare declared Claude Code approval rules. Add `--json` for JSON. |

### RTK approval diagnostics

`doctor rtk --command 'git status'` calls the installed `rtk --version` and `rtk rewrite`, following [RTK's rewrite protocol](https://github.com/rtk-ai/rtk/blob/v0.51.0/src/hooks/rewrite_cmd.rs). It never runs the command or changes permissions. Missing RTK reports `missing`; an unsupported or already-prefixed command reports `unchanged`. RTK's exit 3 with a replacement (it requests approval) and exit 2 without one (it found a deny rule) are reported as results, with `processor_exit` in JSON. Other RTK failures and unreadable settings fail the command.

The report compares simple Bash rules declared in planned project settings (or native project settings when none is planned), project-local settings, and user settings (`CLAUDE_CONFIG_DIR` or `~/.claude`), using [Claude Code's precedence](https://code.claude.com/docs/en/permissions): deny over ask over allow. Exact rules, bare `Bash`, and simple trailing command prefixes are supported; shell wrappers, parameter rules, other shell syntax, and ambiguous patterns report `unknown`. A rewrite from a known ask to a known allow warns.

It lists known RTK `PreToolUse` handlers by settings path and warns when there is more than one. It recognizes direct Bash handlers that use `rtk rewrite` or RTK's Claude processor; plugins, indirect scripts, and other matchers can go unseen.

Runtime approval always reports `unknown`: administrator settings, session options, project trust, enabled hooks, and other handlers can change the outcome on a live host. Warnings do not grant approval or fail the command.

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

| JSON key | Type | Description |
|-----|------|-------------|
| `project` | string | Project directory base name. |
| `layers` | array | Active spec layers, each with `name` and `path`. |
| `specs` | object | Counts: `agents`, `skills`, `rules`, `hooks`, `mcps`. |
| `targets` | array | Targets in `agnostic-ai.yaml`. |
| `last_sync` | string or null | RFC 3339 time of the last successful `sync`. Falls back to the newest generated file's modified time. `null` (`unknown` in text) with no files. |
| `files_changed_last_sync` | number or null | Files written by the last sync. `null` when `last_sync` uses the modified-time fallback. |
| `drift_files` | number | Generated files that differ from what `sync` would write. |
