+++
title = "Inspect and preview"
description = "See where a spec goes, compare tools, preview a spec, and trace a generated file."
weight = 40
aliases = ["/docs/trace/", "/docs/why/", "/docs/graph/"]

[extra]
nav_title = "Inspect and preview"
group = "Reference"

[extra.moved]
view = "@/docs/cli-reference/serve.md#view"
serve = "@/docs/cli-reference/serve.md#serve"
edit-scope = "@/docs/cli-reference/serve.md#edit-scope"
permission-editing = "@/docs/cli-reference/serve.md#permission-editing"
api = "@/docs/cli-reference/serve-api.md"
claude-code-cloud-sessions = "@/docs/cli-reference/companion.md#claude-code-cloud-sessions"
companion = "@/docs/cli-reference/companion.md#companion"
+++

# Inspect and preview

## View {#view .legacy-anchor}

## Serve {#serve .legacy-anchor}

## Save project or local changes {#edit-scope .legacy-anchor}

## Edit permission rules {#permission-editing .legacy-anchor}

## API {#api .legacy-anchor}

## Claude Code cloud sessions {#claude-code-cloud-sessions .legacy-anchor}

## Companion {#companion .legacy-anchor}

For the browser and companion, see [`view` and `serve`](@/docs/cli-reference/serve.md), the [local API](@/docs/cli-reference/serve-api.md), and [`companion`](@/docs/cli-reference/companion.md).

`explain <spec>` finds a spec's output, `why <file>` finds a generated file's source, and `render <spec>` previews contents. Use `compare` before changing tools and `graph` for an overview.

## explain

List the output files and sections a spec feeds, using the `<!-- source: ... -->` markers in merged documents. With `--file`, list instructions that apply to a source file.

Name a spec by its path, or by its path under `.agnostic-ai/` such as `rules/conventional-commits.md`.

Pass an [error code](@/docs/errors.md) such as `AAI-003` or a [lint code](@/docs/cli-reference/check.md#lint) such as `LINT011` to see its cause and fix. Lint codes also show severity and any related `lint` config key (`severity` and `config` in JSON).

```bash
agnostic-ai explain rules/conventional-commits.md --json
agnostic-ai explain builtin:handoff --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable JSON for editor extensions and scripts. |
| `--layers` | Show which project source layers replace or extend a spec. See [below](#debug-a-replaced-spec). |
| `--scope user` | Explain a spec in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) or its `local/` layer. A relative path resolves against that root. |
| `--file <path>` | Explain a source file. See [below](#explain-a-source-file). |
| `--context` | Rank startup context. See [below](#rank-context-contributions). |
| `--inputs` | List generator inputs. See [below](#list-generator-inputs). |

<details>
<summary>How explain groups contributions and shows models</summary>

- Contributions are grouped by configured target, tagged `(full file)` or `(section "<name>")`, plus a "would emit if enabled" list for other tools.
- A tool outside `targets` that cannot render your setup is left out. A failure from a tool in `targets` stops the command and names it once.
- With `--scope user`: an agent or skill is a whole file, a rule a section of the instructions file, a hook its event in a shared hooks file or its standalone hook files, an MCP server its key in the user MCP file, and a settings spec each key it sets, tagged `(key "<key>")`.
- Directory-based user-level hooks (Cline, Grok, Kiro) also list copied command scripts, with the same paths and target root settings as `sync --scope user`.
- A spec with `model` or `effort` lists its [tier](@/docs/spec-format/agents.md#model-tiers) and the model and effort it resolves to per target. `tool default` means the target writes no model. A target with no key for it, such as Codex skills, drops it, and `sync` prints a coverage note.
- For an agent's `can` or `tools` and settings `permissions`, it lists each target's native names, unsupported rules included.
- A widening line names extra access a target grants (for Codex, from the merged permissions and any explicit command policy source). `explain` shows it even with `on-unsupported: error`, where `sync` fails.
- Tool-specific overrides show their values and the field that wins.

</details>

<details>
<summary>Read explain JSON</summary>

```json
{"version": "1", "command": "explain", "spec": {"kind": "rule", "name": "...", "path": "..."},
 "contributions": [{"target": "...", "path": "...", "section": "...", "mode": "full|section|key"}],
 "would_emit_if_enabled": [],
 "model_tier": "strong", "models": [{"target": "...", "model": "...", "effort": "..."}]}
```

`model_tier` and `models` are omitted without `model` or `effort`. `capabilities` (omitted without capability fields) holds `target`, `field`, `capability`, `native`, and `supported`. `widening` lists extra access when present, and `override` names a native field or policy source that replaces the portable rule.

</details>

### Claude skill overrides

<details>
<summary>Claude skill override output</summary>

For a skill written for Claude Code, `explain` lists each Claude settings file whose `skillOverrides` names it, highest precedence first. It reads the same files and applies the same rules as [`doctor`](@/docs/cli-reference/doctor.md#claude-skill-overrides), including how rejected values, duplicate keys, and files that are not strict JSON show. A line that does not apply is marked, so the first unmarked line is the value Claude Code uses:

```text
claude overrides:
  .claude/settings.local.json: off (Claude Code refuses to run it; change it in /skills)
  ~/.claude/settings.json: name-only (not applied: a higher file sets it)
```

The block is omitted when no file names the skill and every file reads. `--json` adds the entries under `overrides`, with `rejected`, `conflict`, `uncertain`, or `problem` as in doctor:

```json
"overrides": [{"target": "claude", "path": ".claude/settings.local.json", "scope": "local", "value": "off", "effective": true}]
```

</details>

### Debug a replaced spec

When editing a pack rule has no effect, inspect its source layers:

```bash
agnostic-ai explain rules/conventional-commits.md --layers
```

```text
rule conventional-commits
  .agnostic-ai/packs/team/rules/conventional-commits.md [pack:team] base (replaced)
  .agnostic-ai/rules/conventional-commits.md [project] replace (contributor)
  .agnostic-ai/local/rules/conventional-commits.md [local] extend (contributor)
effective source: .agnostic-ai/local/rules/conventional-commits.md
```

<details>
<summary>How source layers are read</summary>

- Edit either contributor to change the effective spec. The replaced pack path gives the same report.
- Sources follow sync's order: enabled built-ins, installed packs, project specs, then project-local overrides.
- `base` starts a spec, `replace` discards the earlier chain, and `extend` merges with it.
- `contributor` means the source survives resolution. It does not attribute fields or body text, which an extension can replace.
- With `--json`, `spec` is the effective source and `sources` lists each candidate's `source`, `operation`, and final `role`. Built-ins show a symbolic `builtin` name and version, not a cache path.
- The view reads installed sources offline, supports project specs only, and cannot combine with other explain views or `--target`.

</details>

### Explain a source file

List the instructions planned for one project file, with their source, output path, file or directory condition, and reason. Claude Code, Codex, and Cursor are supported.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

`--file <path>` is the project file (it does not have to exist). `--target <name>` is required: `cursor`, `claude`, or `codex`.

The report describes planned sync output, not what the model loaded.

{% <details summary="Limits, flags, instruction statuses, discovery rules, and JSON"> %}

- `--file` cannot combine with a spec or code argument.
- The report cannot see user instructions, the session launch directory, import approval, or session settings that turn off project instructions.

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

{% </details> %}

### Rank context contributions

`--context --target <name>` ranks the sources that add the most text at session start, in words (as `lint` counts them) and bytes. It needs a configured target and takes no spec argument.

```bash
agnostic-ai explain --context --target claude --json
agnostic-ai explain --context --target cursor --file src/main.go
```

<details>
<summary>Context estimates and duplicate-passage opportunities</summary>

- Counts estimate planned output and spec text, not model tokens. Runtime context, external imports, user-owned files, and host truncation are not counted.
- Inlined imports keep their source paths. Unresolved ones are not read.
- The report ends with opportunities: paragraphs of at least eight words that appear more than once, byte for byte after trimming, in the specs a target loads at startup.
- Each lists its copies, source files, size per copy, and the minimum words and bytes one session start would drop.
- A spec that two generated files both carry counts once, since the fix is the tool setting that reads both files.
- Similar wording is not reported, because only you can tell if two instructions mean the same.
- Edit the canonical specs, then rerun to see the new counts.

</details>

<details>
<summary>Read context accounting groups and JSON</summary>

Entry-point layers, marked rule and review sections, inlined imports, always-on rules, each skill and agent description, and generated framing count separately, so the startup total matches lint. Sources sharing a document count once. A root `AGENTS.md` another tool writes counts for Cursor, and for Copilot when VS Code reads it.

Skill and agent bodies and conditional rule bodies appear under `on-demand`, outside the startup total, with their own `on_demand` sum. With `--file` for Cursor or Claude Code, matching scoped rule bodies appear under `file-scope` with their own total; unmatched or uncertain ones stay on demand. Entries sort by group, then words and bytes (descending), then source path and category. Text and JSON agree.

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

`file` is present when requested. `load` is `startup`, `file-scope`, or `on-demand`. `opportunities` is an empty array when there are none, largest `reduction` first. `size` is one copy; `reduction` is the other copies. `on_demand` and `opportunities` were added in version `1` without changing other fields.

</details>

### List generator inputs

```bash
agnostic-ai explain --inputs
```

List the files and directories that can affect generated output, one per line relative to the repository root (`--json` for an array). [Git hooks](@/docs/git-hooks.md#check-staged-files) use it.

{% <details summary="What the inputs list holds"> %}

- It takes no spec argument or `--file`.
- The config files and `.agnostic-ai/**`.
- Source directories outside `.agnostic-ai/`.
- Files reviews pull in with `@path` (and the entry point's, with `sync.resolve-imports: inline`).
- `agnostic.packs.lock` and `.gitignore`.
- `builtin:<name>@<content-hash>` for each enabled built-in.

{% </details> %}

## compare

Compare how two built-in targets handle your agent and skill fields, rule activation, hooks, MCP connections, and portable permissions, before you switch or add a tool. The [compare targets](@/docs/compare.md) page shows sample paths for any pair.

```bash
agnostic-ai compare claude cursor
```

Each field gets one result per target, with the output paths or the reason, and a `next:` step when known. `(differs)` marks differing results. `--json` prints stable JSON for scripts.

Each result is `preserved`, `translated`, `unsupported`, `excluded`, or `unknown`.

{% <details summary="What each result means, scope, and failures"> %}

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. This describes the file, not that the tools behave the same, a hook ran, or an MCP server connects. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no place for the field or the kind. A field the tool ignores keeps that reason even when its native key is written. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or a scope the target cannot express. |
| `unknown` | The written files do not show either way. |

- A `next:` step can be a required `outputs.zed.tasks-file`.
- A spec whose target filter skips both tools is left out, except settings.
- A built-in shows as `builtin:<name>`; its next step is the `builtins` setting or a project spec of the same name.
- It fails on unknown targets, the same target twice, invalid specs or config, and external adapters.

{% </details> %}

<details>
<summary>Read compared fields, permission mappings, and JSON</summary>

Compared fields:

- agent and skill fields
- rule `scope`, `paths`, `globs`, and `alwaysApply`
- hook `on`, `match`, `event`, `matcher`, `command`, `args`, `timeout`, and `failClosed`, from current event mappings and written handlers
- MCP `type`, `command`, `args`, `url`, and each `env` and `headers` entry
- portable settings permissions, one source rule at a time (allow, ask, deny, default-mode)

Other settings, kinds, and hook fields are out of scope. Skill fields such as `argument-hint`, `effort`, and `disable-model-invocation` are judged from the files each adapter writes, including Codex's separate policy files.

MCP rules:

- Results never show command, argument, URL, environment, or header values, defaults, or reference text.
- A server whose URL or argument reference the tool cannot represent may be omitted whole.
- No explicit transport gives `unknown`.

Permission rules:

- Rows use the source index, such as `permissions.deny[0]`, name the rule, and show native override values and partial mappings.
- A source or native rule holding a credential shows its tool name around `<redacted>`, as `sync --scope user` prints, such as `shell(<redacted>)` or `prefix_rule(<redacted>)`. In an `x-<target>` override, only the command pattern or string that holds it is replaced.
- Credentials in complete native permission records, such as Augment webhook policies, are hidden in text and JSON summaries but compared as written. Filename patterns and ordinary permission actions stay visible.
- Codex shell permissions need `outputs.codex.exec-policies-from-permissions: true`.
- Default-mode applies only to user-level sync, so project comparison reports it unsupported.
- A difference in effective decision or partial support counts, even when both results are translated.

```json
{"version": "1", "command": "compare", "targets": ["claude", "cursor"], "coverage": "...", "caveat": "...",
 "specs": [{"kind": "agent", "name": "...", "path": "...", "fields": [{"field": "tools", "differs": true,
   "results": [{"target": "cursor", "status": "unsupported", "reason": "...", "next": "..."}]}]}],
 "fields": 6, "differences": 3}
```

A built-in has an empty `path` and a `"builtin": {"name", "version"}` object.

</details>

## render

Print what each target would write for one spec.

```bash
agnostic-ai render rules/no-console-log.md --target claude,codex
```

`-t, --target <list>` picks the targets, repeated or comma-separated. Default: all in `agnostic-ai.yaml`.

Each file prints as `# target: <name>: <output path>` and its body. A target that writes nothing for that kind prints a note. [Credential values](@/docs/cli-reference/sync.md#credential-previews) are hidden.

## graph

Show which targets and files each spec feeds (spec → target → file). When a spec seems missing from a tool, run `agnostic-ai graph --spec <name>`: a `-` means that tool gets no file.

```text
$ agnostic-ai graph
spec           | claude cursor codex
code-reviewer  | agent  agent  agent
no-console-log | rule   rule   rule
```

Formats: `--format text|mermaid|dot|json`. Narrow with `--target`, `--spec`, or `--kind`.

{% <details summary="Graph flags, formats, and ordering"> %}

- Rows sort by spec name, then target. Without `targets`, graph covers every tool that can render your setup.
- `mermaid` starts with `graph LR`, `dot` pipes to Graphviz (`agnostic-ai graph --format dot | dot -Tsvg > graph.svg`), and `json` gives one record per file:

```json
{"spec": "no-console-log", "kind": "rule", "target": "claude", "path": ".claude/rules/no-console-log.md"}
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default, aligned table), `mermaid`, `dot`, `json`. |
| `--json` | Same as `--format json`. Fails with another `--format`. |
| `--target` | Restrict to one configured target. Any other name fails. |
| `--spec` | Restrict to one spec name. |
| `--kind` | Restrict to one kind: agent, skill, rule, hook, mcp, command, settings, review, environment, ignore. Any other kind fails. |

{% </details> %}

## why

Find the source to edit for a generated file. Use a project-relative path, even for a file not synced yet. In VS Code, `agnostic-ai: Open canonical source` does the same for the current file.

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

<details>
<summary>Fields in the why report</summary>

| Field | Meaning |
|-------|---------|
| `adapter` | Tool that wrote the file. For a shared file such as `AGENTS.md`, the first tool that reads it. For a path several tools share (`.agents/skills/`), a tool listed in `targets`. If only a tool you did not list writes the path, it reads `adapter: amp (not configured)`. |
| `output keys` | Every `outputs.<target>.*` key whose value appears in the path, or `(adapter defaults)`. |
| `last sync` | UTC time from `.agnostic-ai/.sync-state`, or `unknown` when that file is missing. |
| `configured` | JSON only. `true` when the tool is listed in `targets`. |
| `sources` | Every spec that feeds the file: `full` when the spec owns the file, `section` when it is one of several merged into a shared file. Root instruction files list `.agnostic-ai/AGNOSTIC_AI.md` first, then `.agnostic-ai/local/AGNOSTIC_AI.md` when present, then each [inlined rule](@/docs/target-behavior.md#inlined-rules) as a `section`. |

</details>

Without `.agnostic-ai/.sync-state`, run `sync` first. Use `--json` (or `--format json`) for JSON.

{% <details summary="Other paths, tools outside targets, and flags"> %}

- `why` also recognizes files from tools outside `targets`, skipping any that cannot generate files with your configuration. A failure from a tool in `targets` stops the command and names it once.
- A spec path points you to `explain`, and `AGNOSTIC_AI.md` names the entry-point files it feeds. A path sync does not write reports "not synced or not tracked".

| Flag | Description |
|------|-------------|
| `--format` | `text` (default) or `json`. |
| `--json` | Same as `--format json`. Fails with another `--format`. |

{% </details> %}

## mcp tools

Connect to each configured MCP server, list its tools, and cache the result so `view` and `serve` show them without connecting. No other command connects to MCP servers.

```bash
agnostic-ai mcp tools
agnostic-ai mcp tools --scope user
```

Flags: `--dir` (project to read), `--scope user` (user configuration in `$AGNOSTIC_AI_HOME`), `--json` (print the cached result), `--timeout` (per server, default `20s`).

{% <details summary="Flag details and how servers are probed"> %}

- `--dir` defaults to the current directory. `--scope user` reads the user configuration in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`).
- `--timeout` is the maximum probe time per server, including its header helper. Process cleanup is bounded separately.

- Stdio servers start with their `command`, `args`, and `env`. A relative or omitted working directory resolves against the configuration root.
- HTTP servers use their headers and optional `headersHelper`.
- `${NAME}` resolves from the environment and `${workspaceFolder}` is the root. An unset reference is reported without connecting.
- SSE, WebSocket, and disabled servers are not probed.
- Up to four servers run at once. Cancellation stops the processes it owns.
- Errors and tool descriptions are redacted before display or caching.
- The cache lives in the user cache directory, shared across directory aliases. Changed specs mark a listing stale: rerun `mcp tools` to refresh the overview.

{% </details> %}
