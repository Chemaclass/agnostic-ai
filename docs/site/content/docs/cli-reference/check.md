+++
title = "Check specs and output"
description = "Check specs and generated files: validate, lint, verify, doctor, and status."
weight = 30

[extra]
group = "Reference"
+++

# Check specs and output

## validate

Load all specs and report parse errors. On success it prints `loaded 12 entries. ok.`. With no specs it prints `loaded 0 entries. ok.` and stderr suggests `init` or `import`.

| Check | Reports |
|-------|------------|
| Hook events | A hook with no `event:`, or an event no configured target supports. The report lists the supported events. |
| Orphaned kinds | Hook or MCP specs that no enabled target reads. The line names the targets that would. |
| Declared sources | A `sources.<kind>` path in `agnostic-ai.yaml` with no directory. It prints a `note:` on stderr, loads the kind empty, and passes. |
| Entry-point fences | A `::target` or `::targets` name in `.agnostic-ai/AGNOSTIC_AI.md` that is neither a built-in target nor an external adapter listed in `targets`. Also a tool that reads no entry-point file (`cursor`, or any target with `outputs.<target>.rules-file`). |
| Global rules | With `--global`: a rule with scope, path, glob, or target conditions, which `sync --global` rejects. |
| Global settings | With `--global`: a settings `effort` that a tool cannot take, such as `max` for Claude. |

| Flag | Description |
|------|-------------|
| `--fix` | Rewrite source spec files to repair issues it can fix. |
| `--global` | Validate the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Hook events are checked against the tools `sync --global` writes hooks for, narrowed by a `targets` list in the home config. Works outside a project. |

Hook events accepted per target:

| Target | Events |
|--------|--------|
| Claude | `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification` |
| Codex | `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`, `PreCompact`, `PostCompact` |
| Gemini | `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection` |
| Cursor | `beforeShellExecution`, `afterShellExecution`, `beforeMCPExecution`, `afterMCPExecution`, `beforeReadFile`, `afterFileEdit`, `beforeSubmitPrompt`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `sessionStart`, `sessionEnd`, `subagentStart`, `subagentStop`, `preCompact`, `stop`, `afterAgentResponse`, `afterAgentThought`, `beforeTabFileRead`, `afterTabFileEdit`, `workspaceOpen` |
| Augment | `PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd` |
| Qoder | `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `Elicitation`, `ElicitationResult`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`, `Setup` |

## lint

Run checks beyond the schema. Exits 1 on errors.

```bash
agnostic-ai lint --strict
```

| Flag | Description |
|------|-------------|
| `--strict` | Exit 1 on warnings too, for CI. |
| `--suggest-capabilities` | Suggest neutral names for tool aliases (LINT037). Off by default. |
| `--global` | Lint the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Also reports LINT010 and LINT014. Budgets come from the `lint` key in the home config. Works outside a project. |
| `--json` | Print `{version, command, findings}` on stdout. Each finding has `code`, `severity` (`error` or `warn`), `path`, and `message`, like the `lint` list in `doctor --json`. Works with `--global`. |
| `--files <path>...` | Report only findings on these files, or on specs below a directory. `-` reads paths from stdin, one per line. A missing path, or no path, fails. Not with `--global`. The [spec guard hook](@/docs/spec-format/hooks.md#spec-guard) uses it after each edit. |

`agnostic-ai explain LINT011` prints a code's cause, fix, and config key.

| Code | Finding |
|------|---------|
| LINT001 | Warning. A spec has no body and no description. |
| LINT003 | Error. Two specs of one kind share a `name`. Hooks that share an event and matcher are fine. |
| LINT004 | Warning. No enabled target supports the spec's kind, so sync writes it nowhere. The finding lists the tools that do. |
| LINT005 | Warning. A hook sets `matcher` on an event that does not read one. Drop it or use a tool-call event such as `PreToolUse`. |
| LINT006 | Error. Frontmatter opens with `---` and never closes. |
| LINT010 | Error, `--global` only. A rule with scope, path, glob, or target conditions. |
| LINT014 | Error, `--global` only. A settings `effort` or `permissions.default-mode` that a tool's user settings cannot take, such as effort `max` for Claude or Copilot. `sync --global` drops it with a note. |
| LINT013 | Error. A rule's `globs` or `x-<target>.globs` is neither a string nor a list of strings. `validate` reports it too. |
| LINT016 | Error. A `dev-commands` entry in an environment spec has no `name:` or `command:`, repeats a name, is not a mapping, sets a key no tool reads, or gives `cwd`, `url`, `auto-port`, `port`, or `env` the wrong type. |
| LINT017 | Warning. A `gitignore.commit` entry such as `cursor:environments` names a target missing from `targets`. |
| LINT018 | Warning. A skill's frontmatter sets `scope`, which does nothing. The scope comes from its folder under `skills/`. |
| LINT019 | Warning. A skill or command uses Claude Code body syntax (`` !`command` ``, a ` ```! ` block, `$ARGUMENTS`, `$0`, `$1`) outside a `::target` fence, so an enabled tool reads it as text. See [Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax). |
| LINT020 | Warning. A rule sits in a folder that names a project directory, such as `rules/backend/`, but its `scope:` points outside it. `scope:` wins. Move the file or drop `scope`. |
| LINT021 | Warning. With Codex enabled: a Bash `allow` or `deny` rule in portable Settings or `outputs.claude.settings.permissions` has no Codex prefix that gives the same decision. `lint --strict` fails. See [Bash permission translation](@/docs/targets/codex.md#translate-bash-permissions). |
| LINT022 | Warning. A settings `protected` path covers a file sync writes, such as `.claude/**`. Protect the spec under `.agnostic-ai/` instead. `lint --strict` fails. See [protected paths](@/docs/spec-format/settings.md#protected-paths). |
| LINT023 | Error. A settings `protected` block is invalid: no `paths`, a `decision` other than `ask` or `deny`, an unknown key, or a path outside the project or with a character class, brace, or negation. `sync` fails too. |
| LINT024 | Warning. A [`coverage.accept`](@/docs/configuration.md#coverageaccept) entry matches no coverage note on one of its targets. Remove the entry or that target. `lint --strict` fails. |
| LINT025 | Warning. A [`models`](@/docs/configuration.md#models) tier a spec names has no entry and no `default` for an enabled tool that writes the spec, so it gets that tool's default model. Also raised for a tier named like a Claude model, such as `opus`. `lint --strict` fails. |
| LINT026 | Warning. A Claude model name reaches a tool that cannot load it, through an agent's `model` or the `default` of a tier it names. Write `model: {claude: <name>}` or add the tool to the tier. `lint --strict` fails. |
| LINT027 | Error. An MCP spec holds a value JSON cannot hold, such as a YAML `.nan` or `.inf`. The finding names the field, for example `x-amp.timeout`. `validate` reports it too, and `sync` fails. |
| LINT028 | Warning. An MCP `url` or `args` element holds a reference form only one tool reads, such as `${env:NAME}`, `{env:NAME}`, or `{% raw %}${{ secrets.NAME }}{% endraw %}`. Sync copies it as text to the other tools. Write `${NAME}`, or move the value under `x-<target>:`. `lint --strict` fails. |
| LINT029 | Warning. With Kiro and a tool that writes rules into `AGENTS.md` (such as Codex) enabled, a Kiro agent sets `x-kiro.resources` without `file://AGENTS.md`. With Kiro's `chat.disableInheritingDefaultResources` on, that agent loads no always-on rules. Add `file://AGENTS.md` to its resources. `lint --strict` fails. |
| LINT030 | Warning. A hook for Gemini CLI has a command with a bare `$GEMINI_PROJECT_DIR`, `$GEMINI_CWD`, `$GEMINI_PLANS_DIR`, `$GEMINI_SESSION_ID`, or `$CLAUDE_PROJECT_DIR`. Gemini replaces it as text before the shell runs. Write `"${NAME}"`. `lint --strict` fails. |
| LINT031 | Warning. A spec's `description` starts with `TODO`, the placeholder `agnostic-ai new` writes. It shows in tool pickers and decides when a skill fires. Replace it. `lint --strict` fails. |
| LINT032 | Error. A hook's `on:` or `match:` holds an unknown value, is mixed with `event:` or `matcher:`, names an event that an enabled tool it reaches does not read the same way (such as `on: stop` on Crush), or a tool kind that such a tool lacks (such as `match: read` on Codex). Sync leaves the hook out for that tool. See [portable events](@/docs/spec-format/hooks.md#portable-events). |
| LINT033 | Error. A spec body holds an [agent or skill reference](@/docs/spec-format/_index.md#agent-and-skill-references) to a name that does not exist, or that does not sync to a tool the spec reaches. The finding lists the known names or the tools. |
| LINT035 | Error. An MCP `env` or `headers` value is neither a `${NAME}` reference nor marked `!literal`, so it may be a secret that sync writes into every tool. The finding names the server, the field, and the key, never the value. `agnostic-ai migrate --only secrets` turns a credential into a reference and marks the rest. `sync` stops. See [secrets and plain settings](@/docs/spec-format/mcps.md#plain-settings). |
| LINT034 | Warning, project only. A hook's `event:` and `matcher:` have a portable form that gives every enabled tool the same native event and matcher, such as `event: PreToolUse` with `matcher: Bash`. The finding names the `on:` and `match:` to write. `agnostic-ai migrate --only hooks` rewrites these hooks. `lint --strict` fails. |
| LINT036 | Error. An agent's `can:` or skill's `allowed-tools` holds an unknown capability, a malformed `read(<path>)`, `edit(<path>)`, `shell(<pattern>)`, or `mcp:<server>`, or a value that is not a list. For an agent it also fires when `can:` sits beside `tools:` or under `x-<target>`. A settings permission rule fails when it is an unknown capability, has a pattern on one that takes none (such as `write(.env)`), has an empty pattern, or is not a string (such as an unquoted rule holding `: `). `validate` reports it too, and `sync` stops. See [capabilities](@/docs/spec-format/agents.md#capabilities) and [permission rules](@/docs/spec-format/settings.md#permission-rules). |
| LINT037 | Warning, only with `--suggest-capabilities`. A tool alias has an exact neutral capability form. |
| LINT038 | Warning. A bare capability under settings `permissions.allow` or `permissions.ask` covers a whole tool. The finding names the tools that take it and a scoped alternative. Scoped and `deny` rules do not warn. `lint --strict` fails. See [permission rules](@/docs/spec-format/settings.md#permission-rules). |
| LINT039 | Warning, project only. A [shared memory](@/docs/memory.md) index has more than 100 lines, or the two indexes pass 6,000 bytes. When a target loads memory through the [session-start hook](@/docs/memory.md#load-at-session-start), the finding names the first 10 facts that hook never loads; otherwise it names the targets that load the whole indexes every session. Only projects with the `memory` built-in get the size check. Merge or drop facts. `lint --strict` fails. |
| LINT040 | Error, project only. A memory index line links a fact file that does not exist. Restore the file, or run [`memory index`](@/docs/cli-reference/maintain.md#memory) to drop the line. |
| LINT041 | Warning, project only. A memory fact file has no index line, so no tool loads it. [`memory index`](@/docs/cli-reference/maintain.md#memory) adds one. `lint --strict` fails. |
| LINT042 | Error, project only. A line in a memory index or fact file looks like a credential, such as a prefixed token, a URL password, or a `Bearer` token. The finding names the line, never the value. |
| LINT008 | Error. A stdio MCP server lacks `command:`, or an `http`/`sse`/`ws` one lacks `url:`. `x-<target>` cannot set either. |

More warnings:

| Code | Finding |
|------|---------|
| LINT007 | A frontmatter key one edit away from a key agnostic-ai reads (`glob:` for `globs:`), so the setting is lost. `sync` prints the same warning. Put tool-native keys under `x-<target>:`. A key some tools read at the top level (Qoder's `glob:`, OpenCode's and Kilo's `mode:`) is flagged only when none of them is in `targets`. Settings and environment specs are not checked. |
| LINT009 | A permission that approves more than it says. An `allow` rule such as `Bash(git * main)` also matches a force push to main. Write the exact value, or keep `*` at the end (`Bash(go test:*)`). A `deny` rule with the same shape blocks nothing in Claude Code, so it is flagged too. |
| LINT012 | A skill or agent description (or `x-<target>.description`) is longer than [`lint.description-chars`](@/docs/configuration.md#lint) (default 1024). A skill description past 1024 warns even with a raised budget. |
| LINT015 | A spec body names another spec by a target-native path such as `.claude/skills/style/SKILL.md`. The finding names the `.agnostic-ai/` source path to use. |

LINT011 warns when a tool loads more words at session start than [`lint.instructions-words`](@/docs/configuration.md#lint) allows (default 2000). The count covers:

- the entry-point file as `sync` writes it
- always-on rule files
- every skill and agent description
- `@`-imported rule files

Tools with the same numbers share one line:

```
LINT011 [warn] AGENTS.md: cline, windsurf, trae load 2396 words every session: AGENTS.md 1187 (AGNOSTIC_AI.md 136, rules 1032), always-on rule files 908, skill descriptions 195, agent descriptions 106; budget 2000 (lint.instructions-words).
```

It also warns past a tool's byte cap. Codex stops reading `AGENTS.md` at 32 KiB (`project_doc_max_bytes`). Antigravity truncates rule files past 24,000 bytes.

{% <details summary="LINT011 for the Codex AGENTS.md chain"> %}
Codex also reads each scoped `AGENTS.md` between the root and its working directory, and stops at the same 32 KiB across the chain. LINT011 sums the chain per scope and warns past [`lint.codex-chain-bytes`](@/docs/configuration.md#lint) (default 32768). It names the scope, the total, and each file with its size. Setting a review spec to `target: cursor` takes its text out of Codex's `AGENTS.md`.

{% </details> %}

## verify

Run your own behavior check against each selected AI tool. Omit `--target` to check every configured target. It first runs `sync --check`, and a missing or stale generated file stops it.

```bash
agnostic-ai verify --target codex
```

The command in [`verify.command`](@/docs/configuration.md#verify) runs once per target, without a shell, and gets this JSON on stdin. Stdout and stderr pass through. A non-zero exit stops the run and becomes the `agnostic-ai` exit code.

| Field | Meaning |
|------|---------|
| `version` | Contract version, currently `1`. |
| `target` | Tool being verified, such as `"codex"`. |
| `configured_model` | Optional. Model in the generated config, not the one used at runtime. |
| `cli` | Optional. Detected CLI command, resolved path, and `--version` output. Omitted when the binary is missing. |
| `harness_fingerprint` | SHA-256 digest of the tool's specs and generated files. It identifies the setup. |

## doctor

Report four kinds of file:

- **missing**: never synced
- **stale**: out of date with the specs
- **edited**: changed since the last sync
- **orphaned**: no longer generated, kept because sync cannot prove it wrote the file

Doctor is read-only unless you pass `--fix`. It exits non-zero on any drift, [lint](#lint) error, or untrusted, modified, or unreadable Codex hook. Hooks you disabled on purpose do not fail. It also names any [spec migration](@/docs/cli-reference/maintain.md#migrate) that applies.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--fix` | Write missing, stale, and edited files, and remove each nested `CLAUDE.md` a rule already holds. In a terminal, it offers to remove each kept orphan, defaulting to no. Otherwise it keeps the orphans and says why. The exit stays non-zero while any remain. |
| `--backup` | With `--fix`, copy each existing file to `<path>.bak` before overwriting it or removing a confirmed orphan. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default. |
| `--check-references` | Flag relative Markdown links in generated skills whose file is missing on disk. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`, plus `lint`, `hook_trust`, and `packaging_ignore` lists, and `references` with `--check-references`. |

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
| **Global names** | A project skill or agent that shares its name with one in `~/.agnostic-ai/`. Per tool, it shows which wins. A name in [`sync.allow-global-names`](@/docs/configuration.md#syncallow-global-names) shows as allowed. See [shared names](@/docs/configuration.md#global-shared-names). | Never |
| **Instructions** | A hint when `AGNOSTIC_AI.md` still holds the long default text. Replace it with your own instructions. | Never |

{% <details summary="What the packaging check covers"> %}
The check handles literal paths, `*`, `?`, character classes, and `**` segments. Braces, extended globs, escaped patterns, and unsupported classes give a coverage-check warning, not an uncovered claim.

It reads only the root ignore file. It ignores `package.json` allowlists, built-in package exclusions, nested ignore files, and Dockerfile-specific ignore files. Verify with `npm pack --dry-run`, `vsce ls`, or your Docker build context.
{% </details> %}

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

Runtime approval always remains `unknown`. Managed settings, session flags, project trust, hook activation, and other handlers can change the actual outcome. The report describes declared rules rather than verifying a live host. Processor failures and unreadable settings fail the command. Warnings are diagnostic and do not grant approval or fail an otherwise successful inspection.

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
