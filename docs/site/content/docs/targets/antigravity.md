+++
title = "Google Antigravity"
description = "What agnostic-ai writes for Google Antigravity: file paths, what Antigravity supports, and config options."
weight = 140

[extra]
group = "Reference"
target_id = "antigravity"

[extra.moved]
set-up-this-tool = "@/docs/targets/antigravity.md#first-rule"
+++

# Google Antigravity (`antigravity`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Antigravity reads per-rule files under `.agents/rules/` and custom subagents under `.agents/agents/`. agnostic-ai writes both, plus skills, MCP servers, and hooks.

## First rule

Install the client using the [official setup guide](https://antigravity.google/download), then open your project in it.

Select `antigravity` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `.agents/rules/conventional-commits.md`.

Rules need native trigger frontmatter. Sync supplies it from the portable activation fields.

For existing files, check [what `import antigravity` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `.agents/rules/conventional-commits.md` for the commit-prefix rule. Open the project in Antigravity. Confirm the rule is Always On, then ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Shared memory

With `builtins: [memory]`, sync writes `antigravity-memory-index.md` as an always-on rule. Native `@[label](path)` includes load the project index and checkout personal index. Paths resolve from the generated rule file, including when `outputs.antigravity.rules-dir` changes. The rule stays identical whether either index exists. Sync reads existing indexes to check the expanded rule size and does not create empty ones.

With `memory.personal: repo` in local configuration, only project memory is included. The shared policy finds personal memory through `agnostic-ai memory path`. Global sync keeps that policy and omits project-file includes.

Antigravity expands includes before checking its 24,000-byte rule limit. All always-on rules share a 20,000-token budget, which can replace large rules with pointers. Missing-index behavior is not documented. See the [official file include and size rules](https://www.antigravity.google/docs/rules/).

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

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>

### Entry point

`.agents/AGENTS.md` is the [documented per-directory entry point](https://antigravity.google/docs/rules) (`<dir>/.agents/AGENTS.md` or `<dir>/.agents/GEMINI.md`, both `always_on` and frontmatter-free). It stays apart from the root `AGENTS.md` that Codex, Amp, and Warp use.

### Paths

Rules, skills, and MCP go to the plural `.agents/` form Antigravity prefers ([rules](https://antigravity.google/docs/rules), [skills](https://antigravity.google/docs/skills?tab=ide)). Antigravity lists the singular paths as legacy.

### Plugins

Point any output folder or file key at `.agents/plugins/<name>/` to ship those specs as a [workspace plugin](https://antigravity.google/docs/plugins), active only in that project. agnostic-ai writes the required `plugin.json` at the plugin root, one per plugin, covering the five components (`skills/`, `agents/`, `rules/`, `mcp_config.json`, `hooks.json`). A path inside a component, such as `.agents/plugins/team/skills/extra`, gets none.

### Rules

Every file starts with frontmatter declaring a `trigger`. Antigravity [silently discards](https://antigravity.google/docs/rules) a file without one or with an unknown value. The generated-file header follows the closing `---`.

- `alwaysApply` is checked first, as in [Devin Desktop / windsurf](@/docs/targets/windsurf.md) and Cursor. `true` writes `trigger: always_on` and ignores `globs`, so the rule that `new rule` creates (`globs: "**/*"` plus `alwaysApply: true`) stays always-on. When unset, it counts as `false` if `globs` holds something other than a catch-all such as `**/*`, and as `true` otherwise.
- With `alwaysApply: false` (or `globs` and no `alwaysApply`), `globs` becomes `trigger: glob` plus a quoted, comma-joined `globs`. A `description` alone becomes `trigger: model_decision`. Neither becomes `trigger: manual` (loads only on an @-mention). Each case has the field it needs, so no sync note appears. `description` is kept on any trigger.
- **`x-antigravity.trigger` overrides the mapping.** `import antigravity` writes it, because the portable fields cannot always recover the trigger. A `manual` rule with a `description` would come back as `model_decision`. An override missing its field (`globs` for `glob`, `description` for `model_decision`), or outside `always_on`/`glob`/`model_decision`/`manual`, becomes `trigger: manual` with a sync note, never `always_on`.
- **24,000-byte cap.** Antigravity [truncates](https://antigravity.google/docs/rules) a rule file over 24,000 bytes after expanding `@[label](path)` includes. agnostic-ai writes it in full and adds a sync note, so split the rule. The count covers the written file (frontmatter, generated-file header, heading) and expands the built-in memory rule's project and checkout personal indexes, stripping included frontmatter. Other rules' includes are not counted.
- **20,000-token budget.** All `always_on` rules share 20,000 tokens. Past it, Antigravity replaces its largest rules with a `path: description` pointer. `sync` gives no note, so give large rules a `description`.
- **Scoped rules**: `scope: <dir>` writes to `<dir>/.agents/rules/<name>.md` with the same frontmatter. Antigravity walks up from each file it reads or edits and [loads rules at each level](https://antigravity.google/docs/rules). It scans only the `.md` files directly inside `.agents/rules/`, so rules never nest deeper.

### Agents

One custom subagent at `.agents/agents/<name>/agent.md`, the nested form in [Antigravity's subagent reference](https://antigravity.google/docs/subagents) (a flat form also exists). Frontmatter has the required `name` and `description`. The body is the system prompt.

- Goose and OpenHands scan only top-level `.md` files in that folder. The nested form keeps Antigravity's `model` tier apart from their model IDs. `import antigravity` reads flat files only when no nested profile exists, so it skips their agents.
- **Devin reads this folder too.** With `windsurf` in `targets`, Devin finds this profile and `.devin/agents/<name>.md` under one name, and only the Devin copy has `allowed-tools`. See the [shared agent discovery](@/docs/target-behavior.md#spec-kind-behavior).
- A portable `tools` list is dropped with a sync note. Antigravity has its own names (`view_file`, `replace_file_content`, `grep_search`, `run_command`, ...), and an unknown one can hang the subagent. Set `x-antigravity.tools` instead.
- `model` is a tier (`inherit`, `flash`, `pro`), so any other value is dropped the same way.
- Other documented keys (`mainAgent`, `subagent`, `commandExecutionPolicy`, `mcpServers`, `skills`/`plugins`) go under `x-antigravity`.

### Skills

Antigravity's [native skills layout](https://codelabs.developers.google.com/getting-started-with-antigravity-skills), shared with every tool that writes `.agents/skills/` (see [shared skills](@/docs/target-behavior.md)), so identical folders are written once. Frontmatter is cut to `name` and `description`. Files next to the skill are copied unchanged. `sync --global` writes `~/.gemini/config/skills/<name>/`, the [global path](https://antigravity.google/docs/skills?tab=ide) for the IDE and Antigravity app.

Antigravity documents no field that keeps a skill user-only: the agent reads relevant skills on its own, and `/<skill-name>` runs one by hand ([Antigravity skills](https://antigravity.google/docs/skills)). Its `disable-slash-command: true` does the opposite, leaving the skill to the model. So `disable-model-invocation` is dropped with a note, and commands are skipped.

### MCP

One `mcpServers` object in `.agents/mcp_config.json` ([docs](https://antigravity.google/docs/mcp?tab=ide)).

- Remote servers use `serverUrl`, not the legacy `url` / `httpUrl` or the `url` form that Claude Code and Cursor share. stdio servers have `command`, `args`, `env`, and `cwd`. Remote ones add `headers`.
- Both kinds accept `disabled` by that name (see [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target)), unlike Codex and Kilo, which write `enabled: false`.
- `authProviderType`, `oauth`, `disabledTools`, `description`, `roots`, and any new field go under `x-antigravity`, like Zed and Warp.

### Hooks

Hooks are merged into `.agents/hooks.json` (or `outputs.antigravity.hooks-file`), the [documented location](https://antigravity.google/docs/hooks?tab=ide).

- **Keyed by definition name, not event**, unlike every other tool with hooks. Each spec becomes a top-level definition named after it, holding its one event. `disabled: true` writes `enabled: false` beside the event key, the reverse of MCP's literal `disabled`.
- Six events: `PreToolUse`, `PostToolUse`, `SessionStart`, `PreInvocation`, `PostInvocation`, `Stop`. Other events are skipped with a sync note. The first two hold `{matcher, hooks: [...]}` groups. The other four hold a plain handler list and ignore the matcher, which gets a note. `SessionStart` runs once when a conversation starts, before the first model call. The portable `on: session-start` is not mapped; set `event: SessionStart` or `x-antigravity: {event: SessionStart}`.
- Per handler: `type` (defaults to `"command"`, and is always written), `command`, and optional `timeout` (seconds, vendor default 30).
- A Claude-style matcher is valid regex that matches none of Antigravity's tool names. It is written as is with a sync note, as in OpenHands, Crush, and Windsurf.

### Commands

The public-preview docs do not confirm them, so they are skipped with a warning. `on-unsupported: silent` suppresses it.

<details>
<summary>Leftovers from older versions</summary>

- Older versions wrote the undocumented `.agent/AGENTS.md`. Sync renames a file it wrote there to `.agent/AGENTS.md.bak`.
- Sync removes the `.agent/rules` and `.agent/skills` files it wrote, unless `outputs.antigravity.rules-dir` / `skills-dir` opts back into the legacy path.
- Agents used to be flattened into `.agents/rules/agent-<name>.md`, which the subagent loader does not read. Sync removes the copy it wrote for every current agent.
- Sync removes a flat agent profile it wrote when no enabled tool still writes it. With Goose or OpenHands enabled, it stays as their output.
- The IDE still loads `~/.gemini/antigravity/skills/`, which older versions wrote, but the Antigravity app does not. Move those skills or re-run `sync --global`.

</details>

</details>

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.antigravity.rules-dir` | `.agents/rules` | |
| `outputs.antigravity.agents-dir` | `.agents/agents` | |
| `outputs.antigravity.skills-dir` | `.agents/skills` | |
| `outputs.antigravity.mcp-file` | `.agents/mcp_config.json` | |
| `outputs.antigravity.hooks-file` | `.agents/hooks.json` | |
| `outputs.antigravity.rules-file` | unset | writes one legacy merged file and skips the instructions that point to source specs |

## Global configuration

`agnostic-ai sync --global --only antigravity` adds your home MCP specs to `~/.gemini/config/mcp_config.json`, using the same fields as project output. A disabled server keeps `disabled: true`.

`agnostic-ai import --global antigravity` reads this file, maps `serverUrl` back to `url`, and keeps extra fields under `x-antigravity`. See [global MCP ownership and name conflicts](@/docs/global-configuration.md#global-mcp-servers).

## Import

`agnostic-ai import antigravity` reads `.agents/rules`, `.agents/skills/` (with bundled files), and `.agents/AGENTS.md`. It falls back to `.agent/rules`, `.agent/skills/`, and `.agent/AGENTS.md` only when the preferred path is absent. It also looks for `<dir>/.agents/rules/` copies to rebuild scoped rules.

- **Triggers**: `trigger` becomes `alwaysApply` (`always_on` to `true`, `glob`/`model_decision`/`manual` to `false`). It is also kept under `x-antigravity.trigger`, which `sync` reads first, so a `manual` rule with a `description` stays `manual`. An unknown `trigger` gets no `alwaysApply` and a warning, and `sync` writes it as `manual`, never `always_on`.
- **Globs**: `globs` is kept, and the singular `glob:` is merged into it.
- **Agents and MCP**: nested profiles win over flat files. `serverUrl` becomes `url`, and other fields are kept under `x-antigravity`.

## Protected paths

Not enforced. Antigravity takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).
