+++
title = "Google Antigravity"
description = "What agnostic-ai writes for Google Antigravity: file paths, what Antigravity supports, and config options."
weight = 140

[extra]
group = "Reference"
target_id = "antigravity"
+++

# Google Antigravity (`antigravity`)

Antigravity reads per-rule files under `.agents/rules/` and custom subagents under `.agents/agents/`. agnostic-ai writes both, plus skills, MCP servers, and hooks.

## Output

```
.agents/AGENTS.md              # entry-point file with a short pointer body
.agents/rules/<name>.md        # one per rule
<scope>/.agents/rules/<name>.md # one per scoped rule
.agents/agents/<name>/agent.md # one per agent (custom subagent)
.agents/skills/<name>/SKILL.md # one folder per skill (Antigravity's native path)
.agents/mcp_config.json        # when MCP entries exist
.agents/hooks.json             # when hook entries exist
.agents/plugins/<name>/plugin.json  # only when a per-kind dir points into a plugin
```

- **Entry point**: `.agents/AGENTS.md` is the [documented per-directory entry point](https://antigravity.google/docs/rules) (`<dir>/.agents/AGENTS.md` or `<dir>/.agents/GEMINI.md`, both `always_on` and frontmatter-free). It stays apart from the root `AGENTS.md` that Codex, Amp, and Warp use.
- **Paths**: rules, skills, and MCP go to the plural `.agents/` form Antigravity prefers ([rules](https://antigravity.google/docs/rules), [skills](https://antigravity.google/docs/skills?tab=ide)). Antigravity lists the singular paths as legacy.
- **Plugins**: Point any output folder or file key at `.agents/plugins/<name>/` to ship those specs as a [workspace plugin](https://antigravity.google/docs/plugins), active only in that project. agnostic-ai writes the required `plugin.json` at the plugin root, one per plugin, covering the five components (`skills/`, `agents/`, `rules/`, `mcp_config.json`, `hooks.json`). A path inside a component, such as `.agents/plugins/team/skills/extra`, gets none.
- **Rules**: every file starts with frontmatter declaring a `trigger`. Antigravity [silently discards](https://antigravity.google/docs/rules) a file without one or with an unknown value. The generated-file header follows the closing `---`.
  - `alwaysApply` is checked first, as in [Devin Desktop / windsurf](@/docs/targets/windsurf.md) and Cursor. `true` writes `trigger: always_on` and ignores `globs`, so the rule that `new rule` creates (`globs: "**/*"` plus `alwaysApply: true`) stays always-on. When unset, it counts as `false` if `globs` holds something other than a catch-all such as `**/*`, and as `true` otherwise.
  - With `alwaysApply: false` (or `globs` and no `alwaysApply`), `globs` becomes `trigger: glob` plus a quoted, comma-joined `globs`. A `description` alone becomes `trigger: model_decision`. Neither becomes `trigger: manual` (loads only on an @-mention). Each case has the field it needs, so no coverage note appears. `description` is kept on any trigger.
  - **`x-antigravity.trigger` overrides the mapping.** `import antigravity` writes it, because the portable fields cannot always recover the trigger. A `manual` rule with a `description` would come back as `model_decision`. An override missing its field (`globs` for `glob`, `description` for `model_decision`), or outside `always_on`/`glob`/`model_decision`/`manual`, becomes `trigger: manual` with a coverage note, never `always_on`.
  - **24,000-byte cap.** Antigravity [truncates](https://antigravity.google/docs/rules) a rule file over 24,000 bytes after expanding `@[label](path)` includes. agnostic-ai writes it in full and adds a coverage note, so split the rule. The count covers the written file (frontmatter, generated-file header, heading) without includes. A large include can pass the check and still exceed the limit.
  - **20,000-token budget.** All `always_on` rules share 20,000 tokens. Past it, Antigravity replaces its largest rules with a `path: description` pointer. `sync` gives no note, so give large rules a `description`.
  - **Scoped rules**: `scope: <dir>` writes to `<dir>/.agents/rules/<name>.md` with the same frontmatter. Antigravity walks up from each file it reads or edits and [loads rules at each level](https://antigravity.google/docs/rules). It scans only the `.md` files directly inside `.agents/rules/`, so rules never nest deeper.
- **Agents**: one custom subagent at `.agents/agents/<name>/agent.md`, the nested form in [Antigravity's subagent reference](https://antigravity.google/docs/subagents) (a flat form also exists). Frontmatter has the required `name` and `description`. The body is the system prompt.
  - Goose and OpenHands scan only top-level `.md` files in that folder. The nested form keeps Antigravity's `model` tier apart from their model IDs. `import antigravity` reads flat files only when no nested profile exists, so it skips their agents.
  - **Devin reads this folder too.** With `windsurf` in `targets`, Devin finds this profile and `.devin/agents/<name>.md` under one name, and only the Devin copy has `allowed-tools`. See the [cross-cutting agents note](@/docs/targets/_index.md).
  - A portable `tools` list is dropped with a coverage note. Antigravity has its own names (`view_file`, `replace_file_content`, `grep_search`, `run_command`, ...), and an unknown one can hang the subagent. Set `x-antigravity.tools` instead.
  - `model` is a tier (`inherit`, `flash`, `pro`), so any other value is dropped the same way.
  - Other documented keys (`mainAgent`, `subagent`, `commandExecutionPolicy`, `mcpServers`, `skills`/`plugins`) go under `x-antigravity`.
- **Skills**: Antigravity's [native skills layout](https://codelabs.developers.google.com/getting-started-with-antigravity-skills), shared with every tool that writes `.agents/skills/` (see [shared skills](@/docs/target-behavior.md)), so identical folders are written once. Frontmatter is cut to `name` and `description`. Files next to the skill are copied unchanged. `sync --global` writes `~/.gemini/config/skills/<name>/`, the [global path](https://antigravity.google/docs/skills?tab=ide) for the IDE and Antigravity 2.0.
- **MCP**: one `mcpServers` object in `.agents/mcp_config.json` ([docs](https://antigravity.google/docs/mcp?tab=ide)).
  - Remote servers use `serverUrl`, not the legacy `url` / `httpUrl` or the `url` form that Claude Code and Cursor share. stdio servers have `command`, `args`, `env`, and `cwd`. Remote ones add `headers`.
  - Both kinds accept `disabled` by that name (see [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target)), unlike Codex and Kilo, which write `enabled: false`.
  - `authProviderType`, `oauth`, `disabledTools`, `description`, `roots`, and any new field go under `x-antigravity`, like Zed and Warp.
- **Hooks**: hooks are merged into `.agents/hooks.json` (or `outputs.antigravity.hooks-file`), the [documented location](https://antigravity.google/docs/hooks?tab=ide).
  - **Keyed by definition name, not event**, unlike every other tool with hooks. Each spec becomes a top-level definition named after it, holding its one event. `disabled: true` writes `enabled: false` beside the event key, the reverse of MCP's literal `disabled`.
  - Five events: `PreToolUse`, `PostToolUse`, `PreInvocation`, `PostInvocation`, `Stop`. Other events are skipped with a coverage note. The first two hold `{matcher, hooks: [...]}` groups. The other three hold a plain handler list and ignore the matcher, which gets a note.
  - Per handler: `type` (defaults to `"command"`, and is always written), `command`, and optional `timeout` (seconds, vendor default 30).
  - A Claude-style matcher is valid regex that matches none of Antigravity's tool names. It is written as is with a coverage note, as in OpenHands, Crush, and Windsurf.
- **Commands**: the public-preview docs do not confirm them, so they are skipped with a warning. `on-unsupported: silent` suppresses it.

{% <details summary="Leftovers from older versions"> %}
- Older versions wrote the undocumented `.agent/AGENTS.md`. Sync renames a file it wrote there to `.agent/AGENTS.md.bak`.
- Sync removes the `.agent/rules` and `.agent/skills` files it wrote, unless `outputs.antigravity.rules-dir` / `skills-dir` opts back into the legacy path.
- Agents used to be flattened into `.agents/rules/agent-<name>.md`, which the subagent loader does not read. Sync removes the copy it wrote for every current agent.
- Sync removes a flat agent profile it wrote when no enabled tool still writes it. With Goose or OpenHands enabled, it stays as their output.
- The IDE still loads `~/.gemini/antigravity/skills/`, which older versions wrote, but Antigravity 2.0 does not. Move those skills or re-run `sync --global`.
{% </details> %}

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.antigravity.rules-dir` | `.agents/rules` | |
| `outputs.antigravity.agents-dir` | `.agents/agents` | |
| `outputs.antigravity.skills-dir` | `.agents/skills` | |
| `outputs.antigravity.mcp-file` | `.agents/mcp_config.json` | |
| `outputs.antigravity.hooks-file` | `.agents/hooks.json` | |
| `outputs.antigravity.rules-file` | unset | writes one legacy merged file and skips the pointer body |

## Global configuration

`agnostic-ai sync --global --only antigravity` adds your home MCP specs to `~/.gemini/config/mcp_config.json`, using the same fields as project output. A disabled server keeps `disabled: true`.

Servers you add under other names stay. A different server with the same name stops sync. See [global MCP servers](@/docs/configuration.md#global-mcp-servers) for how to resolve a conflict.

`agnostic-ai import --global antigravity` reads this file, maps `serverUrl` back to `url`, and keeps extra fields under `x-antigravity`. See [global MCP servers](@/docs/configuration.md#global-mcp-servers).

## Import

`agnostic-ai import antigravity` reads `.agents/rules`, `.agents/skills/` (with bundled assets), and `.agents/AGENTS.md`. It falls back to `.agent/rules`, `.agent/skills/`, and `.agent/AGENTS.md` only when the preferred path is absent. It also looks for `<dir>/.agents/rules/` copies to rebuild scoped rules.

- **Triggers**: `trigger` becomes `alwaysApply` (`always_on` to `true`, `glob`/`model_decision`/`manual` to `false`). It is also kept under `x-antigravity.trigger`, which `sync` reads first, so a `manual` rule with a `description` stays `manual`. An unknown `trigger` gets no `alwaysApply` and a warning, and `sync` writes it as `manual`, never `always_on`.
- **Globs**: `globs` is kept, and the singular `glob:` is merged into it.
- **Agents and MCP**: nested profiles win over flat files. `serverUrl` becomes `url`, and other fields are kept under `x-antigravity`.

## Protected paths

Not enforced. Antigravity takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Antigravity from the Google Antigravity public-preview download page.
2. Check the tree: `ls .agents/AGENTS.md .agents/rules/ .agents/agents/ .agents/skills/`, `grep "Generated by agnostic-ai" .agents/rules/*.md` for the generated-file header (after the frontmatter), `ls .agents/skills/*/SKILL.md >/dev/null`, and `python -m json.tool .agents/mcp_config.json > /dev/null` when MCP specs exist.
3. Open the project. `.agents/AGENTS.md` shows in the project-instructions panel with no "unrecognized file" warnings.
4. Open a `.agents/rules/<name>.md` and confirm Antigravity picks it up. Each `.agents/skills/<name>/SKILL.md` loads as a skill, and each `.agents/mcp_config.json` server appears in the MCP panel.
5. Ask the agent to delegate to a custom subagent by name. Each `.agents/agents/<name>/agent.md` is selectable in the subagent panel and its task reaches Idle. A hang points at an unmapped `tools` name. Check `x-antigravity.tools` against the vendor's names.
6. Trigger an agent action, such as a refactor. The rules apply, with no schema-validation log entries about `.agents/`.
