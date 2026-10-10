+++
title = "Inspect and preview"
description = "See where a spec goes, compare tools, preview a spec, and trace a generated file."
weight = 40

[extra]
group = "Reference"
+++

# Inspect and preview

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

For an agent's `can` or `tools`, and for settings `permissions`, the report lists each configured target's native names. Unsupported rules stay visible. A widening line names the extra access a target grants. Native overrides show their values and name the field that wins. For Codex, widening follows the merged permission policy and any explicit exec policy source.

```bash
agnostic-ai explain agents/reviewer.md
agnostic-ai explain settings/permissions.yaml --json
```

The JSON `capabilities` array holds `target`, `field`, `capability`, `native`, and `supported`. `widening` lists extra access when present. `override` names a native field or policy source that replaces the portable rule. The array is omitted for specs without these fields. `explain` still shows translations with `on-unsupported: error`. `sync` fails on widening.

### Explain a source file

Start from a project file instead of a spec. The report lists configured instructions from the planned sync output, with their source, output path, selector, and reason. Claude Code, Codex, and Cursor are supported.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
agnostic-ai explain --file services/payments/handler.go --target claude
agnostic-ai explain --file services/payments/handler.go --target codex
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Project file to inspect. The file does not have to exist. Cannot combine with a spec or error code argument. |
| `--target <name>` | Required with `--file`. Must be a configured target. Supported: `cursor`, `claude`, `codex`. |

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

For Claude Code, the report reads the emitted `paths` values, including native overrides, and discovers Markdown rules recursively under `.claude/rules/`. Nested `CLAUDE.md` files cover files beneath their directory. For other files, their loading depends on the session launch directory, so the result is `unknown`. Imported instructions and outputs outside native discovery also report `unknown` when session approval or settings are needed. Complex path patterns the command cannot evaluate, including brace expansion and character classes, name that limit in their reason. See [Claude Code's instruction discovery](https://code.claude.com/docs/en/memory).

The report describes planned project configuration, not the model's active context. It does not inspect user instructions, the session launch directory, import approval, or session settings that disable project instructions.

```json
{"version": "1", "command": "explain", "file": "...", "target": "cursor", "note": "...",
 "instructions": [{"status": "match", "source": "...", "output": "...", "selector": "...", "reason": "..."}]}
```

### List generator inputs

```bash
agnostic-ai explain --inputs
```

`--inputs` is its own mode. It lists every file and directory whose change can change a generated file, one per line (`--json` for an array). Paths are relative to the repository root, like a hook manager's glob. It takes no spec and no `--file`. See [git hooks](@/docs/git-hooks.md#check-staged-files). It lists:

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

It covers agent and skill fields, rule `scope`, `paths`, `globs`, and `alwaysApply`, and hook `on`, `match`, `event`, `matcher`, `command`, `args`, `timeout`, and `failClosed`. MCP coverage includes transport (`type`), `command`, `args`, `url`, and each `env` and `headers` entry. Portable settings permissions are compared one source rule at a time, including allow, ask, deny, and default-mode. Other settings, spec kinds, and hook fields are outside this comparison. Skill fields such as `argument-hint`, `effort`, and `disable-model-invocation` are judged from the files each adapter writes, including Codex policy sidecars. Each field gets one result per target:

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no place for the field or the kind. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or a scope the target cannot express. |
| `unknown` | The written files do not show either way. |

`preserved` describes the written file. It does not prove the tools behave the same, that a hook ran, or that an MCP server connects. Hook results use current event mappings and written handlers. A field the tool ignores keeps that reason even when its native key remains in the output. Required output options, such as `outputs.zed.tasks-file`, appear in the next step. `(differs)` marks a field with a different result per target. Each result names the output paths or the reason, plus a `next:` step when known.

MCP entries name affected fields and output paths without printing command, argument, URL, environment, or header values. Default values and unrecognized reference text stay out of reasons too. An unwritable launch reference can leave the whole server out. A native file that omits an explicit transport is reported as `unknown`.

Permission rows use the source list index, such as `permissions.deny[0]`, and name the rule in the result. Native overrides and partial mappings stay visible. Shell permission output on Codex requires `outputs.codex.exec-policies-from-permissions: true`. The portable default-mode mapping belongs to global sync, so project comparison reports it as unsupported. Permission differences include effective native decisions and partial support, even when both results are translated.

The command fails on unknown targets, the same target twice, invalid specs or config, and external adapters.

```json
{"version": "1", "command": "compare", "targets": ["claude", "cursor"], "coverage": "...", "caveat": "...",
 "specs": [{"kind": "agent", "name": "...", "path": "...", "fields": [{"field": "tools", "differs": true,
   "results": [{"target": "cursor", "status": "unsupported", "reason": "...", "next": "..."}]}]}],
 "fields": 6, "differences": 3}
```

## render

Print what each target writes for one spec, without writing files.

```bash
agnostic-ai render rules/no-console-log.md --target claude,codex
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Targets to render, repeated or comma-separated. Default: all in `agnostic-ai.yaml`. |

Each file prints as `# target: <name>: <output path>` and its body. A target that writes nothing for that kind prints a note.

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
