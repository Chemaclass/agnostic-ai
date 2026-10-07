+++
title = "Kilo"
description = "What agnostic-ai writes for Kilo: file paths, what Kilo cannot do, and output options."
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
- `color` is not checked. Kilo accepts hex (`#FF5733`) or a theme token such as `primary`, `accent`, or `error` ([custom subagents](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/custom-subagents.md)), so `color: blue` may not show as you expect. See [`color` support by target](@/docs/spec-format/agents.md#color-support-by-target).
- `mode` takes OpenCode's `primary`, `subagent`, or `all`.
- `disable`, `hidden`, `steps`, `temperature`, and `top_p` ([agent options](https://kilo.ai/docs/customize/custom-subagents)) need `x-kilo`, for example `x-kilo: {temperature: 0.1, steps: 15}`.
- Kilo has no `tools:` key, so a `tools` list becomes a [`permission`](https://kilo.ai/docs/customize/agent-permissions) map: `tools: [Read, Grep]` writes `permission: {"*": deny, read: allow, grep: allow}`. The catch-all comes first because the last matching rule wins.
- These names translate: `Read`, `Glob`, `Grep`, `Edit`, `Write`, `Bash`, `WebFetch`, `WebSearch`, `Task`, `Skill`, `TodoRead`, `TodoWrite`, and `mcp__<server>__<tool>` as `{server}_{tool}` ([permission keys](https://kilo.ai/docs/getting-started/settings/auto-approving-actions), [tool groups](https://kilo.ai/docs/automate/tools)). Other names drop with a coverage note. If nothing translates, no map is written, because `{"*": deny}` alone would lock the agent out. `x-kilo: {permission: {...}}` wins outright.

A scoped `edit` deny also sets Kilo's `write` deny for that path, so an unscoped `write` allow cannot bypass it.

**Skills** go to the shared `.agents/skills/<name>/SKILL.md` tree. Kilo loads it by default beside its own `.kilo/skills/`. Codex, amp, zed, crush, openhands, windsurf, and augment share it. Kilo also scans `.claude/skills/` (in the VS Code extension, only with Claude Code Compatibility on). Any other `outputs.kilo.skills-dir` is added to `skills.paths` in `kilo.jsonc` ([skills](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/skills.md)). Your `skills.paths` entries and `skills.urls` stay.

**Rules**:

- Unscoped rules go into the root `AGENTS.md`, which Kilo [always loads](https://kilo.ai/docs/customize/agents-md) ("cannot be individually disabled"). A rule already in that block gets no `.kilo/rules/` file or `instructions` entry, so it loads once.
- A rule whose text differs for Kilo, such as one with a path variable, keeps its file.
- Scoped rules use nested `AGENTS.md`. See [target behavior](@/docs/target-behavior.md#entry-point-files).
- Kilo's [order](https://kilo.ai/docs/customize/agents-md) is agent prompt, then project `instructions`, then `AGENTS.md`, then global. Kilo still loads the legacy `.kilocode/rules/`, which this adapter never writes.

{% <details summary="No root rules block"> %}
Kilo reads no rules block when a `file` output override moves the entry point off the root `AGENTS.md`, or when `AGENTS.md` is in `sync.unmanaged`. (`sync.unmanaged` also stops sync writing `AGENTS.md` for codex and every other reader.) Unscoped rules then get one file each under `.kilo/rules/`, listed by explicit path (not a glob) in the [`instructions`](https://kilo.ai/docs/customize/custom-rules) array of `kilo.jsonc`. When no rule file is left, sync removes only the `instructions` entries for rules that `AGENTS.md` now carries. Your own entries stay.
{% </details> %}

**Commands** use Kilo's slash-command path ([workflows](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/workflows.md)). As with agents, the name comes from the filename. Frontmatter carries `description`, `agent`, `model`, `variant` (a reasoning-effort override such as `low` or `high`), and `subtask` when set, close to OpenCode's list. Kilo v7.6.0 reserves `goal` for commands and MCP prompts ([goals](https://kilo.ai/docs/code-with-ai/agents/goals)). A command spec named `goal` is still written, with a note to rename it.

**MCP** servers merge into the `mcp` map of `kilo.jsonc` (not the deprecated `mcpServers`):

- Stdio sets `type: "local"`, joins `command` and `args` into one `command` array, and uses `environment` for env vars.
- Remote sets `type: "remote"` with `url` and `headers`. `oauth: false` turns off automatic OAuth.
- `disabled: true` writes `"enabled": false`. An enabled server gets no key.
- Both keep `timeout` (milliseconds, zero included). `x-kilo` can override `timeout` and `oauth`.
- `sync` keeps your keys in `kilo.jsonc`. It reads JSONC, as Kilo documents, but drops comments and trailing commas with a warning.

**Hooks**: Kilo auto-registers each plugin module at startup ([plugins](https://kilo.ai/docs/automate/extending/plugins)). Its plugins work like [OpenCode](@/docs/targets/opencode.md)'s, and one renderer serves both. `PreToolUse` and `PostToolUse` map to `tool.execute.before` and `tool.execute.after`. Other documented events use the `event` hook with an `event.type` guard. Matchers, exit code 2 blocking, and `disabled: true` behave the same. Only the module shape differs: `export default { id: "<name>", server }`. Hooks on `shell.env`, `experimental.session.compacting`, or an unmapped event drop with a coverage note.

**Settings**: a settings spec's default `model` and its `x-kilo` block (for Kilo-only keys such as `sandbox`) merge into the top level of `kilo.jsonc`. An `x-kilo` `permission` instead merges tool by tool with the translated rules.

The portable `allow`, `deny`, and `ask` lists merge into the `permission` key of `kilo.jsonc` ([auto-approving actions](https://kilo.ai/docs/getting-started/settings/auto-approving-actions)):

- Each rule becomes one glob under one tool key, matched against the tool's arguments. `Bash(npm run:*)` becomes `bash: {"npm run *": "allow"}`, `Bash(rm -rf /)` becomes `bash: {"rm -rf /": "deny"}`, `Read(docs/*)` becomes `read: {"docs/*": "allow"}`, a bare `Bash` becomes `bash: {"*": "allow"}`, and `mcp__github__list_issues` becomes `github_list_issues`.
- A rule in two lists takes the stricter action.
- An `Edit` deny or ask also lands under `write`. Claude Code's `Edit` rules cover every tool that edits files, and Kilo keeps `write` apart. So `Edit(.env)` in `deny` blocks writing `.env` too. An `Edit` allow stays under `edit`.
- Inside one tool, patterns go allow, then ask, then deny. Kilo takes the last match, and Claude Code lets a deny or ask win over any allow, so a narrower allow never bypasses a broader deny.
- `WebFetch(domain:example.test)` drops with a coverage note, because Kilo matches URLs. Unknown tool names drop too.
- `x-kilo.permission` writes Kilo's own map. It wins for the tool keys it names, and that spec's portable lists are skipped.
- Your entries for tools agnostic-ai does not set stay.

This adapter never writes `.kilo/kilo.jsonc`. A hand-written one merges over the root `kilo.jsonc` ([config order](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/getting-started/settings/index.md#config-file-precedence)), so its `mcp` or `instructions` shadow the generated ones.

**Ignore** specs write `.kilocodeignore` in the project root. Kilo's [migrator](https://kilo.ai/docs/customize/context/kilocodeignore), not this adapter, turns it into read and edit denials.

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.kilo.rules-dir` | `.kilo/rules` | |
| `outputs.kilo.agents-dir` | `.kilo/agents` | |
| `outputs.kilo.skills-dir` | `.agents/skills` | |
| `outputs.kilo.commands-dir` | `.kilo/commands` | |
| `outputs.kilo.hooks-dir` | `.kilo/plugin` | Kilo only loads plugins from `plugin/` or `plugins/`, so moving this stops the hooks loading |
| `outputs.kilo.mcp-file` | `kilo.jsonc` | |
| `outputs.kilo.ignore-file` | `.kilocodeignore` | |

With `builtins: [memory]`, sync adds both [shared memory](@/docs/memory.md) indexes to `instructions`. While rules go into `AGENTS.md` (the default), your own entries stay, and dropping the built-in removes only those two. When sync lists rule files in `instructions`, it owns the whole list.

With `memory.personal: repo`, the personal store lies outside the project root, and Kilo [ignores project-declared files there](https://github.com/Kilo-Org/kilocode/blob/v7.8.8/packages/opencode/src/session/instruction.ts). Sync writes only the shared index and prints a note. Add the store's `MEMORY.md` to `instructions` in `~/.config/kilo/kilo.jsonc` to load it. See [repo mode](@/docs/memory.md#one-store-per-repository).

## Import

`agnostic-ai import kilo` reads `.kilocodeignore` and two fields from the root `kilo.jsonc`: the default `model` (into `settings/kilo.yaml`) and portable entries in the `permission` map (into `settings/permissions-kilo.yaml`). Keys with no portable spelling (`external_directory`, `lsp`, `doom_loop`, namespaced MCP keys) stay in the file.

Generated rule sections in `AGENTS.md` are recovered when their source specs are missing. Hand-written instructions, native rule directories, agents, skills, commands, MCP servers, and plugin hooks are not imported.

## Protected paths

Advisory. Sync writes no edit guard for this tool, so it prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

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
