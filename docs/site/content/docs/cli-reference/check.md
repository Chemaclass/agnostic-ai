+++
title = "Check specs and output"
description = "Validate, lint, verify, and diagnose specs and generated files."
weight = 30

[extra]
group = "Reference"
+++

# Check specs and output

## validate

Load all specs, report parse errors, and print `loaded 12 entries. ok.` on success. With no specs, stdout still says `loaded 0 entries. ok.` and stderr suggests `init` or `import`.

| Check | Reports |
|-------|------------|
| Hook events | A hook spec's `event:` missing, or supported by no configured target (with the supported list). |
| Orphaned kinds | Hook or MCP specs no enabled target consumes, one line per kind naming targets that would. |
| Declared sources | An explicit `sources.<kind>` path in `agnostic-ai.yaml` with no directory. Warning only. |
| Entry-point fences | A `::target` / `::targets` name in `.agnostic-ai/AGNOSTIC_AI.md` that is not a built-in target or listed in `targets` (external adapter), or that reads no entry-point file (`cursor`, or any target with `outputs.<target>.rules-file`). |
| Global rules | With `--global`, a rule with scope, path, glob, or target conditions, which `sync --global` rejects. |
| Global settings | With `--global`, a settings `effort` a target cannot take, such as `max` for Claude. |

| Flag | Description |
|------|-------------|
| `--fix` | Rewrite source spec files to repair autofixable issues. |
| `--global` | Validate the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Hook events are checked against the targets `sync --global` writes hooks for. A home config `targets` list narrows the checks. Works outside a project. |

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

Semantic checks beyond the schema. Exits 1 on error findings. It flags empty specs, dead specs (kinds no enabled target supports), and hooks that set a matcher on an event that ignores it.

```bash
agnostic-ai lint --strict
```

| Flag | Description |
|------|-------------|
| `--strict` | Exit 1 on warnings too, for CI. |
| `--global` | Lint the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Also reports LINT010 and LINT014. Budgets come from the home config's `lint` key. Works outside a project. |

| Code | Finding |
|------|---------|
| LINT003 | Two specs of one kind share a `name`; the loader keeps one body. Hooks sharing an event and matcher are fine. |
| LINT006 | Error. Frontmatter opens `---` and never closes, so the raw YAML is emitted as body. |
| LINT010 | Error, `--global` only. A rule with scope, path, glob, or target conditions, which `sync --global` rejects. |
| LINT014 | Error, `--global` only. A settings `effort` a target's user effort key cannot take, such as `max` for Claude or Copilot, which `sync --global` drops with a note. |
| LINT013 | Error. A rule's `globs` or `x-<target>.globs` is neither a string nor a list of strings. The rule loads in every session. `validate` reports it too. |
| LINT016 | Error. An environment spec's `dev-commands` entry has no `name:` or `command:`, repeats a name, is not a mapping, sets a key no target reads, or gives `cwd`, `url`, `auto-port`, `port`, or `env` the wrong type. `x-claude` overrides are checked too. |
| LINT017 | Warning. A `gitignore.commit` entry such as `cursor:environments` names a target that is not in `targets`, so it commits nothing. |
| LINT018 | Warning. A skill's frontmatter sets `scope`, which has no effect: a skill's scope comes from its folder under `skills/`. Use `workspaces` for extra Cursor copies. |
| LINT008 | Error. A stdio MCP server lacks `command:`, or an `http`/`sse`/`ws` one lacks `url:`. `x-<target>` cannot set either reserved field. |

LINT007 warns on a frontmatter key one edit away from a key agnostic-ai reads (`glob:` for `globs:`), since the setting is lost. `sync` prints the same warning. Put target-native keys under `x-<target>:`. A key some targets read at the top level (Qoder's `glob:`, OpenCode's and Kilo's `mode:`) is flagged only when none of those targets is in `targets`. Settings and environment specs are not checked.

LINT015 warns when a spec body names another spec by a target-native path such as `.claude/skills/style/SKILL.md`. The finding names the `.agnostic-ai/` source path to use.

LINT009 warns on a permission that approves more than it says. An `allow` rule such as `Bash(git * main)` also matches a force push to main. Write the exact value, or keep `*` at the end (`Bash(go test:*)`). A `deny` rule with the same shape blocks nothing in Claude Code, so it is flagged too. `ask` rules are skipped.

LINT011 warns when a target loads more words at session start than [`lint.instructions-words`](@/docs/configuration.md#lint) allows (default 2000). The count covers the entry-point file as `sync` writes it, always-on rule files, every skill and agent description, and `@`-imported rule files. The adapter decides what is always on. Targets with the same numbers share one line:

```
LINT011 [warn] AGENTS.md: cline, windsurf, trae load 2396 words every session: AGENTS.md 1187 (AGNOSTIC_AI.md 136, rules 1032), always-on rule files 908, skill descriptions 195, agent descriptions 106; budget 2000 (lint.instructions-words).
```

A target with a published byte cap also warns past it: Codex stops reading `AGENTS.md` at 32 KiB (`project_doc_max_bytes`), Antigravity truncates rule files past 24,000 bytes.

Codex also reads each scoped `AGENTS.md` between the root and the directory it works in, and stops at the same 32 KiB across the chain. LINT011 sums that chain per scope and warns past [`lint.codex-chain-bytes`](@/docs/configuration.md#lint) (default 32768). It names the scope, the total, and each file with its size and review section. A scope under one that already warns stays quiet. Setting a review spec to `target: cursor` takes its text out of Codex's `AGENTS.md`.

```
LINT011 [warn] apps/engine/src/integrations/AGENTS.md: Codex reads 47812 bytes in scope apps/engine/src/integrations, past 32768 (lint.codex-chain-bytes; Codex project_doc_max_bytes defaults to 32768): AGENTS.md 12907 (reviews 310), apps/engine/AGENTS.md 4531, apps/engine/src/integrations/AGENTS.md 18790 (reviews 18200). Codex drops what passes its limit.
```

LINT012 warns on a skill or agent description (or `x-<target>.description`) longer than [`lint.description-chars`](@/docs/configuration.md#lint) (default 1024). A skill description past 1024, the Agent Skills limit, warns even under a raised budget.

## verify

Run a project-owned behavior check against each selected AI harness (omit `--target` for all configured targets). It first runs the target-scoped `sync --check`; a missing or stale generated file stops verification.

```bash
agnostic-ai verify --target codex
```

The command in [`verify.command`](@/docs/configuration.md#verify) runs once per target, without a shell, and receives this JSON document on stdin. Stdout and stderr pass through. A non-zero verifier exit stops the run and becomes the `agnostic-ai` exit code.

| Field | Meaning |
|------|---------|
| `version` | Contract version, currently `1`. |
| `target` | Target being verified, such as `"codex"`. |
| `configured_model` | Optional. Model from the rendered native config, not the model used at runtime. |
| `cli` | Optional. Detected CLI command, resolved path, and `--version` output. Omitted when the binary is missing. |
| `harness_fingerprint` | Stable SHA-256 digest of target-relevant specs and rendered files. It identifies the harness; it is not a score. |

The verifier owns datasets, judging, results, and baselines.

## doctor

Report missing (never synced), stale (out of date with the specs), edited (changed since the last sync), and orphaned (no longer generated, kept because edited) files. Read-only unless `--fix`. Exits non-zero on any drift or [lint](#lint) error.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--fix` | Write missing, stale, and edited files. Orphans stay for you to delete, so the exit stays non-zero while any remain. |
| `--backup` | With `--fix`, copy each existing file to `<path>.bak` before overwriting. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default. |
| `--check-references` | Flag relative Markdown links in generated skills whose file is missing on disk. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`, plus a `lint` list. With `--check-references`, adds a `references` list. |

`--check-references` reads each Markdown document a selected target writes for its skills. A link is valid when it resolves from the document's own directory or, inside the project, from the project root. Code spans, code blocks, URLs, absolute paths, and `#fragment`-only links are skipped; only the file of a `file#fragment` link is checked. [`doctor.check-references.ignore`](@/docs/configuration.md#doctorcheck-referencesignore) exempts destinations that can never resolve. It exits non-zero on any broken link. Findings group by source spec and link:

```
Skill references:
  ✗ .agnostic-ai/skills/deploy/SKILL.md:8 links to missing references/setup.md
      targets: claude, codex
```

Each `references` entry in the JSON has `target`, `source` (omitted when unknown), `path`, `line`, and `destination`. Each `lint` entry has `code`, `severity` (`error` or `warn`), `path`, and `message`.

Then doctor prints:

| Block | What it shows | Counts as drift |
|-------|---------------|-----------|
| **Spec health** | The findings `agnostic-ai lint` reports. An error fails doctor; a warning shows without failing. With any finding, the next step points at `agnostic-ai lint`. | No |
| **Tracked despite ignored** | A generated path git tracks and ignores, with the `git rm --cached` command. | No |
| **MCP** | Whether each stdio `command:` resolves on PATH, with install hints. `url:`-only servers are skipped. | No |
| **Script divergence** | Basenames under `.agnostic-ai/scripts/<tool>/` whose bodies differ across tools, with the suggested path `.agnostic-ai/scripts/<basename>`. | Yes, not auto-fixable |
| **Unmanaged config** | Markdown and TOML config files without a provenance marker, grouped by the `import` source that adopts each. | No |
| **User-owned** | [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) entries, left out of Unmanaged config. | Never |

Subcommands run one check: `doctor config` (validate `agnostic-ai.yaml`), `doctor install` (which AI CLIs are on PATH), `doctor mcp` (resolve each MCP server's command binary).

## status

Show project configuration and sync state. Exits 0 even on drift; use `sync --check` or `doctor` in CI.

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

