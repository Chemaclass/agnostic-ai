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
| `--layers` | Show which project source layers replace or extend a spec. Add `--json` for the same ordered sources. |
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

### Claude skill overrides

For a skill that sync writes for Claude Code, `explain` lists each Claude settings file whose `skillOverrides` names the skill. It checks the files [`doctor` reads](@/docs/cli-reference/check.md#claude-skill-overrides), highest precedence first; managed settings and `--settings` are not read. A line that does not take effect is marked, so the first unmarked line is the value Claude Code applies:

```text
claude overrides:
  .claude/settings.local.json: off (Claude Code refuses to run it; change it in /skills)
  ~/.claude/settings.json: name-only (not applied: a higher file sets it)
```

A value Claude Code rejects, such as `"Off"` or `false`, shows as rejected. Claude Code may then skip the whole file, so its other lines say so and overrule nothing. Two keys in one file that name the skill with different values are both marked as conflicting, since Claude Code does not say which applies. The file still overrules lower files. A file that is not strict JSON shows as skipped by Claude Code, even when no other file names the skill. The block is omitted when no file names the skill and every file reads. `--json` adds the same entries, or omits the key:

```json
"overrides": [{"target": "claude", "path": ".claude/settings.local.json", "scope": "local", "value": "off", "effective": true}]
```

A rejected value adds `"rejected": true`, a conflicting key `"conflict": true`, and another line in a file with a rejected value `"uncertain": true`. A skipped file has `problem` and no `value`.

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

Here the project replaces the pack rule, and the personal file extends the project rule. Edit either contributor to change the effective spec. You can also pass the replaced pack path to get the same report.

Sources follow sync's resolution order: enabled built-ins, installed packs, project specs, then project-local overrides. `base` starts a spec, `replace` discards the earlier chain, and `extend` merges with it. `contributor` means the source survives resolution, without attributing individual fields or body text. An extension can replace body text or metadata while still participating in the merge.

With `--json`, `spec` identifies the effective source and `sources` lists each candidate's `source`, `operation`, and final `role`. Built-ins use a symbolic `builtin` name and version instead of a cache path. The view reads installed sources offline. It supports project specs only and cannot be combined with the other explain views or `--target`.

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
