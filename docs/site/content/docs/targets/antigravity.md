+++
title = "Google Antigravity"
description = "How agnostic-ai emits Google Antigravity configuration: native paths, capability limits, and output options."
weight = 140

[extra]
group = "Reference"
target_id = "antigravity"
+++

# Google Antigravity (`antigravity`)

## Output

```
.agents/AGENTS.md              # entry-point pointer body
.agents/rules/<name>.md        # one per rule
<scope>/.agents/rules/<name>.md # one per scoped rule
.agents/agents/<name>/agent.md # one per agent (custom subagent)
.agents/skills/<name>/SKILL.md # one folder per skill (Antigravity's native path)
.agents/mcp_config.json        # when MCP entries exist
.agents/plugins/<name>/plugin.json  # only when a per-kind dir points into a plugin
```

Antigravity reads per-rule files under `.agents/rules/` and custom subagents under `.agents/agents/`. The adapter emits both.

Point any per-kind output key at `.agents/plugins/<name>/` to ship those specs as a [workspace plugin](https://antigravity.google/docs/plugins), active only in that project. agnostic-ai writes the required `plugin.json` manifest at the plugin root. The five documented components are `skills/`, `agents/`, `rules/`, `mcp_config.json`, and `hooks.json`; a plugin with several of them gets one manifest. A path inside a component, such as `.agents/plugins/team/skills/extra`, is not a component and gets none.

`.agents/AGENTS.md` is the [documented per-directory entry point](https://antigravity.google/docs/rules) (`<dir>/.agents/AGENTS.md` or `<dir>/.agents/GEMINI.md`, both `always_on` and frontmatter-free). It keeps clear of the root `AGENTS.md` that codex, amp, and warp own, and rule bodies still land in `.agents/rules/`. Older versions wrote the undocumented `.agent/AGENTS.md`: sync renames a managed leftover there to `.agent/AGENTS.md.bak`, and import reads the new path first, falling back to the old one only when it is absent.

Rules, skills, and MCP default to the plural `.agents/` form Antigravity prefers ([rules](https://antigravity.google/docs/rules), [skills](https://antigravity.google/docs/skills?tab=ide)); the singular paths are listed as legacy. A stale managed tree at `.agent/rules` / `.agent/skills` is swept on sync unless `outputs.antigravity.rules-dir` / `skills-dir` explicitly opts back into the legacy path.

- **Rules**: every rule file starts with YAML frontmatter declaring a `trigger`. Antigravity [silently discards](https://antigravity.google/docs/rules) a file without one or with an unknown value. The provenance header goes after the closing `---`.
  - `alwaysApply` decides the trigger first, the same precedence [Devin Desktop / windsurf](@/docs/targets/windsurf.md) and Cursor use. `true` or unset writes `trigger: always_on` and ignores `globs`, so `new rule`'s default seed (`globs: "**/*"` plus `alwaysApply: true`) stays always-on.
  - With `alwaysApply: false`: `globs` maps to `trigger: glob` plus a quoted, comma-joined `globs`; a bare `description` maps to `trigger: model_decision`; neither maps to `trigger: manual` (load only on an @-mention). Each branch already has its required companion field, so no coverage note fires. `description` carries through on any trigger.
  - **`x-antigravity.trigger` overrides the generic mapping.** `import antigravity` writes it from the project's own `trigger`, because the generic fields cannot always recover it (a `manual` rule with a `description` would re-derive as `model_decision`). A valid override missing its companion field (`globs` for `glob`, `description` for `model_decision`), or a value outside `always_on`/`glob`/`model_decision`/`manual`, falls to `trigger: manual` with a coverage note, never to `always_on`.
  - **24,000-byte per-file cap.** Antigravity [truncates](https://antigravity.google/docs/rules) any rule file over 24,000 bytes after expanding `@[label](path)` includes. agnostic-ai never truncates: an over-cap rule emits in full and `sync` reports a coverage note. The count covers the emitted file (frontmatter, provenance header, and heading included) with no includes resolved, so a rule with a large include can pass the note and still exceed the real limit. Split the rule to stay under.
  - **20,000-token aggregate budget.** All `always_on` rules share one 20,000-token budget. Past it, Antigravity demotes its largest rules to a `path: description` pointer instead of cutting them. `sync` raises no note for this; give large rules a `description` so the pointer stays useful.
  - **Scoped rules.** A rule with `scope: <dir>` emits to `<dir>/.agents/rules/<name>.md`. Antigravity walks up the tree from each file it reads or edits and [loads rules at each level](https://antigravity.google/docs/rules). The frontmatter matches unscoped rules. It never nests deeper, because Antigravity scans only immediate `.md` children of `.agents/rules/`.
- **Agents**: one custom subagent per agent at `.agents/agents/<name>/agent.md`, the nested form in [Antigravity's subagent reference](https://antigravity.google/docs/subagents) (a flat form also exists).
  - Goose and OpenHands scan only top-level `.md` files in the same root, so the nested form keeps Antigravity's `model` tier apart from their free-form model IDs. Frontmatter carries the required `name` and `description`; the body is the system prompt.
  - The sync ledger removes a managed flat profile from an earlier sync when no enabled target still writes it. With Goose or OpenHands enabled, that flat path stays as their shared output. `import antigravity` prefers nested profiles and reads flat files only when no nested profile exists, so it does not ingest Goose or OpenHands agents.
  - Agents used to be flattened into `.agents/rules/agent-<name>.md`, which the subagent loader never reads. A managed copy at the old name is swept for every current agent.
  - **Devin reads this tree too.** With `windsurf` in `targets`, one agent spec produces this profile and `.devin/agents/<name>.md`, and Devin finds both under one name. Only the Devin copy has `allowed-tools`. See the [cross-cutting agents note](@/docs/targets/_index.md) for the cost and workaround.
  - A generic `tools` list is dropped with a coverage note. Antigravity uses its own tool names (`view_file`, `replace_file_content`, `grep_search`, `run_command`, ...), and an unknown name can hang the subagent. Set `x-antigravity.tools` with Antigravity's names.
  - `model` is a tier enum (`inherit`, `flash`, `pro`), not a model ID, so any other value is dropped the same way.
  - Other documented keys (`mainAgent`, `subagent`, `commandExecutionPolicy`, `mcpServers`, `skills`/`plugins`) go through `x-antigravity`.
- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md`, Antigravity's [native skills layout](https://codelabs.developers.google.com/getting-started-with-antigravity-skills). Codex, Amp, Zed, Crush, and OpenHands share this tree, so identical folders dedupe. Frontmatter is reduced to `name` and `description`. Sibling files copy byte-for-byte.
  - `sync --global` writes to `~/.gemini/config/skills/<name>/`, the [global path](https://antigravity.google/docs/skills?tab=ide) for both the IDE and Antigravity 2.0. The IDE still loads the legacy `~/.gemini/antigravity/skills/`, which older agnostic-ai versions wrote, but Antigravity 2.0 does not. Move those skills or re-run `sync --global`.
- **MCP**: servers land in `.agents/mcp_config.json` under one `mcpServers` object ([docs](https://antigravity.google/docs/mcp?tab=ide)).
  - Remote servers use `serverUrl`; the legacy `url` / `httpUrl` names are not supported. This is a dedicated schema, not the `url` shape claude and cursor share.
  - stdio servers carry `command`, `args`, `env`, and `cwd`; remote servers add `headers`.
  - Both transports accept `disabled` by that name (see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target)), unlike codex and kilo, which map it to `enabled: false`.
  - `authProviderType`, `oauth`, and `disabledTools` have no dedicated mapping. They, `description`, `roots`, and any future field go through `x-antigravity`, like Zed and Warp.
  - `import antigravity` renames `serverUrl` to `url` and keeps other fields under `x-antigravity`.
- **Hooks**: merge into `.agents/hooks.json` (override via `outputs.antigravity.hooks-file`), the [documented location](https://antigravity.google/docs/hooks?tab=ide).
  - **The file is keyed by hook definition name, not by event**, unlike every other hook target. Each spec becomes its own top-level definition, named after the spec, holding its one event.
  - A spec's `disabled: true` writes `enabled: false` on its definition, beside the event key. This is the inverse of the literal `disabled` this adapter writes for MCP servers.
  - Five events: `PreToolUse`, `PostToolUse`, `PreInvocation`, `PostInvocation`, `Stop`. The first two hold `{matcher, hooks: [...]}` groups. The other three hold a plain handler list and ignore the matcher; the adapter writes each shape and notes a matcher set on those three.
  - Per handler: `type` (defaults to `"command"`, written explicitly), `command`, and optional `timeout` (seconds, vendor default 30). Any other event is skipped with a coverage note.
  - Antigravity uses its own tool names, so a matcher from a Claude spec is valid regex that matches nothing. It emits verbatim with a coverage note instead of a guessed rename, as OpenHands, Crush, and Windsurf do.
  - The hook payload's `transcriptPath` is under `~/.gemini/antigravity-ide`, which confirms the IDE runs the hooks.

Commands are unconfirmed in the public-preview docs and skip with a warning. Add `on-unsupported: silent` to suppress it.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.antigravity.rules-dir` | `.agents/rules` | |
| `outputs.antigravity.agents-dir` | `.agents/agents` | |
| `outputs.antigravity.skills-dir` | `.agents/skills` | |
| `outputs.antigravity.mcp-file` | `.agents/mcp_config.json` | |
| `outputs.antigravity.hooks-file` | `.agents/hooks.json` | |
| `outputs.antigravity.rules-file` | unset | writes a legacy merged document and skips the pointer-body write |

## Import

`agnostic-ai import antigravity` reads `.agents/rules` and `.agents/skills/`, falling back to the singular `.agent/rules` and `.agent/skills/` only when the preferred directory is absent. It also scans the tree for `<dir>/.agents/rules/` copies to rebuild scoped rules. Skill imports include bundled assets.

A rule's `trigger` maps to `alwaysApply` (`always_on` to `true`, `glob`/`model_decision`/`manual` to `false`) and is also kept under `x-antigravity.trigger`. `sync` reads that override first, so a `manual` rule with a `description` round-trips as `manual`. `globs` carries through, and the singular `glob:` folds onto it. An unknown `trigger` gets no `alwaysApply` mapping and a warning; `x-antigravity.trigger` keeps it, but `sync` renders it as `manual`, never `always_on`. The entry point is read from `.agents/AGENTS.md`, falling back to the legacy `.agent/AGENTS.md`.

Agents and MCP servers import as described under **Agents** and **MCP**: nested profiles win over legacy flat files, and `serverUrl` becomes `url`.

## Verify

1. Install Antigravity from the Google Antigravity public-preview download page.
2. Check the tree: `ls .agents/AGENTS.md .agents/rules/ .agents/agents/ .agents/skills/`, `grep "Generated by agnostic-ai" .agents/rules/*.md` for the provenance header (after the frontmatter), `test -f .agents/skills/*/SKILL.md`, and `python -m json.tool .agents/mcp_config.json > /dev/null` when MCP specs exist.
3. Open the project. `.agents/AGENTS.md` shows in the project-instructions panel with no "unrecognized file" warnings.
4. Open a `.agents/rules/<name>.md` and confirm it is picked up. Each `.agents/skills/<name>/SKILL.md` loads as a skill, and each `.agents/mcp_config.json` server appears in the MCP panel.
5. Ask the agent to delegate to a custom subagent by name. Each `.agents/agents/<name>/agent.md` is selectable in the subagent panel and its task reaches Idle. A hang points at an unmapped `tools` name, so check `x-antigravity.tools` against the vendor's names.
6. Trigger an agent action (e.g. ask for a refactor). The rules apply, with no schema-validation log entries about `.agents/`.
