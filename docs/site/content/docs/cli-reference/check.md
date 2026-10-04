+++
title = "Check specs and output"
description = "Validate, lint, verify, and diagnose specs and generated files."
weight = 30

[extra]
group = "Reference"
+++

# Check specs and output

## validate

Load all specs and report parse errors. On success it prints `loaded 12 entries. ok.`. With no specs, stdout says `loaded 0 entries. ok.` and stderr suggests `init` or `import`.

| Check | Reports |
|-------|------------|
| Hook events | A hook spec with no `event:`, or an event no configured target supports. The report lists the supported events. |
| Orphaned kinds | Hook or MCP specs that no enabled target consumes. One line per kind names the targets that would. |
| Declared sources | An explicit `sources.<kind>` path in `agnostic-ai.yaml` with no directory. It prints a `note:` on stderr. The kind loads empty and the run passes, since a fresh clone has no empty directories. |
| Entry-point fences | A `::target` or `::targets` name in `.agnostic-ai/AGNOSTIC_AI.md` that is neither a built-in target nor listed in `targets` (external adapter). Also a name whose target reads no entry-point file (`cursor`, or any target with `outputs.<target>.rules-file`). |
| Global rules | With `--global`: a rule with scope, path, glob, or target conditions. `sync --global` rejects it. |
| Global settings | With `--global`: a settings `effort` that a target cannot take, such as `max` for Claude. |

| Flag | Description |
|------|-------------|
| `--fix` | Rewrite source spec files to repair autofixable issues. |
| `--global` | Validate the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Hook events are checked against the targets `sync --global` writes hooks for. A `targets` list in the home config narrows the checks. Works outside a project. |

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

Run semantic checks beyond the schema. It exits 1 on errors. It flags empty specs, dead specs (kinds no enabled target supports), and hooks that set a matcher on an event that ignores it.

```bash
agnostic-ai lint --strict
```

| Flag | Description |
|------|-------------|
| `--strict` | Exit 1 on warnings too, for CI. |
| `--global` | Lint the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Also reports LINT010 and LINT014. Budgets come from the `lint` key in the home config. Works outside a project. |
| `--json` | Print `{version, command, findings}` on stdout. Each finding has `code`, `severity` (`error` or `warn`), `path`, and `message`, the shape of the `lint` list in `doctor --json`. The exit status matches the text output. Works with `--global`. |

`agnostic-ai explain LINT011` prints a code's cause, fix, and config key. `lint` prints that command after its findings.

| Code | Finding |
|------|---------|
| LINT001 | Warning. A spec has no body and no description. |
| LINT003 | Error. Two specs of one kind share a `name`. The loader keeps one body. Hooks that share an event and matcher are fine. |
| LINT004 | Warning. No enabled target supports the spec's kind, so sync writes it nowhere. The finding lists the targets that do. |
| LINT005 | Warning. A hook sets `matcher` on an event that does not consume one, so the tool ignores it. Drop it or use a tool-call event such as `PreToolUse`. |
| LINT006 | Error. Frontmatter opens with `---` and never closes, so the raw YAML is emitted as body. |
| LINT010 | Error, `--global` only. A rule with scope, path, glob, or target conditions. `sync --global` rejects it. |
| LINT014 | Error, `--global` only. A settings `effort` or `permissions.default-mode` that a target's user settings cannot take, such as effort `max` for Claude or Copilot. `sync --global` drops it with a note. |
| LINT013 | Error. A rule's `globs` or `x-<target>.globs` is neither a string nor a list of strings, so the rule loads in every session. `validate` reports it too. |
| LINT016 | Error. A `dev-commands` entry in an environment spec has no `name:` or `command:`, repeats a name, is not a mapping, sets a key no target reads, or gives `cwd`, `url`, `auto-port`, `port`, or `env` the wrong type. `x-claude` overrides are checked too. |
| LINT017 | Warning. A `gitignore.commit` entry such as `cursor:environments` names a target missing from `targets`, so it commits nothing. |
| LINT018 | Warning. A skill's frontmatter sets `scope`, which has no effect. A skill's scope comes from its folder under `skills/`. Use `workspaces` for extra Cursor copies. |
| LINT019 | Warning. A skill or command line uses Claude Code body syntax (`` !`command` ``, a ` ```! ` block, `$ARGUMENTS`, or `$0`, `$1`, ...) outside a `::target` fence, and an enabled target reads it as plain text. The finding names the line and the targets. See [Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax). |
| LINT020 | Warning. A rule sits in a folder that names a project directory, such as `rules/backend/`, but its `scope:` points outside it. The frontmatter wins. Move the file or drop `scope`. |
| LINT021 | Warning. With Codex enabled: a supported Bash `allow` or `deny` rule in portable Settings or `outputs.claude.settings.permissions` has no explicit Codex prefix that covers it with the same effective decision, including portable deny and ask exclusions. Restrictive descendants also warn for an allowed prefix. Checks inline, YAML file, and imported policy sources. `lint --strict` fails. See [Bash permission translation](@/docs/targets/codex.md#translate-bash-permissions). |
| LINT022 | Warning. A settings `protected` path covers a file sync writes, such as `.claude/**`. Sync regenerates that file from its source spec, so protect the spec under `.agnostic-ai/` instead. A path that also covers its own settings spec, such as `**`, is skipped. A target that fails to render gets its own LINT022, and the others are still checked. `lint --strict` fails. See [protected paths](@/docs/spec-format/settings.md#protected-paths). |
| LINT023 | Error. A settings `protected` block is invalid: no `paths`, a `decision` other than `ask` or `deny`, an unknown key, or a path outside the project or with a character class, brace, or negation. `sync` fails on the same block. |
| LINT024 | Warning. A [`coverage.accept`](@/docs/configuration.md#coverageaccept) entry matches no coverage note on one of its targets, for example because the target now supports the field. Remove the entry or that target. A target that fails to load or emit gets its own LINT024 naming the error, and its entries are not checked. `lint --strict` fails. |
| LINT025 | Warning. A [`models`](@/docs/configuration.md#models) tier that an agent, skill, command, or settings spec names has no entry and no `default` for an enabled target that writes the spec, so the spec gets that tool's default model. An effort-only tier is not checked. Also raised for a tier named like a Claude model, such as `opus`, since `model: opus` then names the tier. The editor server reports it too. `lint --strict` fails. |
| LINT026 | Warning. A Claude model name reaches a target that cannot load it, through an agent's shared `model` (a scalar or `default`) or through the `default` of a tier the agent names. Write `model: {claude: <name>}` or add the target to the tier. `lint --strict` fails. |
| LINT027 | Error. An MCP spec holds a value JSON cannot hold, such as a YAML `.nan` or `.inf`. The finding names the field, for example `x-amp.timeout`. `validate` reports it too. `sync` fails on it instead of leaving the server out. |
| LINT028 | Warning. An MCP `url` or `args` element holds a reference form only one tool reads, such as `${env:NAME}`, `{env:NAME}`, or `{% raw %}${{ secrets.NAME }}{% endraw %}`. Sync copies it as text to each enabled tool that does not read that form, which the finding names. Only the field sync writes for the transport is checked. Write `${NAME}`, or move the value under `x-<target>:`. Older imports wrote these. `lint --strict` fails. |
| LINT029 | Warning. With Kiro and an inlining target such as codex enabled, a Kiro agent sets `x-kiro.resources` without `file://AGENTS.md`. The always-on rules reach Kiro only through `AGENTS.md`, which custom agents inherit by default. With Kiro's `chat.disableInheritingDefaultResources` on, that agent loads none of them. Add `file://AGENTS.md` to its resources. `lint --strict` fails. |
| LINT030 | Warning. A hook that emits to Gemini CLI has a command with a bare `$GEMINI_PROJECT_DIR`, `$GEMINI_CWD`, `$GEMINI_PLANS_DIR`, `$GEMINI_SESSION_ID`, or `$CLAUDE_PROJECT_DIR`. Gemini replaces each bare name as text before the shell runs, so shell syntax around it, or a project path holding another such name, can turn the value into code. Write `"${NAME}"`, which Gemini leaves to the shell. `lint --strict` fails. |
| LINT031 | Warning. A spec's `description` starts with `TODO`, the placeholder `agnostic-ai new` writes for agents, skills, rules, hooks, and MCP servers. Sync copies it to every target, where it shows in tool pickers and decides when a skill fires. Replace it with what the spec is for. `lint --strict` fails. |
| LINT032 | Error. A hook's `on:` or `match:` holds an unknown value, is mixed with `event:` or `matcher:`, names an event that an enabled target it reaches does not read the same way, such as `on: stop` on Crush, or names a tool kind that an enabled target it reaches has no tool for, such as `match: read` on Codex. Sync leaves the hook out on that target. See [portable events](@/docs/spec-format/hooks.md#portable-events). |
| LINT033 | Error. A spec body holds an [agent or skill reference](@/docs/spec-format/_index.md#agent-and-skill-references), and no agent or skill in the project has that name, or that agent or skill does not sync to a target the spec reaches. The finding lists the known names or the targets. |
| LINT034 | Warning, project only. A hook's `event:` and `matcher:` have a portable form that gives every enabled target the hook reaches the same native event and matcher, such as `event: PreToolUse` with `matcher: Bash` on Claude Code and Codex. The finding names the `on:` and `match:` to write. `agnostic-ai migrate --only hooks` rewrites exactly these hooks. `lint --strict` fails. |
| LINT035 | Error. An agent's `can:` holds an unknown capability, a malformed `shell(<pattern>)` or `mcp:<server>`, or a value that is not a list, sits beside `tools:`, or sits under `x-<target>`. `validate` reports it too, and `sync` stops instead of writing the agent without its restriction. See [capabilities](@/docs/spec-format/agents.md#capabilities). |
| LINT008 | Error. A stdio MCP server lacks `command:`, or an `http`/`sse`/`ws` one lacks `url:`. `x-<target>` cannot set either reserved field. |

More warnings:

| Code | Finding |
|------|---------|
| LINT007 | A frontmatter key one edit away from a key agnostic-ai reads (`glob:` for `globs:`), so the setting is lost. `sync` prints the same warning. Put target-native keys under `x-<target>:`. A key that some targets read at the top level (Qoder's `glob:`, OpenCode's and Kilo's `mode:`) is flagged only when none of those targets is in `targets`. Settings and environment specs are not checked. |
| LINT009 | A permission that approves more than it says. An `allow` rule such as `Bash(git * main)` also matches a force push to main. Write the exact value, or keep `*` at the end (`Bash(go test:*)`). A `deny` rule with the same shape blocks nothing in Claude Code, so it is flagged too. `ask` rules are skipped. |
| LINT012 | A skill or agent description (or `x-<target>.description`) is longer than [`lint.description-chars`](@/docs/configuration.md#lint) (default 1024). A skill description past 1024, the Agent Skills limit, warns even under a raised budget. |
| LINT015 | A spec body names another spec by a target-native path such as `.claude/skills/style/SKILL.md`. The finding names the `.agnostic-ai/` source path to use. |

LINT011 warns when a target loads more words at session start than [`lint.instructions-words`](@/docs/configuration.md#lint) allows (default 2000). The count covers:

- the entry-point file as `sync` writes it
- always-on rule files
- every skill and agent description
- `@`-imported rule files

The adapter decides what is always on. Targets with the same numbers share one line:

```
LINT011 [warn] AGENTS.md: cline, windsurf, trae load 2396 words every session: AGENTS.md 1187 (AGNOSTIC_AI.md 136, rules 1032), always-on rule files 908, skill descriptions 195, agent descriptions 106; budget 2000 (lint.instructions-words).
```

A target with a published byte cap also warns past it. Codex stops reading `AGENTS.md` at 32 KiB (`project_doc_max_bytes`). Antigravity truncates rule files past 24,000 bytes.

{% <details summary="LINT011 for the Codex AGENTS.md chain"> %}
Codex also reads each scoped `AGENTS.md` between the root and the directory it works in, and stops at the same 32 KiB across the chain. LINT011 sums that chain per scope and warns past [`lint.codex-chain-bytes`](@/docs/configuration.md#lint) (default 32768). It names the scope, the total, and each file with its size and review section. A scope under one that already warns stays quiet. Setting a review spec to `target: cursor` takes its text out of Codex's `AGENTS.md`.

```
LINT011 [warn] apps/engine/src/integrations/AGENTS.md: Codex reads 47812 bytes in scope apps/engine/src/integrations, past 32768 (lint.codex-chain-bytes; Codex project_doc_max_bytes defaults to 32768): AGENTS.md 12907 (reviews 310), apps/engine/AGENTS.md 4531, apps/engine/src/integrations/AGENTS.md 18790 (reviews 18200). Codex drops what passes its limit.
```
{% </details> %}

## verify

Run your own behavior check against each selected AI harness. Omit `--target` to check every configured target. It first runs `sync --check` for those targets. A missing or stale generated file stops it.

```bash
agnostic-ai verify --target codex
```

The command in [`verify.command`](@/docs/configuration.md#verify) runs once per target, without a shell. It receives this JSON document on stdin. Stdout and stderr pass through. A non-zero verifier exit stops the run and becomes the `agnostic-ai` exit code.

| Field | Meaning |
|------|---------|
| `version` | Contract version, currently `1`. |
| `target` | Target being verified, such as `"codex"`. |
| `configured_model` | Optional. Model from the rendered native config, not the model used at runtime. |
| `cli` | Optional. Detected CLI command, resolved path, and `--version` output. Omitted when the binary is missing. |
| `harness_fingerprint` | Stable SHA-256 digest of target-relevant specs and rendered files. It identifies the harness. It is not a score. |

The verifier owns datasets, judging, results, and baselines.

## doctor

Report four kinds of file:

- **missing**: never synced
- **stale**: out of date with the specs
- **edited**: changed since the last sync
- **orphaned**: no longer generated, kept because ownership could not be proven

Doctor is read-only unless you pass `--fix`. It exits non-zero on any drift, [lint](#lint) error, or untrusted or modified Codex hook. Unreadable hook trust state also fails. Hooks you disabled on purpose are reported but do not fail. It also names any [spec migration](@/docs/cli-reference/maintain.md#migrate) that applies, without failing.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--fix` | Write missing, stale, and edited files, and remove each nested `CLAUDE.md` a rule already holds. In a terminal, it offers to remove each kept orphan that sync recorded, defaulting to no. Outside a terminal, or when a configured target cannot be loaded, it keeps the orphans and says why. The exit stays non-zero while any remain. |
| `--backup` | With `--fix`, copy each existing file to `<path>.bak` before overwriting or confirmed orphan removal. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default. |
| `--check-references` | Flag relative Markdown links in generated skills whose file is missing on disk. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`, plus `lint`, `hook_trust`, and `packaging_ignore` lists. With `--check-references`, adds a `references` list. |

`--check-references` reads each Markdown file a selected target writes for its skills. A link is valid when it resolves from the document's own directory or, inside the project, from the project root. It skips code spans, code blocks, URLs, absolute paths, and `#fragment`-only links. For a `file#fragment` link it checks only the file. [`doctor.check-references.ignore`](@/docs/configuration.md#doctorcheck-referencesignore) exempts destinations that can never resolve. Doctor exits non-zero on any broken link. Findings group by source spec and link:

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
| **Spec health** | The findings `agnostic-ai lint` reports. An error fails doctor. A warning does not. The next step points at `agnostic-ai lint`. | No |
| **Coverage notes** | With [`coverage.accept`](@/docs/configuration.md#coverageaccept) set, how many notes it accepts. `sync -v` lists them. `doctor --json` has the count as `coverage_accepted`. | No |
| **Packaging ignores** | Generated paths that an existing root `.npmignore`, `.vscodeignore`, or `.dockerignore` does not cover. Names the ignore file and paths. Unsupported patterns and read errors are reported separately. | No, advisory only |
| **Tracked despite ignored** | A generated path git tracks and ignores, with the `git rm --cached` command. | No |
| **Codex hook trust** | With Codex selected: inactive handlers in the project hooks file and user `hooks.json`, with `/hooks` as the next step. Reads only user trust from `CODEX_HOME/config.toml` (default `~/.codex/config.toml`). Untrusted, modified, or unreadable status fails. Disabled status does not. | Never grants trust |
| **MCP** | Whether each stdio `command:` resolves on PATH, with install hints. `url:`-only servers are skipped. Then each `${NAME}` a server reads in `env`, `headers`, `url`, or `args` that is unset in this shell, by name only. A reference with a default is skipped. | No |
| **Nested CLAUDE.md** | With `claude` enabled: a hand-written `<dir>/CLAUDE.md` whose trimmed text equals the body of a rule scoped to `<dir>`, as `import claude` leaves it. Claude Code loads it beside the synced rule. A file whose text differs from every such rule is never listed. | Yes, `--fix` removes it |
| **Script divergence** | Basenames under `.agnostic-ai/scripts/<tool>/` whose bodies differ across tools, with the suggested path `.agnostic-ai/scripts/<basename>`. | Yes, not auto-fixable |
| **Unmanaged config** | Markdown and TOML config files with no provenance marker, grouped by the `import` source that adopts each. | No |
| **User-owned** | [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) entries, left out of Unmanaged config. | Never |
| **Global names** | A project skill or agent that shares its name with one in `~/.agnostic-ai/`. Per target, it shows where one hides the other and which wins. A name in [`sync.allow-global-names`](@/docs/configuration.md#syncallow-global-names) shows as allowed. See [shared names](@/docs/configuration.md#global-shared-names). | Never |
| **Instructions** | A hint when `AGNOSTIC_AI.md` still holds the long default text an earlier release seeded. Replace it with your own instructions. | Never |

Packaging coverage uses the real outputs of the selected targets, including configured paths and skill assets. It checks missing outputs too, so the warning can show before sync. Doctor never changes packaging ignore files, and packaging warnings do not change its exit code.

{% <details summary="What the packaging check covers"> %}
The check handles literal paths, `*`, `?`, character classes, and `**` path segments, with each format's anchoring and negation rules. Braces, extended globs, escaped patterns, and unsupported classes produce a coverage-check warning instead of a claim that paths are uncovered.

It checks only the existing root ignore file. It does not predict package contents or process `package.json` allowlists, built-in package exclusions, nested ignore files, or Dockerfile-specific ignore files. Verify the result with `npm pack --dry-run`, `vsce ls`, or your Docker build context.
{% </details> %}

Subcommands run one check:

| Subcommand | Check |
|------------|-------|
| `doctor config` | Validate `agnostic-ai.yaml`. |
| `doctor install` | Which AI CLIs are on PATH. |
| `doctor mcp` | Resolve each MCP server's command binary and list its unset `${NAME}` references. |

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
| `last_sync` | string or null | RFC 3339 time of the last successful `sync`, from `.agnostic-ai/.sync-state`. Falls back to the newest generated file mtime; `null` (`unknown` in text) with no files. |
| `files_changed_last_sync` | number or null | Files written by the last sync; `null` under the mtime fallback. |
| `drift_files` | number | Emitted files that differ from what `sync` would produce. |

