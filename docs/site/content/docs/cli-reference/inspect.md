+++
title = "Inspect and preview"
description = "See where a spec goes, compare tools, preview a spec, and trace a generated file."
weight = 40

[extra]
nav_title = "Inspect and preview"
group = "Reference"
+++

# Inspect and preview

Start with the item you know: `explain <spec>` finds a spec's output, `why <file>` finds a generated file's source, and `render <spec>` previews file contents. Use `compare` before changing tools and `graph` for an overview. These commands help a coding agent find the source to edit.

## explain

List every output file and section one spec contributes to. It works backward from the `<!-- source: ... -->` markers in merged documents. With `--file`, list the instructions that apply to one source file instead.

Given an [error code](@/docs/errors.md) such as `AAI-003` or a [lint code](@/docs/cli-reference/check.md#lint) such as `LINT011`, it prints the code's cause and fix. A lint code also shows its severity and any `lint` config key that tunes it. `--json` adds `severity` and `config` fields.

```bash
agnostic-ai explain rules/conventional-commits.md --json
agnostic-ai explain builtin:handoff --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable JSON for editor extensions and scripts. |
| `--global` | Explain a spec in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) or its `local/` layer. A relative path resolves against that root. |

<details>
<summary>Read contribution paths, model resolution, and explain JSON</summary>

Contributions are grouped by configured target. A "would emit if enabled" list covers tools that are not enabled. Each entry is tagged `(full file)` or `(section "<name>")`.

With `--global`, contributions read like this:

| Spec | Contribution |
|------|--------------|
| Agent or skill | A whole file. |
| Rule | A section of the instructions file. |
| Hook | Its event in the hooks file. |
| MCP server | Its key in the user MCP file. |
| Settings | Each key it sets, tagged `(key "<key>")`. |

A spec that sets `model` or `effort` also lists the [tier](@/docs/spec-format/agents.md#model-tiers) it names and the model and effort it resolves to on each configured target it reaches. `tool default` means the target writes no model. A target with no model or effort key for that kind, such as Codex skills, drops it and `sync` prints a coverage note.

```json
{"version": "1", "command": "explain", "spec": {"kind": "rule", "name": "...", "path": "..."},
 "contributions": [{"target": "...", "path": "...", "section": "...", "mode": "full|section|key"}],
 "would_emit_if_enabled": [],
 "model_tier": "strong", "models": [{"target": "...", "model": "...", "effort": "..."}]}
```

`model_tier` and `models` are omitted when the spec sets neither `model` nor `effort`.

</details>

For an agent's `can` or `tools`, and for settings `permissions`, the report lists each configured target's native names. Unsupported rules stay visible. A widening line names the extra access a target grants. Tool-specific overrides show their values and name the field that wins. For Codex, the report calculates extra access from the merged permissions and any explicit command policy source.

```bash
agnostic-ai explain agents/reviewer.md
agnostic-ai explain settings/permissions.yaml --json
```

`explain` still shows translations with `on-unsupported: error`. `sync` fails on widening.

<details>
<summary>Read capability JSON fields</summary>

The JSON `capabilities` array holds `target`, `field`, `capability`, `native`, and `supported`. `widening` lists extra access when present. `override` names a native field or policy source that replaces the portable rule. The array is omitted for specs without these fields.

</details>

### Explain a source file

Start from a project file instead of a spec. The report lists configured instructions from the planned sync output, with their source, output path, file or directory condition, and reason. Claude Code, Codex, and Cursor are supported.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
agnostic-ai explain --file services/payments/handler.go --target claude
agnostic-ai explain --file services/payments/handler.go --target codex
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Project file to inspect. The file does not have to exist. Cannot combine with a spec or error code argument. |
| `--target <name>` | Required with `--file`. Must be a configured target. Supported: `cursor`, `claude`, `codex`. |

The report describes planned project configuration, not the instructions the model has actually loaded. It does not inspect user instructions, the session launch directory, import approval, or session settings that disable project instructions.

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

A root `AGENTS.md` written for another tool such as Codex reaches Cursor too. Cursor reports `unknown` for braces, character classes, negation, or unreadable frontmatter.

For Codex, root `AGENTS.md` and `AGENTS.override.md` instructions have no file condition. When both are planned in one directory, the override takes priority. Nested instructions report `unknown` and state whether the file is under their directory. Codex discovers instructions from the project root to its session launch directory, which this command cannot establish. Custom filenames also report `unknown` because their discovery needs a fallback filename or session setting. See [Codex's instruction discovery](https://learn.chatgpt.com/docs/agent-configuration/agents-md).

For Claude Code, the report reads the generated `paths` values, including tool-specific overrides, and discovers Markdown rules recursively under `.claude/rules/`. Nested `CLAUDE.md` files cover files beneath their directory. For other files, loading depends on the directory where the session starts, so the result is `unknown`. Imported instructions and outputs outside native discovery also report `unknown` when session approval or settings are needed. Complex path patterns the command cannot evaluate, including brace expansion and character classes, name that limit in their reason. See [Claude Code's instruction discovery](https://code.claude.com/docs/en/memory).

```json
{"version": "1", "command": "explain", "file": "...", "target": "cursor", "note": "...",
 "instructions": [{"status": "match", "source": "...", "output": "...", "selector": "...", "reason": "..."}]}
```

</details>

### Rank context contributions

Use `--context --target <name>` to see which sources add the most text when a session starts, even when no lint warning appears. The report uses the same whitespace-separated word accounting as `lint`, with bytes beside each count. It needs a configured project target, takes no spec argument, and writes nothing.

```bash
agnostic-ai explain --context --target codex
agnostic-ai explain --context --target claude --json
agnostic-ai explain --context --target cursor --file src/main.go
```

These are estimates from planned output and spec text, not model tokens or observed context. Runtime context, external imports, user-owned files, and host truncation are unknown. Inlined imports retain their source paths; unresolved imports are not expanded or read by this report.

<details>
<summary>Read context accounting groups and JSON</summary>

Entry-point layers, marked rule and review sections, inlined imports, always-on rules, and individual skill and agent descriptions used to find skills and agents are counted separately. Generated framing has its own contribution so the startup word total stays consistent with lint. Sources sharing a generated document are counted once for the selected target.

Skill and agent bodies and conditional rule bodies appear under `on-demand`, outside the startup totals. With `--file` for Cursor or Claude Code, matching scoped rule bodies appear under `file-scope`, with a separate additional total. Unmatched or uncertain rule bodies remain on demand. Entries sort by loading group, descending words and bytes, then source path and category.

```json
{"version": "1", "command": "explain", "target": "claude", "note": "...",
 "startup": {"words": 80, "bytes": 500}, "file_scope": {"words": 0, "bytes": 0},
 "contributions": [{"source": ".agnostic-ai/skills/review/SKILL.md",
 "category": "skill discovery", "load": "startup", "words": 40, "bytes": 220}]}
```

`file` is present when requested. `load` is `startup`, `file-scope`, or `on-demand`. The text and JSON reports use the same contributions and totals.

</details>

### List generator inputs

```bash
agnostic-ai explain --inputs
```

`--inputs` lists the files and directories that can affect generated output, one per line. Use `--json` for an array. Paths are relative to the repository root. Run it without a spec argument or `--file`. See [git hooks](@/docs/git-hooks.md#check-staged-files). It lists:

- the config files
- `.agnostic-ai/**`
- source directories outside `.agnostic-ai/`
- files that reviews pull in with `@path` (and the entry point's, with `sync.resolve-imports: inline`)
- `agnostic.packs.lock`
- `.gitignore`
- `builtin:<name>@<content-hash>` for each enabled built-in

## compare

Compare how two built-in targets handle the project's agent and skill fields, rule activation, hook configuration, MCP connections, and portable permissions, before you switch or add a tool.

```bash
agnostic-ai compare claude cursor
```

| Flag | Description |
|------|-------------|
| `--json` | Stable JSON for scripts. |

`preserved` describes the written file. It does not prove the tools behave the same, that a hook ran, or that an MCP server connects. Hook results use current event mappings and written handlers. A field the tool ignores keeps that reason even when its native key remains in the output. Required output options, such as `outputs.zed.tasks-file`, appear in the next step. `(differs)` marks a field with a different result per target. Each result names the output paths or the reason, plus a `next:` step when known.

The command fails on unknown targets, the same target twice, invalid specs or config, and external adapters.

<details>
<summary>Read compared fields, result labels, permission mappings, and JSON</summary>

It covers agent and skill fields, rule `scope`, `paths`, `globs`, and `alwaysApply`, and hook `on`, `match`, `event`, `matcher`, `command`, `args`, `timeout`, and `failClosed`. MCP coverage includes transport (`type`), `command`, `args`, `url`, and each `env` and `headers` entry. Portable settings permissions are compared one source rule at a time, including allow, ask, deny, and default-mode. Other settings, spec kinds, and hook fields are outside this comparison. Skill fields such as `argument-hint`, `effort`, and `disable-model-invocation` are judged from the files each adapter writes, including Codex's separate policy files. Each field gets one result per target:

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no place for the field or the kind. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or a scope the target cannot express. |
| `unknown` | The written files do not show either way. |

MCP comparison names fields and output paths without showing command, argument, URL, environment or header values, including defaults and unknown reference text. If the tool's configuration cannot represent a reference in the server URL or arguments, the whole server can be omitted. If the native file has no explicit transport, the result is `unknown`.

Permission rows use the source list index, such as `permissions.deny[0]`, name the rule, and show native override values and partial mappings. Recognized credentials in complete native permission records, such as Augment webhook policies, are hidden in text and JSON summaries. Filename patterns and ordinary permission actions remain visible. Matching and comparison still use the original values. Codex shell permissions require `outputs.codex.exec-policies-from-permissions: true`. Portable default-mode applies to global sync, so project comparison reports it as unsupported. Differences include effective decisions and partial support, even when both results are translated.

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

Each file prints as `# target: <name>: <output path>` and its body. A target that writes nothing for that kind prints a note.

Known credential values are hidden in the displayed body. See [credential previews](@/docs/cli-reference/sync.md#credential-previews).

## graph

Show which targets and files each spec feeds (spec → target → file). It is read-only. See the [graph](@/docs/graph.md) guide.

```bash
agnostic-ai graph --format mermaid --target claude
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default, aligned table), `mermaid`, `dot`, `json`. |
| `--target` | Restrict to one target. |
| `--spec` | Restrict to one spec name. |
| `--kind` | Restrict to one kind: agent, skill, rule, hook, mcp, command. |

## why

Show a generated file's adapter, source specs, `outputs.<target>.*` keys, and last sync time. See the [why](@/docs/trace.md) guide.

```bash
agnostic-ai why .claude/rules/no-console-log.md --format json
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default) or `json`. |
| `--json` | Alias for JSON output. |

## view

Show a read-only view of the configuration as versioned JSON or readable text. Nothing is written. See the [Authority view](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/authority-view.md) notes for the contract.

`view authority` answers, for one subject on one coding tool, which exact action on which exact target is permitted, with the rule and file behind each decision. Four states stay separate: what the specs declare, what the selected tool does with it, whether the generated file is installed, and runtime enforcement, which is never observed.

```bash
agnostic-ai view authority --agent reviewer --target claude
agnostic-ai view authority --json
agnostic-ai view overview --json
```

| Flag | Description |
|------|-------------|
| `--agent` | The subject: an agent name, or `session` (default) for the tool itself before any agent narrows it. |
| `--target` | The coding tool to resolve effects for. Default: the first in `agnostic-ai.yaml`. |
| `--json` | The full view as JSON, schema `authority/v1` or `overview/v1`. |
| `--dir` | Project directory to read. Default: the current directory. |
| `--global` | Read the user-level configuration in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) instead of a project. |
| `--no-installed` | Skip comparing the generated files with disk. |

`view overview` prints the six harness sections with their configured items and per-tool support, and a feed of findings from `lint`, `doctor`, and drift, each with its evidence.

## serve

Host the views as a local JSON API, and the built web client when `--ui` names its directory. The project loads on every request, so an edited spec shows on the next refresh. The API proposes changes and reports their impact, but never writes.

```bash
agnostic-ai serve --ui ui/dist --open
agnostic-ai serve --global
```

| Flag | Description |
|------|-------------|
| `--addr` | Loopback address to listen on. Default `127.0.0.1:4310`. Any other address is refused. |
| `--dir` | Project directory to read. Default: the current directory. |
| `--ui` | Directory of the built client to serve, such as `ui/dist`. |
| `--open` | Open the served URL in a browser. |
| `--global` | Serve the user-level configuration; the overview and feed are not available there. |
| `--no-installed` | Skip comparing the generated files with disk. |
| `--allow-origin` | An origin, such as `http://localhost:5173`, whose pages may call the API from another port. Repeat for more. The Vite dev server needs none, since it forwards `/api`. |

The API is for this machine only and has no login, so `serve` guards it:

- It answers only requests addressed to `localhost`, `127.0.0.1`, or `::1`, and answers 421 to any other `Host`. This stops a page that points its own name at your machine.
- It refuses browser requests from any other origin with 403, unless `--allow-origin` names that origin. A request without an `Origin` header passes, as from `curl`, unless the browser marks an `/api` request as coming from another site (`Sec-Fetch-Site`), as an image or script tag does.
- `POST /api/views/authority/impact` needs `Content-Type: application/json`; any other body type gets 415.
- Credentials written in permission rules, URLs, and settings values show as `<redacted>`. A rule shown redacted cannot be named in a proposal; edit the settings file.
- The installed check uses built-in adapters only. An external `agnostic-ai-<name>` adapter is never run.

Endpoints: `GET /api/views/authority?agentId=<id>&target=<tool>`, `POST /api/views/authority/impact`, `GET /api/views/authority/schema`, `GET /api/views/overview`, `GET /api/views/feed`, and `GET /api/health`.
