+++
title = "Kilo"
description = "How agnostic-ai emits Kilo configuration: native paths, capability limits, and output options."
weight = 220

[extra]
group = "Reference"
target_id = "kilo"
+++

# Kilo (`kilo`)

Kilo [Code](https://kilo.ai/docs) gets `AGENTS.md`, agents, commands, plugin hooks, shared skills, and a merged `kilo.jsonc`.

## Output

```
AGENTS.md                          # entry-point pointer body + inlined rules (shared path)
.kilo/rules/<name>.md              # one per rule the root AGENTS.md does not carry
.kilo/agents/<name>.md             # one per agent
.agents/skills/<name>/SKILL.md     # one folder per skill, plus bundled assets (shared cross-tool tree)
.kilo/commands/<name>.md           # one per command
.kilo/plugin/<name>.ts             # one per hook, auto-registered at startup
kilo.jsonc                         # instructions, mcp, and permission maps (merged with existing user config)
.kilocodeignore                    # compatibility input for Kilo's permission migrator
```

**Agents** (`.kilo/agents/<name>.md`):

- The filename sets the agent name, so `name:` is never written. Frontmatter carries `description` (falls back to the spec name) plus `color`, `mode`, and `model` when set.
- `color` is not validated. Kilo accepts hex (`#FF5733`) or a theme token such as `primary`, `accent`, or `error` ([custom subagents](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/custom-subagents.md)), so `color: blue` may not render as intended. See [`color` support by target](@/docs/spec-format/agents.md#color-support-by-target).
- `mode` takes OpenCode's `primary`/`subagent`/`all`.
- `disable`, `hidden`, `steps`, `temperature`, and `top_p` ([agent options](https://kilo.ai/docs/customize/custom-subagents)) need `x-kilo`, e.g. `x-kilo: {temperature: 0.1, steps: 15}`.
- Kilo has no `tools:` key, so a `tools` list becomes a [`permission`](https://kilo.ai/docs/customize/agent-permissions) map: `tools: [Read, Grep]` emits `permission: {"*": deny, read: allow, grep: allow}`. The catch-all sorts first because the last matching rule wins.
- Translated names: `Read`, `Glob`, `Grep`, `Edit`, `Write`, `Bash`, `WebFetch`, `WebSearch`, `Task`, `Skill`, `TodoRead`, `TodoWrite`, and `mcp__<server>__<tool>` as `{server}_{tool}` ([permission keys](https://kilo.ai/docs/getting-started/settings/auto-approving-actions), [tool groups](https://kilo.ai/docs/automate/tools)). Other names drop with a coverage note. If nothing translates, no map is written: `{"*": deny}` alone would lock the agent out. `x-kilo: {permission: {...}}` wins outright.

**Skills** go to the shared `.agents/skills/<name>/SKILL.md` tree, which Kilo loads by default beside its own `.kilo/skills/`. It dedupes with codex, amp, zed, crush, openhands, windsurf, and augment. Kilo also scans `.claude/skills/` (in the VS Code extension, only with Claude Code Compatibility enabled). Any other `outputs.kilo.skills-dir` is added to `kilo.jsonc`'s `skills.paths` ([skills](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/skills.md)); your `skills.paths` entries and `skills.urls` stay.

**Rules**:

- Unscoped rules inline into the root `AGENTS.md`, which Kilo [always loads](https://kilo.ai/docs/customize/agents-md) ("cannot be individually disabled"). A rule matching that block gets no `.kilo/rules/` file or `instructions` entry, so it loads once.
- A rule whose text differs for Kilo, such as one with a path variable (the block keeps variables as written), keeps its file.
- Scoped rules use nested `AGENTS.md`. See [target behavior](@/docs/target-behavior.md#entry-point-files).
- Kilo's [precedence](https://kilo.ai/docs/customize/agents-md) is agent prompt > project `instructions` > `AGENTS.md` > global. Kilo still auto-includes legacy `.kilocode/rules/`, which this adapter never writes.

{% <details summary="No root rules block"> %}
A `file` output override that moves the entry point off the root `AGENTS.md`, or `AGENTS.md` in `sync.unmanaged`, leaves no rules block Kilo reads. (`sync.unmanaged` also stops sync writing `AGENTS.md` for codex and every other reader.) Unscoped rules then emit one file each under `.kilo/rules/`, listed by explicit path (not a glob) in `kilo.jsonc`'s [`instructions`](https://kilo.ai/docs/customize/custom-rules) array. When no rule file is left, sync removes from `instructions` only the entries for rules `AGENTS.md` now carries; your own entries stay.
{% </details> %}

**Commands** use Kilo's slash-command path ([workflows](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/workflows.md)); as with agents, `name` comes from the filename. Frontmatter carries `description`, `agent`, `model`, `variant` (a reasoning-effort override such as `low` or `high`), and `subtask` when set, close to OpenCode's list. Kilo v7.6.0 reserves `goal` for commands and MCP prompts ([goals](https://kilo.ai/docs/code-with-ai/agents/goals)): a command spec named `goal` still emits, with a note to rename it. MCP prompt names come from the server at runtime, so they cannot collide.

**MCP** servers merge into the `mcp` map of `kilo.jsonc` (not the deprecated `mcpServers`):

- Stdio sets `type: "local"`, joins `command` and `args` into one `command` array, and uses `environment` for env vars.
- Remote sets `type: "remote"` with `url`/`headers`. `oauth: false` disables automatic OAuth.
- `disabled: true` writes `"enabled": false`; an enabled server gets no key.
- Both keep `timeout` (milliseconds, zero included). `x-kilo` can override `timeout` and `oauth`.
- `sync` keeps your keys in `kilo.jsonc`. It accepts JSONC, as Kilo documents, but drops comments and trailing commas with a warning.

**Hooks**: Kilo auto-registers each plugin module at startup ([plugins](https://kilo.ai/docs/automate/extending/plugins)). Its plugins behave like [OpenCode](@/docs/targets/opencode.md)'s, and one renderer serves both: `PreToolUse` and `PostToolUse` map to `tool.execute.before` and `tool.execute.after`, other documented events use the `event` hook with an `event.type` guard, and matchers, command execution, exit code 2 blocking, and `disabled: true` behave the same. Only the module shape differs: `export default { id: "<name>", server }`. Hooks on `shell.env`, `experimental.session.compacting`, or an unmapped event drop with a coverage note. `.kilo/plugin/` is not imported.

**Settings**: a settings spec's default `model` and its `x-kilo` block (for Kilo-only keys such as `sandbox`) merge into the top level of `kilo.jsonc`. An `x-kilo` `permission` instead merges tool by tool with the translated rules.

The portable `allow`, `deny`, and `ask` lists merge into `kilo.jsonc`'s `permission` key ([auto-approving actions](https://kilo.ai/docs/getting-started/settings/auto-approving-actions)):

- Each rule becomes one glob under one tool key, matched against the tool's arguments. `Bash(npm run:*)` becomes `bash: {"npm run *": "allow"}`, `Bash(rm -rf /)` becomes `bash: {"rm -rf /": "deny"}`, `Read(docs/*)` becomes `read: {"docs/*": "allow"}`, a bare `Bash` becomes `bash: {"*": "allow"}`, and `mcp__github__list_issues` becomes `github_list_issues`.
- Keys sort alphabetically, so `*` precedes every exception, since the last match wins.
- A rule in two lists takes the stricter action.
- `WebFetch(domain:example.test)` drops with a coverage note, because Kilo matches URLs. Unknown tool names drop too.
- `x-kilo.permission` writes Kilo's own map. It wins for the tool keys it names, and that spec's portable lists are skipped.
- Your entries for tools agnostic-ai does not set survive.

A hand-written `.kilo/kilo.jsonc`, which this adapter never writes, merges over the root `kilo.jsonc` ([config precedence](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/getting-started/settings/index.md#config-file-precedence)), so its `mcp` or `instructions` shadow the generated ones.

**Ignore** specs write project-root `.kilocodeignore`. Kilo's [migrator](https://kilo.ai/docs/customize/context/kilocodeignore), not this adapter, turns it into read/edit permission denials. The shared hand-authored-file protection applies.

`import kilo` reads only `.kilocodeignore`, the default model (into `settings/kilo.yaml`), and the `permission` map (into `settings/permissions-kilo.yaml`). Keys with no portable spelling (`external_directory`, `lsp`, `doom_loop`, namespaced MCP keys) stay in the file.

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.kilo.rules-dir` | `.kilo/rules` | |
| `outputs.kilo.agents-dir` | `.kilo/agents` | |
| `outputs.kilo.skills-dir` | `.agents/skills` | |
| `outputs.kilo.commands-dir` | `.kilo/commands` | |
| `outputs.kilo.hooks-dir` | `.kilo/plugin` | Kilo only loads plugins from `plugin/` or `plugins/`, so moving this takes the hooks out of range |
| `outputs.kilo.mcp-file` | `kilo.jsonc` | |
| `outputs.kilo.ignore-file` | `.kilocodeignore` | |

## Protected paths

Advisory. This target has no native edit guard that sync writes, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Kilo Code ([docs](https://kilo.ai/docs)).
2. Check the tree: `ls AGENTS.md .kilo/agents/ .agents/skills/ .kilo/commands/ .kilo/plugin/ kilo.jsonc`, and `grep "Generated by agnostic-ai" .kilo/agents/*.md .kilo/commands/*.md .kilo/plugin/*.ts`. `AGENTS.md` carries each unscoped rule under `## Rules`, with no copy in `.kilo/rules/`.
3. Open the project and confirm each:
   - `.kilo/rules/<name>.md` in the `instructions` array appears in the loaded rules.
   - `.kilo/agents/<name>.md` appears in the agent picker.
   - `.agents/skills/<name>/` folder loads as a skill.
   - `.kilo/commands/<name>.md` runs as `/<name>`.
   - `.kilo/plugin/<name>.ts` loads at startup and runs its command on its matcher or event, with no "schema mismatch" logged.
   - `mcp.<name>` connects, and a disabled spec shows as disabled.
