+++
title = "Check specs and output"
description = "Check specs and generated files: validate, lint, verify, doctor, and status."
weight = 30

[extra]
nav_title = "Check output"
group = "Reference"
+++

# Check specs and output

Start troubleshooting with [`doctor`](#doctor). It combines project checks with local setup checks and suggests a next step. Review the findings, then follow that step. Doctor is read-only by default; `doctor --fix` opts into repairs.

Use [`validate`](#validate) for invalid spec formats, [`lint`](#lint) for spec quality, and [`sync --check`](@/docs/cli-reference/sync.md#sync) for generated files that differ from the sources. [`status`](#status) summarizes the project. [`verify`](#verify) runs your own tasks to check the AI tool's behavior.

`validate` and `lint` skip built-in hooks whose target selectors match no selected tool. User-authored hooks still receive the usual checks.

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

For event names and mappings, see [portable hook events](@/docs/spec-format/hook-reference.md#portable-events) and each [target page](@/docs/targets/_index.md). Use `on:` when sharing an event across tools.

## lint

Check spec quality after parsing. Exits 1 on errors.

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

`agnostic-ai explain LINT011` prints a code's cause, fix, and config key.

See [lint codes](@/docs/cli-reference/lint-codes.md) for the complete findings and context budgets. Fix the named source, rerun lint, then sync if generated output changes.

## verify

Run your own checks against each selected AI tool. Omit `--target` to check every configured target. It first runs `sync --check`, and a missing or stale generated file stops it.

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

Recording is optional. Choose a private destination and an evaluation identifier that names the same task suite, dataset revision, and evaluation rules for both runs:

```bash
agnostic-ai verify --target codex --evaluation smoke-v1 --record before.json
# Change an instruction, then regenerate the tool configuration.
agnostic-ai sync
agnostic-ai verify --target codex --evaluation smoke-v1 --record after.json
agnostic-ai verify compare before.json after.json
agnostic-ai verify compare before.json after.json --json
```

A passing run followed by a failing run reports `regression`. A failing run followed by a passing run reports `recovery`. Matching outcomes report `unchanged`; a target present in only one file reports `missing_target`. Changed harness fingerprints identify the configuration change and do not themselves mean failure. Elapsed differences are candidate duration minus baseline duration, measured in nanoseconds in JSON. One comparison does not establish a statistical improvement.

| Flag | Effect |
|------|--------|
| `--record <path>` | Save one run at this explicit destination. Parent directories must exist. |
| `--evaluation <identifier>` | Name the task suite and dataset revision. Required with `--record`. |
| `--overwrite` | Allow an existing result file to be replaced. Requires `--record`. |

The files use schema `version: 1`. Run fields are `started_at` (UTC RFC 3339), `evaluation`, `verifier_identity`, and `targets`. Each target contains the input fields above, `elapsed_ns`, `outcome`, and an optional `exit_code`. Outcomes are `passed` (exit 0), `failed` (non-zero evaluation exit), `execution_error` (launch, cancellation, or context preparation failure), and `skipped` (not attempted after an earlier failure). Skipped targets have no measured context or exit code; their duration is `0`. CLI and configured model fields remain optional; they do not establish the runtime model.

`verifier_identity` hashes the resolved executable bytes and configured arguments. For known interpreters, a direct script argument also contributes its code bytes; options before the script, module loading, and inline-code forms leave the identity unknown. Other file arguments are evaluation inputs or outputs and their contents do not change verifier identity. Raw arguments, verifier output, and environment values are not saved. Detected CLI and configured model metadata use credential redaction. Keep credentials out of the evaluation identifier; recording rejects an identifier that contains a detected credential. You must change the evaluation identifier when tasks, datasets, remote dependencies, or evaluation rules change without changing the executable, arguments, or recognized script. An unknown or changed verifier identity, different evaluation identifier, or skipped target reports `non_comparable`. Launch failures report `execution_error`. Both human and JSON comparison are reports and exit 0 when valid files can be read, including regressions and non-comparable runs. Invalid or unsupported result files exit 1.

Result files are installed atomically with mode `0600`; an existing file requires `--overwrite`. Use a filesystem that supports hard links when creating a new file. The drift check stops before a verifier runs or a result is written. Evaluation failures keep their original exit code even when saving also fails; saving errors are reported separately. A successful evaluation with a recording error exits 1. No task pass rates, token use, or cost are inferred from command exits.

## doctor

```bash
agnostic-ai doctor
```

Read the findings and the final **Next step** before running a repair or a focused check. See the [troubleshooting guide](@/docs/troubleshooting.md) for symptom-specific paths.

Report four kinds of file:

- **missing**: never synced
- **stale**: out of date with the specs
- **edited**: changed since the last sync
- **orphaned**: no longer generated, kept because sync cannot prove it wrote the file

Doctor is read-only unless you pass `--fix`. It exits non-zero on any drift, [lint](#lint) error, or untrusted, modified, or unreadable Codex hook. Hooks you disabled on purpose do not fail. It also names any [spec migration](@/docs/cli-reference/upgrades.md#migrate) that applies.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--scope <all\|project>` | `all` (default) includes local Codex hook trust and your own Claude settings files. `project` skips them and keeps all project checks. |
| `--fix` | Write missing files and overwrite stale or manually edited generated files. Remove each nested `CLAUDE.md` whose text is already in a scoped rule. In a terminal, ask before removing each kept orphan, and before removing each group of [Claude skill overrides](#claude-skill-overrides), with No as the default. Without a terminal, keep orphans and overrides and explain why. Exit non-zero while any orphan remains. |
| `--backup` | With `--fix`, copy each existing file to `<path>.bak` before overwriting it, removing a confirmed orphan, or removing confirmed skill overrides. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default. |
| `--check-references` | Flag relative Markdown links in generated skills whose file is missing on disk. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`, plus `lint`, `hook_trust`, and `packaging_ignore` lists, and `references` with `--check-references`. |

For a fresh CI runner, run `agnostic-ai doctor --scope project --check-references`. Text marks Codex hook trust as `SKIPPED`. JSON adds `hook_trust_check: {"status": "skipped", "reason": "project scope skips local runtime trust"}` and leaves `hook_trust` empty. This distinguishes a skipped check from one with no findings. `--scope project --fix` repairs project drift without reading or approving trust. `--json` remains read-only. Packaging warnings keep their advisory status.

`--check-references` reads each Markdown file a selected tool writes for its skills. A link is valid when it resolves from the document's directory or from the project root. It skips code, URLs, absolute paths, and `#fragment`-only links, and checks only the file in a `file#fragment` link. [`doctor.check-references.ignore`](@/docs/configuration.md#doctorcheck-referencesignore) exempts destinations that can never resolve. Doctor exits non-zero on any broken link. Findings group by source spec and link:

```
Skill references:
  ✗ .agnostic-ai/skills/deploy/SKILL.md:8 links to missing references/setup.md
      targets: claude, codex
```

Entries in the `--json` report have these fields:

| List | Fields |
|------|--------|
| `references` | `target`, `source` (omitted when unknown), `path`, `line`, `destination`. |
| `lint` | `code`, `severity` (`error` or `warn`), `path`, `message`. |
| `hook_trust` | `path`, `event`, `group`, `handler`, `hook`, `status` (`untrusted`, `modified`, `disabled`, or `unknown`). An `unknown` entry adds `problem` with the failed check. |
| `packaging_ignore` | `path`, and either `uncovered` (generated paths) or `problem` (why coverage could not be checked). |
| `skill_overrides` | With `claude` selected, one item per `skillOverrides` entry, highest file first: `path`, `scope` (`local`, `shared`, or `user`), `skill` (the key as written), `value` (the string set, or the JSON text of another value), `synced` (false for a skill sync does not write), and `effective` (false when a higher file sets the same skill). A file that could not be read or parsed has one item with only `path`, `scope`, and `problem`. Paths outside the project are absolute. |

Then doctor prints:

| Block | What it shows | Counts as drift |
|-------|---------------|-----------|
| **Spec health** | The findings `agnostic-ai lint` reports. An error fails doctor. A warning does not. | No |
| **Coverage notes** | With [`coverage.accept`](@/docs/configuration.md#coverageaccept) set, how many notes it accepts (`coverage_accepted` in `--json`). `sync -v` lists them. | No |
| **Packaging ignores** | Generated paths that an existing root `.npmignore`, `.vscodeignore`, or `.dockerignore` does not cover, with the ignore file and paths. | No, reported only |
| **Tracked despite ignored** | A generated path that git tracks and ignores, with the `git rm --cached` fix. | No |
| **Codex hook trust** | With Codex selected: inactive handlers in the project hooks file and user `hooks.json`, with `/hooks` as the next step. Reads trust from `CODEX_HOME/config.toml` (default `~/.codex/config.toml`). Untrusted, modified, or unreadable status fails. Disabled does not. | Never grants trust |
| **MCP** | Whether each stdio `command:` is on PATH, with install hints. Then each `${NAME}` a server reads in `env`, `headers`, `url`, or `args` that is unset in this shell, by name only. A reference with a default is skipped. | No |
| **Claude hook specs** | With `claude` enabled: a hook spec with several commands that `.claude/settings.json` already runs in one group with different settings, as an older `import claude` wrote it. Each command runs twice. The fix is to delete the spec, run `sync`, then run `import claude`. | No |
| **Nested CLAUDE.md** | With `claude` enabled: a hand-written `<dir>/CLAUDE.md` whose text equals the body of a rule scoped to `<dir>`, as `import claude` leaves it. Claude Code would load it beside the synced rule. | Yes, `--fix` removes it |
| **Script divergence** | Scripts under `.agnostic-ai/scripts/<tool>/` with the same name but different bodies across tools, with the suggested path `.agnostic-ai/scripts/<basename>`. | Yes, no automatic fix |
| **Unmanaged config** | Markdown and TOML config files with no generated-file marker, grouped by the `import` source that adopts each. | No |
| **User-owned** | [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) entries, left out of Unmanaged config. | Never |
| **Global names** | A project skill or agent that shares its name with one in `~/.agnostic-ai/`. Per tool, it shows which wins. A name in [`sync.allow-global-names`](@/docs/configuration.md#syncallow-global-names) shows as allowed. See [shared names](@/docs/global-configuration.md#global-shared-names). | Never |
| **Instructions** | A hint when `AGNOSTIC_AI.md` still holds the long default text. Replace it with your own instructions. | Never |
| **Claude skill overrides** | With `claude` selected: synced skills that `skillOverrides` in a Claude settings file turns off, hides from Claude, or lists without a description. See [Claude skill overrides](#claude-skill-overrides). | Never |

<details>
<summary>What the packaging check covers</summary>

The check handles literal paths, `*`, `?`, character classes, and `**` segments. Braces, extended globs, escaped patterns, and unsupported classes give a coverage-check warning, not an uncovered claim.

It reads only the root ignore file. It ignores `package.json` allowlists, built-in package exclusions, nested ignore files, and Dockerfile-specific ignore files. Verify with `npm pack --dry-run`, `vsce ls`, or your Docker build context.

</details>

### Claude skill overrides

Claude Code's `/skills` menu saves each skill you switch off in `.claude/settings.local.json`, a file that is yours and that git ignores. A skill switched off there still syncs, but Claude Code refuses to run it. Claude Code also reads `skillOverrides` from the shared `.claude/settings.json` and from your user `~/.claude/settings.json` (under `CLAUDE_CONFIG_DIR` when set). Doctor reads each of these files and lists the synced skills each one affects, by state, highest file first: local, shared, user.

```text
Claude skill overrides:
  ! .claude/settings.local.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: deploy
  ! .claude/settings.local.json hides 1 of 3 synced skills from Claude (user-invocable-only): review
    → review them in Claude Code with /skills, run `agnostic-ai doctor --fix` in a terminal, or delete the entries from .claude/settings.local.json
  ! .claude/settings.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: lint
    → team policy: set skillOverrides in .agnostic-ai/overlays/claude.settings.json and run `agnostic-ai sync`
  ! ~/.claude/settings.json hides 1 of 3 synced skills from Claude (user-invocable-only): deploy
    → applies to every project on this machine; delete the entries from ~/.claude/settings.json
  ~ ~/.claude/settings.json also sets 4 entries for skills sync does not write
```

Doctor reports an entry only when no higher file sets the same skill, since the higher file's value applies. A user file that turns `deploy` off is not reported when the project's local file turns it back `on`. `--json` lists every entry and marks the hidden ones `effective: false`.

The shared file is team policy, so its line points to the [overlay](@/docs/spec-format/overlays.md) that sync writes it from. When the overlay sets the same value, the line is a `~` note that names the overlay, not a warning.

When the project sits below the git root, Claude Code reads the local file at the root, or at the main checkout in a worktree. Doctor reads that file too, lists it first, and labels it `(git root)` or `(main checkout)`. Where both local files set a key, Claude Code applies the root file's value. Claude Code skips the root file on Windows and when the root is your home folder, and so does doctor. Claude Code also skips it when you do not own the root or its `.git` or `.claude` entry; doctor does not check ownership and still reports it.

`doctor --fix` in a terminal asks once per state whether to re-enable those skills. A yes removes only their entries and keeps the rest of the file as it was. When a lower file sets the same skill, the question and the result name the value that then applies, and doctor says the skill is re-enabled only when nothing lower changes it. If that lower file holds a rejected value, they say its value may apply, since Claude Code may skip that file. Doctor never changes the file without a yes. It does not offer the change for a linked, hard-linked, read-only, or other user's file, one that sets `skillOverrides` twice, or one listed under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged). It replaces the file in one step, so a failed write leaves it as it was, and leaves it alone if it changed while doctor ran. Skills whose generated files are under `sync.unmanaged` do not count as synced. `--fix` edits only the project's `.claude/settings.local.json`, never the other files. Entries for skills sync does not write, such as your personal ones, are never removed or named: a file with a finding counts them, and `--json` lists them with `synced: false`. `--scope project` reads only the shared file. An empty settings file counts as no settings. Managed (enterprise) settings and `--settings` are not read.

Claude Code reads settings files as strict JSON, so doctor does too: a comment or a trailing comma makes Claude Code skip the whole file, and doctor reports the file with the parse error. A value other than `on`, `off`, `user-invocable-only`, or `name-only` is rejected by Claude Code, which may then skip the whole file. Doctor reports it with its JSON text. Since the file may be skipped, its other entries get a caution line and overrule nothing in a lower file. When two keys in one file name the same skill, such as `Deploy` and `deploy`, with different values, Claude Code does not say which applies: doctor reports both keys, the file still overrules lower files, and `--fix` asks to remove both keys. With the same value, one counts and the other is marked `effective: false`; `--fix` removes both. In `--json`, a rejected value has `rejected: true`, conflicting keys have `conflict: true`, and the other entries of a file with a rejected value have `uncertain: true`.

Subcommands run one check:

| Subcommand | Check |
|------------|-------|
| `doctor config` | Validate `agnostic-ai.yaml`. |
| `doctor install` | Which AI CLIs are on PATH. |
| `doctor mcp` | Find each MCP server's command and list its unset `${NAME}` references. |
| `doctor rtk --command '<command>'` | Preview an RTK rewrite and compare declared Claude Code approval rules. Add `--json` for the same report as JSON. |

### RTK approval diagnostics

`doctor rtk --command 'git status'` calls the installed `rtk --version` and `rtk rewrite` processors. It never executes the supplied command or changes permissions. Missing RTK reports `missing`; an unsupported or already-prefixed command reports `unchanged`.

RTK can return a replacement with exit 3 when its own permission inspection requests approval, or exit 2 without a replacement when it finds a deny rule. Both are diagnostic outcomes, not processor failures. The JSON report includes `processor_exit`; this inspection remains separate from the live host's approval decision. Other processor failures stop the diagnostic.

The report compares simple declared Bash rules from planned project settings, project-local settings, and user settings (`CLAUDE_CONFIG_DIR` or `~/.claude`). When no project settings file is planned, it reads native project settings instead. Exact rules, bare `Bash`, and simple trailing command prefixes are supported. Shell wrappers, parameter rules, other shell syntax, and ambiguous Bash patterns report `unknown`. Deny takes precedence over ask and allow. A rewrite from a known ask decision to a known allow decision produces a warning.

Known RTK PreToolUse handlers are listed by settings path. More than one produces an ownership warning. This inspection recognizes direct Bash handlers using RTK rewrite or its Claude processor; plugins, indirect scripts, and other matcher expressions can remain undiscovered.

Runtime approval always remains `unknown`. Administrator settings, session options, project trust, enabled hooks, and other handlers can change the actual outcome. The report describes declared rules; it does not verify a live host. Processor failures and unreadable settings fail the command. Warnings are diagnostic and do not grant approval or fail an otherwise successful inspection.

The processor interface follows [RTK's rewrite protocol](https://github.com/rtk-ai/rtk/blob/v0.51.0/src/hooks/rewrite_cmd.rs), and rule precedence follows [Claude Code permissions](https://code.claude.com/docs/en/permissions).

## status

Show project configuration and sync state. It exits 0 even on drift. In CI, use `sync --check` or `doctor`.

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
