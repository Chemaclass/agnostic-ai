+++
title = "Inspect and preview"
description = "Explain a source, compare targets, render a spec, and trace routing."
weight = 40

[extra]
group = "Reference"
+++

# Inspect and preview

## explain

List every output file and section one spec contributes to. It works in reverse from the `<!-- source: ... -->` markers in merged documents. With `--file`, list the instructions configured for one source file instead.

Given an [error code](@/docs/errors.md) such as `AAI-003` or a [lint code](@/docs/cli-reference/check.md#lint) such as `LINT011`, it prints the code's cause and fix. A lint code also shows its severity and any `lint` config key that tunes it. `--json` adds `severity` and `config` fields.

```bash
agnostic-ai explain rules/conventional-commits.md --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for editor extensions and scripts. |
| `--global` | Explain a spec in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) or its `local/` layer. A relative path resolves against that root. |

Contributions are grouped by configured target. A "would emit if enabled" list covers inactive adapters. Each entry is tagged `(full file)` or `(section "<name>")`.

With `--global`, contributions read like this:

| Spec | Contribution |
|------|--------------|
| Agent or skill | A whole file. |
| Rule | A section of the instructions file. |
| Hook | Its event in the hooks file. |
| MCP server | Its key in the user MCP file. |
| Settings | Each key it sets, tagged `(key "<key>")`. |

A spec that sets `model` or `effort` also lists the [tier](@/docs/spec-format/agents.md#model-tiers) it names and the model and effort it resolves to on each configured target it reaches. `tool default` means the target writes no model. The list shows the resolved value. A target with no model or effort key for that kind, such as Codex skills, still drops it with a coverage note on `sync`.

```json
{"version": "1", "command": "explain", "spec": {"kind": "rule", "name": "...", "path": "..."},
 "contributions": [{"target": "...", "path": "...", "section": "...", "mode": "full|section|key"}],
 "would_emit_if_enabled": [],
 "model_tier": "strong", "models": [{"target": "...", "model": "...", "effort": "..."}]}
```

`model_tier` and `models` are omitted when the spec sets neither `model` nor `effort`.

For an agent's `can` or `tools`, and for settings `permissions`, the report lists each configured target's native names. Unsupported rules stay visible. A widening line names the extra access a target grants. Native overrides show their values directly and name the field that won. Codex widening follows the merged permission policy and any explicit exec policy source.

```bash
agnostic-ai explain agents/reviewer.md
agnostic-ai explain settings/permissions.yaml --json
```

The JSON `capabilities` array holds `target`, `field`, `capability`, `native`, and `supported`. `widening` lists extra access when present. `override` names a native field or policy source that replaces the portable rule. The array is omitted for specs without these fields. `explain` still shows translations with `on-unsupported: error`; `sync` fails on widening.

### Explain a source file

Start from a project file instead of a spec. The report lists every instruction the target would read from the planned sync output, with its source, output path, selector, and reason. Only Cursor is supported.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Project file to inspect. The file does not have to exist. Cannot combine with a spec or error code argument. |
| `--target <name>` | Required with `--file`. Must be a configured target. Other targets fail with an unsupported-target error. |

| Status | Meaning |
|--------|---------|
| `always` | No file condition: `alwaysApply: true`, or the root `AGENTS.md`. |
| `match` | A `globs` pattern or a nested `AGENTS.md` directory covers the file. |
| `no-match` | A selector exists and misses the file. |
| `model-selected` | `alwaysApply: false` with a description and no globs. Cursor's agent decides. |
| `manual` | `alwaysApply: false` with neither. Loads only when `@`-mentioned. |
| `unknown` | Undocumented glob syntax (braces, classes, negation), or unreadable frontmatter. |
| `excluded` | Target selection (`target`, `targets`, `target-exclude`) leaves the target out. |
| `not-emitted` | The rule targets Cursor but sync writes nothing for it. |

A root `AGENTS.md` written for a peer target such as Codex reaches Cursor too. The report shows what the config makes apply, not what is in the model's active context.

```json
{"version": "1", "command": "explain", "file": "...", "target": "cursor", "note": "...",
 "instructions": [{"status": "match", "source": "...", "output": "...", "selector": "...", "reason": "..."}]}
```

### List generator inputs

```bash
agnostic-ai explain --inputs
```

`--inputs` is its own mode. It lists every file and directory whose change can change a generated output, one per line (`--json` for an array). Paths are relative to the repository root, like a hook manager's glob. It takes no spec and no `--file`. See [git hooks](@/docs/git-hooks.md#check-staged-files). It lists:

- the config files
- `.agnostic-ai/**`
- source directories outside `.agnostic-ai/`
- files that reviews inline with `@path` (and the entry point's, with `sync.resolve-imports: inline`)
- `agnostic.packs.lock`
- `.gitignore`

## compare

Compare how two built-in targets represent the project's agent and skill fields and rule activation, before you switch or add a tool.

```bash
agnostic-ai compare claude cursor
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for scripts. |

It covers agent and skill fields, plus rule `scope`, `paths`, `globs`, and `alwaysApply`. It leaves out hooks and the other spec kinds. Skill fields such as `argument-hint`, `effort`, and `disable-model-invocation` are classified from the files each adapter emits, including Codex policy sidecars. Each field gets one result per target:

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no home for the field or the kind. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or an inexpressible scope. |
| `unknown` | The emission gives no evidence either way. |

`preserved` describes the written file, not runtime behavior. `(differs)` marks a field with a different result per target. Each result names the output paths or the reason, plus a `next:` step when known.

The command fails on unknown targets, the same target twice, invalid specs or config, and external adapters.

```json
{"version": "1", "command": "compare", "targets": ["claude", "cursor"], "coverage": "...", "caveat": "...",
 "specs": [{"kind": "agent", "name": "...", "path": "...", "fields": [{"field": "tools", "differs": true,
   "results": [{"target": "cursor", "status": "unsupported", "reason": "...", "next": "..."}]}]}],
 "fields": 6, "differences": 3}
```

## render

Print what each target emits for one spec, without writing files.

```bash
agnostic-ai render rules/no-console-log.md --target claude,codex
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Targets to render, repeated or comma-separated. Default: all in `agnostic-ai.yaml`. |

Each file prints as `# target: <name>: <output path>` and its body. Targets that emit nothing for the kind print a note.

## graph

Render the spec → target → file dependency graph. It is read-only. See the [graph](@/docs/graph.md) guide.

```bash
agnostic-ai graph --format mermaid --target claude
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default, aligned matrix), `mermaid`, `dot`, `json`. |
| `--target` | Restrict to one target. |
| `--spec` | Restrict to one spec name. |
| `--kind` | Restrict to one kind: agent, skill, rule, hook, mcp, command. |

## why

Show an emitted file's adapter, source spec(s), `outputs.<target>.*` keys, and last sync time. See the [why](@/docs/trace.md) guide.

```bash
agnostic-ai why .claude/rules/no-console-log.md --format json
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default) or `json`. |

