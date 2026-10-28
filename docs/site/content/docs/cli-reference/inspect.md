+++
title = "Inspect and preview"
description = "See where a spec goes, compare tools, preview a spec, and trace a generated file."
weight = 40
aliases = ["/docs/trace/", "/docs/why/", "/docs/graph/"]

[extra]
nav_title = "Inspect and preview"
group = "Reference"
+++

# Inspect and preview

Start with the item you know: `explain <spec>` finds a spec's output, `why <file>` finds a generated file's source, and `render <spec>` previews file contents. Use `compare` before changing tools and `graph` for an overview.

## explain

List every output file and section one spec contributes to, read from the `<!-- source: ... -->` markers in merged documents. With `--file`, list the instructions that apply to one source file instead. Given an [error code](@/docs/errors.md) such as `AAI-003` or a [lint code](@/docs/cli-reference/check.md#lint) such as `LINT011`, print its cause and fix; a lint code adds its severity and any `lint` config key that tunes it (`severity` and `config` in JSON).

```bash
agnostic-ai explain rules/conventional-commits.md --json
agnostic-ai explain builtin:handoff --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable JSON for editor extensions and scripts. |
| `--layers` | Show which project source layers replace or extend a spec. See [below](#debug-a-replaced-spec). |
| `--global` | Explain a spec in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) or its `local/` layer. A relative path resolves against that root. |
| `--file <path>` | Explain a source file. See [below](#explain-a-source-file). |
| `--context` | Rank startup context. See [below](#rank-context-contributions). |
| `--inputs` | List generator inputs. See [below](#list-generator-inputs). |

Contributions are grouped by configured target, each tagged `(full file)` or `(section "<name>")`, plus a "would emit if enabled" list for other tools. With `--global`, an agent or skill is a whole file, a rule a section of the instructions file, a hook its event in the hooks file, an MCP server its key in the user MCP file, and a settings spec each key it sets, tagged `(key "<key>")`.

A spec that sets `model` or `effort` also lists its [tier](@/docs/spec-format/agents.md#model-tiers) and the model and effort it resolves to per target; `tool default` means the target writes no model. A target with no key for it, such as Codex skills, drops it and `sync` prints a coverage note.

For an agent's `can` or `tools`, and settings `permissions`, it lists each target's native names, unsupported rules included. A widening line names extra access a target grants (for Codex, from the merged permissions and any explicit command policy source); `explain` shows it even with `on-unsupported: error`, where `sync` fails. Tool-specific overrides show their values and the field that wins.

<details>
<summary>Read explain JSON</summary>

```json
{"version": "1", "command": "explain", "spec": {"kind": "rule", "name": "...", "path": "..."},
 "contributions": [{"target": "...", "path": "...", "section": "...", "mode": "full|section|key"}],
 "would_emit_if_enabled": [],
 "model_tier": "strong", "models": [{"target": "...", "model": "...", "effort": "..."}]}
```

`model_tier` and `models` are omitted when the spec sets neither `model` nor `effort`. The `capabilities` array, omitted for specs without capability fields, holds `target`, `field`, `capability`, `native`, and `supported`; `widening` lists extra access when present, and `override` names a native field or policy source that replaces the portable rule.

</details>

### Claude skill overrides

For a skill sync writes for Claude Code, `explain` lists each Claude settings file whose `skillOverrides` names the skill, highest precedence first. It reads the same files and applies the same rules as [`doctor`](@/docs/cli-reference/check.md#claude-skill-overrides), including how rejected values, duplicate keys, and files that are not strict JSON show. A line that does not take effect is marked, so the first unmarked line is the value Claude Code applies:

```text
claude overrides:
  .claude/settings.local.json: off (Claude Code refuses to run it; change it in /skills)
  ~/.claude/settings.json: name-only (not applied: a higher file sets it)
```

The block is omitted when no file names the skill and every file reads. `--json` adds the same entries under `overrides`, with `rejected`, `conflict`, `uncertain`, or `problem` as in doctor:

```json
"overrides": [{"target": "claude", "path": ".claude/settings.local.json", "scope": "local", "value": "off", "effective": true}]
```

### Debug a replaced spec

When editing a pack rule has no effect, inspect its source layers:

```bash
agnostic-ai explain rules/conventional-commits.md --layers
```

```text
rule conventional-commits
  .agnostic-ai/packs/team/rules/conventional-commits.md [pack:team] base (replaced)
  rules/conventional-commits.md [project] replace (contributor)
  .agnostic-ai/local/rules/conventional-commits.md [project-user] extend (contributor)
effective source: .agnostic-ai/local/rules/conventional-commits.md
```

Edit either contributor to change the effective spec. Passing the replaced pack path gives the same report. Sources follow sync's order: enabled built-ins, installed packs, project specs, then project-local overrides. `base` starts a spec, `replace` discards the earlier chain, and `extend` merges with it. `contributor` means the source survives resolution; it does not attribute fields or body text, which an extension can replace.

With `--json`, `spec` is the effective source and `sources` lists each candidate's `source`, `operation`, and final `role`. Built-ins show a symbolic `builtin` name and version, not a cache path. The view reads installed sources offline, supports project specs only, and cannot combine with the other explain views or `--target`.

### Explain a source file

List the instructions planned for one project file, with their source, output path, file or directory condition, and reason. Claude Code, Codex, and Cursor are supported.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Project file to inspect. It does not have to exist. Cannot combine with a spec or code argument. |
| `--target <name>` | Required with `--file`. A configured target: `cursor`, `claude`, or `codex`. |

The report reads planned sync output, not what the model loaded. It does not see user instructions, the session launch directory, import approval, or session settings that turn off project instructions.

<details>
<summary>Read instruction statuses, native discovery rules, and JSON</summary>

| Status | Meaning |
|--------|---------|
| `always` | No file condition: Cursor's `alwaysApply: true` or root `AGENTS.md`; Claude Code's project `CLAUDE.md` or rules without `paths`; Codex's root native instruction file. |
| `match` | A file pattern or a nested instruction directory covers the file. |
| `no-match` | A selector misses the file, or a planned instruction filename takes priority over this output. |
| `model-selected` | `alwaysApply: false` with a description and no globs. Cursor's agent decides. |
| `manual` | `alwaysApply: false` with neither. Loads only when `@`-mentioned. |
| `unknown` | The command cannot establish discovery or evaluate the selector. The reason names the missing information. |
| `excluded` | Target selection (`target`, `targets`, `target-exclude`) leaves the target out. |
| `not-emitted` | The rule selects the target but sync writes no instruction for it. |

| Tool | Discovery the report models |
|------|-----------------------------|
| Cursor | A root `AGENTS.md` written for another tool reaches Cursor too. Braces, character classes, negation, or unreadable frontmatter report `unknown`. |
| Codex | Root `AGENTS.md` and `AGENTS.override.md` have no file condition; in one directory the override wins. Nested instructions report `unknown` and say whether the file is under their directory, since [Codex discovers](https://learn.chatgpt.com/docs/agent-configuration/agents-md) from the root to the session launch directory. Custom filenames report `unknown` because they need a fallback filename or session setting. |
| Claude Code | Reads generated `paths`, including tool-specific overrides, and Markdown rules under `.claude/rules/`. A nested `CLAUDE.md` covers files beneath it; elsewhere it depends on the session start directory, so `unknown`. Imports and outputs outside native discovery that need session approval or settings, and patterns it cannot evaluate (braces, character classes), report `unknown` with the reason. See [Claude Code's instruction discovery](https://code.claude.com/docs/en/memory). |

```json
{"version": "1", "command": "explain", "file": "...", "target": "cursor", "note": "...",
 "instructions": [{"status": "match", "source": "...", "output": "...", "selector": "...", "reason": "..."}]}
```

</details>

### Rank context contributions

`--context --target <name>` ranks the sources that add the most text at session start, counting words like `lint` does, with bytes. It needs a configured target, takes no spec argument, and writes nothing.

```bash
agnostic-ai explain --context --target claude --json
agnostic-ai explain --context --target cursor --file src/main.go
```

Counts are estimates from planned output and spec text, not model tokens; runtime context, external imports, user-owned files, and host truncation are not counted. Inlined imports keep their source paths; unresolved ones are not read.

The report ends with opportunities: paragraphs of at least eight words written more than once in the specs that load at startup for that target, byte for byte after trimming. Each lists its copies, source files, size per copy, and the words and bytes one session start could drop at least. A spec that two generated files both carry counts once, because the fix is the tool setting that reads both files, not the spec. Similar wording is not reported, because only you can tell whether two instructions mean the same. Review the copies and edit the canonical specs, then run the command again to see the new counts. The command never edits a file.

<details>
<summary>Read context accounting groups and JSON</summary>

Entry-point layers, marked rule and review sections, inlined imports, always-on rules, each skill and agent description, and generated framing count separately, so the startup total matches lint. Sources sharing a document count once.

Skill and agent bodies and conditional rule bodies appear under `on-demand`, outside the startup total, with their own `on_demand` sum. With `--file` for Cursor or Claude Code, matching scoped rule bodies appear under `file-scope` with their own total; unmatched or uncertain ones stay on demand. Entries sort by group, then words and bytes (descending), then source path and category. Text and JSON use the same numbers.

```json
{"version": "1", "command": "explain", "target": "claude", "note": "...",
 "startup": {"words": 80, "bytes": 500}, "file_scope": {"words": 0, "bytes": 0},
 "on_demand": {"words": 300, "bytes": 2000},
 "contributions": [{"source": ".agnostic-ai/skills/review/SKILL.md",
 "category": "skill discovery", "load": "startup", "words": 40, "bytes": 220}],
 "opportunities": [{"kind": "duplicate-passage", "target": "claude", "load": "startup",
 "passage": "...", "copies": 2, "size": {"words": 14, "bytes": 74},
 "reduction": {"words": 14, "bytes": 74},
 "sources": [{"source": ".agnostic-ai/AGNOSTIC_AI.md", "category": "entry-point layer"}],
 "suggestion": "..."}]}
```

`file` is present when requested. `load` is `startup`, `file-scope`, or `on-demand`. `on_demand` and `opportunities` were added in version `1` without changing other fields; `opportunities` is an empty array when there are none, largest `reduction` first. `size` is one copy; `reduction` is the other copies.

</details>

### List generator inputs

```bash
agnostic-ai explain --inputs
```

List the files and directories that can affect generated output, one per line relative to the repository root (`--json` for an array). Takes no spec argument or `--file`. Used by [git hooks](@/docs/git-hooks.md#check-staged-files). It lists the config files, `.agnostic-ai/**`, source directories outside `.agnostic-ai/`, files reviews pull in with `@path` (and the entry point's, with `sync.resolve-imports: inline`), `agnostic.packs.lock`, `.gitignore`, and `builtin:<name>@<content-hash>` for each enabled built-in.

## compare

Compare how two built-in targets handle the project's agent and skill fields, rule activation, hooks, MCP connections, and portable permissions, before you switch or add a tool. The [compare targets](@/docs/compare.md) page shows sample paths for any pair.

```bash
agnostic-ai compare claude cursor
```

| Flag | Description |
|------|-------------|
| `--json` | Stable JSON for scripts. |

Each field gets one result per target, with the output paths or the reason, and a `next:` step when known, such as a required `outputs.zed.tasks-file`. `(differs)` marks a field whose results differ.

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. This describes the file, not that the tools behave the same, a hook ran, or an MCP server connects. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no place for the field or the kind. A field the tool ignores keeps that reason even when its native key is written. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or a scope the target cannot express. |
| `unknown` | The written files do not show either way. |

The command fails on unknown targets, the same target twice, invalid specs or config, and external adapters.

<details>
<summary>Read compared fields, permission mappings, and JSON</summary>

It covers agent and skill fields; rule `scope`, `paths`, `globs`, and `alwaysApply`; hook `on`, `match`, `event`, `matcher`, `command`, `args`, `timeout`, and `failClosed`, using current event mappings and written handlers; MCP transport (`type`), `command`, `args`, `url`, and each `env` and `headers` entry; and portable settings permissions one source rule at a time (allow, ask, deny, default-mode). Other settings, kinds, and hook fields are out of scope. Skill fields such as `argument-hint`, `effort`, and `disable-model-invocation` are judged from the files each adapter writes, including Codex's separate policy files.

MCP results never show command, argument, URL, environment, or header values, defaults, or reference text. A server whose URL or argument reference the tool cannot represent may be omitted whole; no explicit transport gives `unknown`.

Permission rows use the source index, such as `permissions.deny[0]`, name the rule, and show native override values and partial mappings. Recognized credentials in complete native permission records, such as Augment webhook policies, are hidden in text and JSON summaries but compared as written; filename patterns and ordinary permission actions stay visible. Codex shell permissions need `outputs.codex.exec-policies-from-permissions: true`. Default-mode applies only to global sync, so project comparison reports it unsupported. A difference in effective decision or partial support counts, even when both results are translated.

```json
{"version": "1", "command": "compare", "targets": ["claude", "cursor"], "coverage": "...", "caveat": "...",
 "specs": [{"kind": "agent", "name": "...", "path": "...", "fields": [{"field": "tools", "differs": true,
   "results": [{"target": "cursor", "status": "unsupported", "reason": "...", "next": "..."}]}]}],
 "fields": 6, "differences": 3}
```

</details>

## render

Print what each target writes for one spec, without writing files.

```bash
agnostic-ai render rules/no-console-log.md --target claude,codex
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Targets to render, repeated or comma-separated. Default: all in `agnostic-ai.yaml`. |

Each file prints as `# target: <name>: <output path>` and its body. A target that writes nothing for that kind prints a note. Known credential values are hidden; see [credential previews](@/docs/cli-reference/sync.md#credential-previews).

## graph

Show which targets and files each spec feeds (spec → target → file). Read-only. When a spec seems missing from a tool, run `agnostic-ai graph --spec <name>`: a `-` means that tool gets no file, so check the tool's supported kinds and the spec's target filters. Rows sort by spec name, then target.

```text
$ agnostic-ai graph
spec           | claude cursor codex
code-reviewer  | agent  agent  agent
no-console-log | rule   rule   rule
```

`mermaid` starts with `graph LR`, `dot` pipes to Graphviz (`agnostic-ai graph --format dot | dot -Tsvg > graph.svg`), and `json` gives one record per file:

```json
{"spec": "no-console-log", "kind": "rule", "target": "claude", "path": ".claude/rules/no-console-log.md"}
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default, aligned table), `mermaid`, `dot`, `json`. |
| `--target` | Restrict to one target. |
| `--spec` | Restrict to one spec name. |
| `--kind` | Restrict to one kind: agent, skill, rule, hook, mcp, command. |

## why

Name the source of a generated file, so you edit the source instead. Use a project-relative path, even for a file not synced yet; symlinks resolve to the real path. In VS Code, `agnostic-ai: Open canonical source` does the same for the current file.

```sh
$ agnostic-ai why .cursor/rules/no-console-log.mdc
.cursor/rules/no-console-log.mdc
  adapter: cursor
  output keys: (adapter defaults)
  last sync: 2026-05-15T10:23:00Z
  sources:
    [rule] no-console-log (.agnostic-ai/rules/no-console-log.md): full
```

Edit the path under `sources`, then run `agnostic-ai sync`.

`why` can recognize files from tools outside your `targets` list. If one of those tools cannot generate files with your configuration, it skips that tool. A failure from a tool in `targets` stops the command and names that tool once.

| Field | Meaning |
|-------|---------|
| `adapter` | Tool that wrote the file. For a shared file such as `AGENTS.md`, the first tool that reads it. For a path several tools share (`.agents/skills/`), a tool listed in `targets`. If only a tool you did not list writes the path, it reads `adapter: amp (not configured)`. |
| `output keys` | Every `outputs.<target>.*` key whose value appears in the path, or `(adapter defaults)`. |
| `last sync` | UTC time from `.agnostic-ai/.sync-state`, or `unknown` when that file is missing. |
| `configured` | JSON only. `true` when the tool is listed in `targets`. |
| `sources` | Every spec that feeds the file: `full` when the spec owns the file, `section` when it is one of several merged into a shared file. Root instruction files list `.agnostic-ai/AGNOSTIC_AI.md` first, then `.agnostic-ai/local/AGNOSTIC_AI.md` when present, then each [inlined rule](@/docs/target-behavior.md#inlined-rules) as a `section`. |

Without `.agnostic-ai/.sync-state`, run `sync` first. A spec path points you to `explain`, and `AGNOSTIC_AI.md` names the entry-point files it feeds; a path sync does not write reports "not synced or not tracked".

| Flag | Description |
|------|-------------|
| `--format` | `text` (default) or `json`. |
| `--json` | Alias for JSON output. |

## view

Open the bundled web interface in your browser; `ui` is an alias. From a project folder or any folder inside it, `view` reads that project. Outside a project, it reads the user configuration in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`). Keep the terminal running; Ctrl-C stops it. See the [browser walkthrough](@/docs/inspect-configuration.md).

```bash
agnostic-ai ui
agnostic-ai view --json
agnostic-ai view --global
```

| Flag | Description |
|------|-------------|
| `--json` | Print the overview as JSON (schema `overview/v1`) without starting a server or opening a browser. |
| `--dir` | Read this project directory instead of discovering one. |
| `--addr` | Explicit loopback IP and port. Default `127.0.0.1:4310`. When set, `view` stays on this port and fails if another configuration serves it. |
| `--global` | Read the user configuration instead of a project. Cannot combine with `--dir`. |

A second run for the same project and scope opens the running server and exits. When the default port serves another project or scope, an `--api-only` server, or another program, `view` takes the next free loopback port up to `127.0.0.1:4319` and prints it; later runs find that server. An invalid project configuration reports an error.

The overview lists context, authority, tools, lifecycle, and settings, with findings and generated-file comparisons for the selected targets. Specs scoped only to other targets are left out of lists and counts. Search within a section to find a source file or value, or press **Search** (`Cmd+K` or `Ctrl+K`) to search every section, findings included. Opening a result puts a link to that entry in the address bar; it survives a reload. The browser has light and dark themes and works on narrow screens. Other browser features are under [serve](#serve).

## serve

Serve the bundled web interface and a local JSON API. It works offline without a source checkout or frontend build, and reloads configuration on every request.

```bash
agnostic-ai serve --global
agnostic-ai serve --api-only
```

| Flag | Description |
|------|-------------|
| `--addr` | Explicit loopback IP and port. Default `127.0.0.1:4310`. |
| `--allow-origin` | Additional exact browser origin, such as `http://127.0.0.1:5173` for Vite. The server accepts loopback hosts and same-origin requests otherwise. |
| `--dir` | Project directory to read. Default: the current directory. |
| `--ui` | Serve a built client directory, such as `ui/dist`, instead of the bundled one. |
| `--api-only` | Serve only the JSON API. Cannot combine with `--ui` or `--open`. |
| `--open` | Open the served URL in a browser. |
| `--global` | Serve the user configuration. Its overview shows `lint --global` findings and drift against what `sync --global` would write; doctor checks are skipped. |
| `--no-installed` | Skip comparing generated files with disk. |

In the browser:

- **Authority** lists permission files and rules. Permission proposals have no editor; use the [API](#api).
- The header labels the page **Project** or **Global**. On a project page, **Open global settings** opens the user configuration in a new tab: it reuses a `serve --global` already running on `127.0.0.1:4310` to `4319` that started in this project, or starts one on the next free port. A global page that started in a project (from that button, or `ui --global` or `serve --global` run in a project folder) shows **Open project** with the project's name to go back. A server the button starts stops with the server that started it, and its way back returns to that server, also when it runs on another `--addr`. Each tab edits only the configuration it serves.
- **Open files in** in the header picks Cursor or VS Code for every edit link, and the browser remembers it. A finding on a file opens at its line when known. **Copy path** sits beside each link, because the browser cannot tell whether the editor opened.
- With `--global`, the `AGNOSTIC_AI.md` row in **Context** has **Copy refine prompt**: a prompt that asks a coding agent to review the file against current guidance for instruction files, propose a full rewrite with a change table, recommend which rules belong in an existing or new skill or agent, and leave the file unchanged until you approve.
- **Context size** in **Context** shows the [context report](#rank-context-contributions) for a chosen target: startup, file-scoped (for a file you enter), and on-demand sizes, ranked sources, and duplicated startup passages. Each duplicate has edit links to its sources and **Copy review prompt**, which asks a coding agent to compare the copies and propose a change with its tradeoffs before editing. **Refresh** reads the specs again. Sizes are words and bytes, not tokens, and smaller numbers do not guarantee lower cost or faster replies.
- Each finding has **Copy fix prompt**; **Details** shows the suggested fix, explanation, and full prompt. After you fix the source, **Recheck** runs the checks again with the same refresh and keeps your page, search, and unsaved input. It says whether that finding still appears, marks new findings from the same check on the same file, and treats a failed refresh or a lint run that stopped early as unknown, never as fixed. A doctor finding that disappears is shown as not confirmed, because these doctor checks skip files they cannot read. Snapshots cannot recheck. A findings summary links to the warnings below the controls; errors stay above them.
- Tool lists use logos, with the full name on hover. Source files appear in compact lists with paths and edit links. Every lifecycle stage shows when it runs, its hook count, and any disabled hooks. **Runs on** labels the tools that support the stage; each hook leads with its command or type. Native settings are grouped by tool, with the `x-<tool>` prefix shown once.
- **Settings** shows configured fields first and the rest under **Unconfigured fields**, including a default model or reasoning effort. Global scalar fields offer **Update** and **Clear**. Session retention is editable only with `--global`.
- Default model and reasoning effort have **What each tool gets**: the value sync writes for each configured tool, the file or `agnostic-ai.yaml` key it comes from (Claude Code `outputs.claude.settings.model`, Codex `outputs.codex.config`), and why each other value loses: a later spec, a personal override or clear, a spec scoped to other tools, or a per-tool map with no entry. It names the model tier and alias, says when a tool has no place for the value, and says when sync writes none, without guessing the tool's own default. It reports whether installed files match, not what a running tool uses. It points out an `x-<tool>.model` native key; other `x-<tool>` keys and imported native settings files are not covered. The JSON overview carries it as `resolved` on those fields.
- Under **What each tool gets**, each tool has its own edit control. It saves `model.<tool>` or `effort.<tool>` in the chosen file, which wins over the value for every tool. The origin line says where the value comes from, such as `personal · codex` or `shared · all tools`, and names the `agnostic-ai.yaml` key that replaces it when one does. **Use the value for every tool** removes that tool's entry. Effort has a control only for tools that take an effort setting. A spec whose `model` names a model tier stays read-only here, and so does one tool's effort when the tier gives the effort.
- In a project, each built-in switch previews the config edit, generated files, and findings for every target. **Apply change** writes it; **Cancel** discards the preview, so a reload does not bring it back. **Undo last change** restores the files and permissions from before the last apply. Undo survives a browser reload, expires when the server stops, and is replaced by the next apply. Turning on `memory` in a Git checkout also adds a `.git/info/exclude` line so Git ignores Claude Code's private `settings.local.json`.
- In a project, a default model or reasoning effort uses **Preview and apply**, **Preview removal**, and **Preview personal reset**. The preview names the settings file, the targets, and every source and generated file that changes, without writing any of them. **Apply change** writes them and checks the generated files; that does not mean a running tool has reloaded them. Built-ins and settings share one **Undo last change**, which names the change.
- Global built-ins and settings edits save source files without syncing. Run `agnostic-ai sync` (or `sync --global`) to apply them. If a boolean or built-in changed since the page loaded, the save stops and offers **Reload saved values**.

Apply needs no earlier sync: the preview lists the generated files, sync state, and lock `.gitignore` entries it creates. It includes pending source changes when files are missing or match sync's recorded checksums, and undo can restore them. These need the CLI: hand edits, files without ownership records, external adapters, paths outside the project, linked skill folders, and memory changes that need fact migration or external storage. A changed input or output blocks apply or undo until a fresh preview. A failed write restores the files the operation owns; a recovery error names any it kept and points to `agnostic-ai doctor`.

Edits keep unrelated text and private file permissions. A validation failure restores the edit, unless another process changed the file; that is reported as a conflict.

### Save shared or personal changes {#edit-scope}

The top of **Settings** picks where edits go, and the browser remembers it. **Shared (committed)**, the default, writes the shared files Git tracks. **Personal (not committed)** writes git-ignored [local overrides](@/docs/local-overrides.md): built-ins go to `agnostic-ai.local.yaml`, and a settings field to `.agnostic-ai/local/settings/<name>.yaml`, which overlays the shared spec of that name. In the global view they go to `local/agnostic-ai.yaml` and `local/settings/<name>.yaml`. Outputs a personal built-in adds go to `.git/info/exclude`, not the shared `.gitignore` block.

A value from a personal file shows a **personal** badge in both modes. **Reset to shared value**, or **Preview personal reset** for a project model or effort, removes your override and the personal file once it is empty. Shared mode asks you to reset a personal value before changing the shared value. Your first personal built-in change copies the shared list into `agnostic-ai.local.yaml`, since that list replaces the shared one whole; **Reset built-ins to the shared list** drops it. A personal list set through a YAML merge (`<<`) is refused; edit it by hand. A personal save is refused when its file resolves, through links, outside the local folder, into a shared config or source folder, or to a file Git tracks.

Before a personal save writes its file, the server checks that Git ignores it. When no rule does yet, as in a project that never ran `sync`, it adds the local folder (or `agnostic-ai.local.yaml`) to `.git/info/exclude`, which linked worktrees share. Your other lines in that file stay. If Git still would not ignore the file, the save is refused and the personal file is not written; add the path to `.gitignore` and save again. A new `.git/info/exclude` rule remains if it was added before a refusal or stale conflict, to protect personal files. Outside a Git repository there is nothing to ignore, so the save writes the file directly.

In a linked worktree, a built-in only you turn on needs `agnostic-ai sync` in the CLI. The first personal change keeps the `memory.personal` mode inherited from the main checkout, and a reset leaves `memory.personal: checkout` behind rather than switch to the main checkout's mode.

### API

Edit and impact requests take JSON. Every edit and preview names its `audience`: `shared` or `personal`. The older `scope` field (`team` or `local`) is accepted until the next minor release; a request that sends both with different meanings returns 400. A change plan carries both `audience` and `scope` until then. A boolean edit must include its `enabled` or `value` field, even when false. Invalid values and YAML that cannot be spliced safely are refused without changing the file. Previews hide recognized credentials using the [credential preview rules](@/docs/cli-reference/sync.md#credential-previews), and run no MCP servers, hooks, or external adapters.

| Endpoint | Body or query |
|----------|---------------|
| `GET /api/health` | None. Returns `ok`, absolute `root`, `global`, and `ui` (whether the web interface is served), without loading specs. A global server started in a project adds that project's absolute `projectRoot`. |
| `POST /api/switch` | `{}` with `Content-Type: application/json`. Finds or starts the server for the other configuration: the user configuration from a project, or `projectRoot` from a global server. Returns `url`, `root`, `global`, and `started`. Answers 404 on a global server with no `projectRoot`, and 503 with the command to run when no server could start. |
| `GET /api/views/overview` | None. |
| `GET /api/views/context` | `?target=<tool>`, a configured target; default the first one. `&file=<path>` adds scoped rules for that file, as `--file` does; `fileTargets` lists the targets that accept it. Returns `targets`, `target`, absolute `root`, and `report`, the `explain --context --json` output. With `--global`, `supported` is false and `reason` says why. |
| `GET /api/views/authority` | `?agentId=<id>&target=<tool>` for per-agent decisions. |
| `POST /api/views/authority/impact` | A permission proposal to preview. |
| `GET /api/views/authority/schema` | None. |
| `POST /api/edit/builtin` | `{audience, name, enabled, expected?}`, or `{audience: "personal", reset: true, expected?}` to drop the personal list. `expected` is the shown boolean, or the shown list for reset; a changed value returns 409 without saving the requested change. Built-ins apply to every tool, so a `target` other than `all` returns 400. |
| `POST /api/edit/value` | `{audience, path, key, value, expected?}`, where `value` is a boolean, or `null` with `audience: personal` to reset. `expected` is the shown boolean; a changed value returns 409 without saving the requested change. The key already names the tool (`x-claude.verbose`), so a `target` other than `all` returns 400. |
| `POST /api/edit/field` | `{audience, path?, key, target?, value, expected?}` for `model`, `effort`, and global `session-retention-days`. `target` is `all` (the default) or a configured tool; for one tool the edit writes `model.<tool>` or `effort.<tool>`, turning `model: x` into `{default: x, <tool>: value}`, and an effort level is checked against that tool only. A tool with no effort setting returns 400. When a model tier gives the spec its effort, one tool's effort returns 422, because a spec value turns the tier's effort off for every tool. Session retention takes only `all`. With `all`, a field that is already a per-tool map gets its `default` entry. `null` clears the value at that address, or resets your override with `audience: personal`; a map left with only `default` goes back to a single value, except in a personal file over a shared per-tool map, where a single value would replace the whole shared map, so it stays `{default: x}`. `expected` is the value the page showed at that address in the file being written, or `null` if that file did not set it. If the file now holds another value, or a personal file now sets that address for a shared save, the answer is 409 and nothing is written. A file that already holds the new value counts as saved. |
| `GET /api/changes/builtin` | None. Returns the session's change state. |
| `POST /api/changes/builtin/preview` | `{audience, name, enabled}` for any shipped built-in, `{audience: "personal", reset: true}`, or `{audience, setting: {path?, key, target?, value, expected?}}` for `model` or `effort`, with the fields `/api/edit/field` takes. A setting plan carries `key`, `target` for one tool, and a `title` such as `effort set to high` or `model for codex set to gpt-5.5`; undo state names it in `undoTitle`. |
| `POST /api/changes/builtin/apply`, `.../undo`, `.../cancel` | `{id}` from the change state. Cancel discards that preview without writing files and keeps the last undo; it answers 409 when that preview was already replaced or closed. |

Omit `path` only when at most one editable settings spec exists; with none, a save creates `settings/defaults.yaml` for the selected audience. The response names the edited file.

## mcp tools

Connect to each configured MCP server, list its tools, and cache the result so `view` and `serve` show them without connecting. It is the only command that connects to MCP servers.

```bash
agnostic-ai mcp tools
agnostic-ai mcp tools --global
```

| Flag | Description |
|------|-------------|
| `--dir` | Project directory to read. Default: the current directory. |
| `--global` | Read the user configuration in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`). |
| `--json` | Print the cached result as JSON. |
| `--timeout` | Maximum probe time per server, including its header helper. Default `20s`. Process cleanup is bounded separately. |

Stdio servers start with their `command`, `args`, and `env`, with relative or omitted working directories resolved against the configuration root; HTTP servers use their headers and optional `headersHelper`. `${NAME}` resolves from the environment and `${workspaceFolder}` is the root; an unset reference is reported without connecting. SSE, WebSocket, and disabled servers are not probed. Up to four servers run at once; cancellation stops owned processes. Errors and tool descriptions are redacted before display or caching.

The cache lives in the user cache directory, shared across directory aliases. Changed specs mark a listing stale; rerun `mcp tools` to show it in the overview.
