+++
title = "Google Antigravity"
description = "How agnostic-ai emits Google Antigravity configuration: native paths, capability limits, and output options."
weight = 140

[extra]
group = "Reference"
target_id = "antigravity"
+++

# Google Antigravity (`antigravity`)

Antigravity reads per-rule files under `.agents/rules/` and custom subagents under `.agents/agents/`. agnostic-ai writes both, plus skills, MCP servers, and hooks.

## Output

```
.agents/AGENTS.md              # entry-point pointer body
.agents/rules/<name>.md        # one per rule
<scope>/.agents/rules/<name>.md # one per scoped rule
.agents/agents/<name>/agent.md # one per agent (custom subagent)
.agents/skills/<name>/SKILL.md # one folder per skill (Antigravity's native path)
.agents/mcp_config.json        # when MCP entries exist
.agents/hooks.json             # when hook entries exist
.agents/plugins/<name>/plugin.json  # only when a per-kind dir points into a plugin
```

- **Entry point**: `.agents/AGENTS.md` is the [documented per-directory entry point](https://antigravity.google/docs/rules) (`<dir>/.agents/AGENTS.md` or `<dir>/.agents/GEMINI.md`, both `always_on` and frontmatter-free). It stays clear of the root `AGENTS.md` that codex, amp, and warp own.
- **Paths**: rules, skills, and MCP default to the plural `.agents/` form Antigravity prefers ([rules](https://antigravity.google/docs/rules), [skills](https://antigravity.google/docs/skills?tab=ide)). Antigravity lists the singular paths as legacy.
- **Plugins**: point any per-kind output key at `.agents/plugins/<name>/` to ship those specs as a [workspace plugin](https://antigravity.google/docs/plugins), active only in that project. agnostic-ai writes the required `plugin.json` at the plugin root: one per plugin, across the five components (`skills/`, `agents/`, `rules/`, `mcp_config.json`, `hooks.json`). A path inside a component, such as `.agents/plugins/team/skills/extra`, gets none.
- **Rules**: every file starts with frontmatter declaring a `trigger`. Antigravity [silently discards](https://antigravity.google/docs/rules) a file without one or with an unknown value. The provenance header follows the closing `---`.
  - `alwaysApply` decides first, as in [Devin Desktop / windsurf](@/docs/targets/windsurf.md) and Cursor. `true` writes `trigger: always_on` and ignores `globs`, so `new rule`'s seed (`globs: "**/*"` plus `alwaysApply: true`) stays always-on. When unset, it counts as `false` if `globs` holds something other than a catch-all such as `**/*`, and `true` otherwise.
  - With `alwaysApply: false` (or `globs` and no `alwaysApply`), `globs` maps to `trigger: glob` plus a quoted, comma-joined `globs`. A bare `description` maps to `trigger: model_decision`. Neither maps to `trigger: manual` (load only on an @-mention). Each branch has its companion field, so no coverage note fires. `description` carries through on any trigger.
  - **`x-antigravity.trigger` overrides the mapping.** `import antigravity` writes it, because the generic fields can't always recover the trigger. A `manual` rule with a `description` would re-derive as `model_decision`. An override missing its companion field (`globs` for `glob`, `description` for `model_decision`), or outside `always_on`/`glob`/`model_decision`/`manual`, falls to `trigger: manual` with a coverage note, never `always_on`.
  - **24,000-byte cap.** Antigravity [truncates](https://antigravity.google/docs/rules) a rule file over 24,000 bytes after expanding `@[label](path)` includes. agnostic-ai emits it in full with a coverage note, so split the rule. The count covers the emitted file (frontmatter, provenance header, heading) without includes. A large include can pass the note and still exceed the limit.
  - **20,000-token budget.** All `always_on` rules share 20,000 tokens. Past it, Antigravity demotes its largest rules to a `path: description` pointer. `sync` gives no note, so give large rules a `description`.
  - **Scoped rules**: `scope: <dir>` emits to `<dir>/.agents/rules/<name>.md` with the same frontmatter. Antigravity walks up from each file it reads or edits and [loads rules at each level](https://antigravity.google/docs/rules). It scans only immediate `.md` children of `.agents/rules/`, so rules never nest deeper.
- **Agents**: one custom subagent at `.agents/agents/<name>/agent.md`, the nested form in [Antigravity's subagent reference](https://antigravity.google/docs/subagents) (a flat form also exists). Frontmatter carries the required `name` and `description`. The body is the system prompt.
  - Goose and OpenHands scan only top-level `.md` files in that root, so the nested form keeps Antigravity's `model` tier apart from their model IDs. `import antigravity` reads flat files only when no nested profile exists, so it skips their agents.
  - **Devin reads this tree too.** With `windsurf` in `targets`, Devin finds this profile and `.devin/agents/<name>.md` under one name, and only the Devin copy has `allowed-tools`. See the [cross-cutting agents note](@/docs/targets/_index.md).
  - A generic `tools` list drops with a coverage note, since Antigravity has its own names (`view_file`, `replace_file_content`, `grep_search`, `run_command`, ...), and an unknown one can hang the subagent. Set `x-antigravity.tools` instead.
  - `model` is a tier (`inherit`, `flash`, `pro`), so any other value drops the same way.
  - Other documented keys (`mainAgent`, `subagent`, `commandExecutionPolicy`, `mcpServers`, `skills`/`plugins`) go through `x-antigravity`.
- **Skills**: Antigravity's [native skills layout](https://codelabs.developers.google.com/getting-started-with-antigravity-skills), shared with every target that writes `.agents/skills/` (see [shared skills](@/docs/target-behavior.md)), so identical folders dedupe. Frontmatter is reduced to `name` and `description`. Sibling files copy byte-for-byte. `sync --global` writes `~/.gemini/config/skills/<name>/`, the [global path](https://antigravity.google/docs/skills?tab=ide) for the IDE and Antigravity 2.0.
- **MCP**: one `mcpServers` object in `.agents/mcp_config.json` ([docs](https://antigravity.google/docs/mcp?tab=ide)).
  - Remote servers use `serverUrl`, not the legacy `url` / `httpUrl` or the `url` shape claude and cursor share. stdio servers carry `command`, `args`, `env`, and `cwd`. Remote ones add `headers`.
  - Both accept `disabled` by that name (see [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target)), unlike codex and kilo, which write `enabled: false`.
  - `authProviderType`, `oauth`, `disabledTools`, `description`, `roots`, and any future field go through `x-antigravity`, like Zed and Warp.
- **Hooks**: merge into `.agents/hooks.json` (or `outputs.antigravity.hooks-file`), the [documented location](https://antigravity.google/docs/hooks?tab=ide).
  - **Keyed by definition name, not event**, unlike every other hook target. Each spec becomes a top-level definition named after it, holding its one event. `disabled: true` writes `enabled: false` beside the event key, the inverse of MCP's literal `disabled`.
  - Five events: `PreToolUse`, `PostToolUse`, `PreInvocation`, `PostInvocation`, `Stop`. Others skip with a coverage note. The first two hold `{matcher, hooks: [...]}` groups. The other three hold a plain handler list and ignore the matcher, which gets a note.
  - Per handler: `type` (defaults to `"command"`, written explicitly), `command`, and optional `timeout` (seconds, vendor default 30).
  - A Claude-style matcher is valid regex that matches none of Antigravity's tool names. It emits verbatim with a coverage note, as in OpenHands, Crush, and Windsurf.
  - The payload's `transcriptPath` is under `~/.gemini/antigravity-ide`, which confirms the IDE runs the hooks.
- **Commands**: the public-preview docs don't confirm them, so they skip with a warning. `on-unsupported: silent` suppresses it.

{% <details summary="Leftovers from older versions"> %}
- Older versions wrote the undocumented `.agent/AGENTS.md`. Sync renames a managed leftover to `.agent/AGENTS.md.bak`.
- Sync sweeps a stale managed `.agent/rules` / `.agent/skills` tree, unless `outputs.antigravity.rules-dir` / `skills-dir` opts back into the legacy path.
- Agents used to be flattened into `.agents/rules/agent-<name>.md`, which the subagent loader doesn't read. Sync sweeps a managed copy for every current agent.
- The sync ledger removes a managed flat agent profile when no enabled target still writes it. With Goose or OpenHands enabled, it stays as their output.
- The IDE still loads `~/.gemini/antigravity/skills/`, which older versions wrote, but Antigravity 2.0 doesn't. Move those skills or re-run `sync --global`.
{% </details> %}

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

`agnostic-ai import antigravity` reads `.agents/rules`, `.agents/skills/` (with bundled assets), and `.agents/AGENTS.md`. It falls back to `.agent/rules`, `.agent/skills/`, and `.agent/AGENTS.md` only when the preferred path is absent. It scans for `<dir>/.agents/rules/` copies to rebuild scoped rules.

- **Triggers**: `trigger` maps to `alwaysApply` (`always_on` to `true`, `glob`/`model_decision`/`manual` to `false`). It also stays under `x-antigravity.trigger`, which `sync` reads first, so a `manual` rule with a `description` round-trips as `manual`. An unknown `trigger` gets no `alwaysApply` and a warning, and `sync` renders it as `manual`, never `always_on`.
- **Globs**: `globs` carries through, and the singular `glob:` folds onto it.
- **Agents and MCP**: nested profiles win over flat files. `serverUrl` becomes `url`, and other fields stay under `x-antigravity`.

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Antigravity from the Google Antigravity public-preview download page.
2. Check the tree: `ls .agents/AGENTS.md .agents/rules/ .agents/agents/ .agents/skills/`, `grep "Generated by agnostic-ai" .agents/rules/*.md` for the provenance header (after the frontmatter), `ls .agents/skills/*/SKILL.md >/dev/null`, and `python -m json.tool .agents/mcp_config.json > /dev/null` when MCP specs exist.
3. Open the project. `.agents/AGENTS.md` shows in the project-instructions panel with no "unrecognized file" warnings.
4. Open a `.agents/rules/<name>.md` and confirm Antigravity picks it up. Each `.agents/skills/<name>/SKILL.md` loads as a skill, and each `.agents/mcp_config.json` server appears in the MCP panel.
5. Ask the agent to delegate to a custom subagent by name. Each `.agents/agents/<name>/agent.md` is selectable in the subagent panel and its task reaches Idle. A hang points at an unmapped `tools` name. Check `x-antigravity.tools` against the vendor's names.
6. Trigger an agent action, such as a refactor. The rules apply, with no schema-validation log entries about `.agents/`.
