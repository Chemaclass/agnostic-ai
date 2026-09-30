+++
title = "Inspect and preview"
description = "Explain a source, compare targets, render a spec, and trace routing."
weight = 40

[extra]
group = "Reference"
+++

# Inspect and preview

## explain

List every output file and section one spec contributes to, the reverse of the `<!-- source: ... -->` markers in merged documents. With `--file`, list the instructions configured for one source file instead.

```bash
agnostic-ai explain rules/conventional-commits.md --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for editor extensions and scripts. |
| `--global` | Explain a spec in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) or its `local/` layer. A relative path resolves against that root. |

Contributions are grouped by configured target, plus a "would emit if enabled" list for inactive adapters, tagged `(full file)` or `(section "<name>")`. With `--global`, an agent or skill is a whole file, a rule is a section of the instructions file, a hook is its event in the hooks file, an MCP server is its key in the user MCP file, and a settings spec lists each key it sets, tagged `(key "<key>")`.

A spec that sets `model` or `effort` also lists the model and effort each configured target it reaches resolves to, and the [tier](@/docs/spec-format/agents.md#model-tiers) it names. `tool default` means the target writes no model. A target that has no effort key still drops the effort, with a coverage note on `sync`.

```json
{"version": "1", "command": "explain", "spec": {"kind": "rule", "name": "...", "path": "..."},
 "contributions": [{"target": "...", "path": "...", "section": "...", "mode": "full|section|key"}],
 "would_emit_if_enabled": [],
 "model_tier": "strong", "models": [{"target": "...", "model": "...", "effort": "..."}]}
```

`model_tier` and `models` are omitted when the spec sets neither `model` nor `effort`.

### Explain a source file

Start from a project file instead of a spec. The report lists every instruction the target would read from the planned sync output, with its source, output path, selector, and reason. Cursor is the only supported target.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Project file to inspect. The file does not have to exist. Cannot be combined with a spec or error code argument. |
| `--target <name>` | Required with `--file`. Must be a configured target. Other targets fail with an unsupported-target error. |
| `--inputs` | List every file and directory whose change can change a generated output, one per line (`--json` for an array): the config files, `.agnostic-ai/**`, source directories outside it, files reviews inline with `@path` (and the entry point's, with `sync.resolve-imports: inline`), `agnostic.packs.lock`, and `.gitignore`. Paths are relative to the repository root, like a hook manager's glob. Takes no spec. See [git hooks](@/docs/git-hooks.md#check-staged-files). |

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

A root `AGENTS.md` written for a peer target such as Codex reaches Cursor too. The report shows configured applicability, not the model's active context.

```json
{"version": "1", "command": "explain", "file": "...", "target": "cursor", "note": "...",
 "instructions": [{"status": "match", "source": "...", "output": "...", "selector": "...", "reason": "..."}]}
```

## compare

Compare how two built-in targets represent the project's agent and skill fields and rule activation, before you switch or add a tool.

```bash
agnostic-ai compare claude cursor
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for scripts. |

Coverage is agent and skill fields plus rule `scope`, `paths`, `globs`, and `alwaysApply`. Hooks and the other spec kinds are left out. Skill fields such as `argument-hint`, `effort`, and `disable-model-invocation` are classified from the files each adapter emits, including Codex policy sidecars. Each field gets one result per target:

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no home for the field or the kind. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or an inexpressible scope. |
| `unknown` | The emission gives no evidence either way. |

`preserved` describes the written file, not runtime behavior. `(differs)` marks a field with a different result per target. Each result names the output paths or reason, plus a `next:` step when known. Unknown targets, the same target twice, invalid specs or config, and external adapters fail the command.

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

Render the spec → target → file dependency graph. Read-only. Full guide in [graph](@/docs/graph.md).

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

Show an emitted file's adapter, source spec(s), `outputs.<target>.*` keys, and last sync time. Full guide in [why](@/docs/trace.md).

```bash
agnostic-ai why .claude/rules/no-console-log.md --format json
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default) or `json`. |

