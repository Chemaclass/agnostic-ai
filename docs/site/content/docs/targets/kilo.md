+++
title = "Kilo"
description = "How agnostic-ai emits Kilo configuration: native paths, capability limits, and output options."
weight = 220

[extra]
group = "Reference"
target_id = "kilo"
+++

# Kilo (`kilo`)

## Output

```
AGENTS.md                          # entry-point pointer body + inlined rules (shared path)
.kilo/rules/<name>.md              # one per rule
.kilo/agents/<name>.md             # one per agent
.agents/skills/<name>/SKILL.md     # one folder per skill, plus bundled assets (shared cross-tool tree)
.kilo/commands/<name>.md           # one per command
.kilo/plugin/<name>.ts             # one per hook, auto-registered at startup
kilo.jsonc                         # instructions, mcp, and permission maps (merged with existing user config)
.kilocodeignore                    # compatibility input for Kilo's permission migrator
```

Kilo [Code](https://kilo.ai/docs) reads the root `AGENTS.md` and loads agents from `.kilo/agents/<name>.md`.

**Agents**:

- Kilo takes the agent name from the filename, so `name:` is never written. Frontmatter carries `description` (falls back to the spec name) plus `color`, `mode`, and `model` when set.
- `color` passes through without validation. Kilo accepts hex (`#FF5733`) or a theme token such as `primary`, `accent`, or `error` ([custom subagents](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/custom-subagents.md)), so `color: blue` may not render as intended. See [`color` support by target](@/docs/spec-format.md#color-support-by-target).
- `mode` uses OpenCode's `primary`/`subagent`/`all` values.
- `disable`, `hidden`, `steps`, `temperature`, and `top_p` ([agent options](https://kilo.ai/docs/customize/custom-subagents)) are reachable only through `x-kilo`, e.g. `x-kilo: {temperature: 0.1, steps: 15}`.
- Kilo has no `tools:` key. A spec's `tools` list becomes Kilo's [`permission`](https://kilo.ai/docs/customize/agent-permissions) map instead: `tools: [Read, Grep]` emits `permission: {"*": deny, read: allow, grep: allow}`. The catch-all sorts first because the last matching rule wins.
- Translated names: `Read`, `Glob`, `Grep`, `Edit`, `Write`, `Bash`, `WebFetch`, `WebSearch`, `Task`, `Skill`, `TodoRead`, `TodoWrite`, and `mcp__<server>__<tool>` as `{server}_{tool}` ([permission keys](https://kilo.ai/docs/getting-started/settings/auto-approving-actions), [tool groups](https://kilo.ai/docs/automate/tools)). Other names drop with a coverage note. If nothing translates, no map is written, since `{"*": deny}` alone would lock the agent out. `x-kilo: {permission: {...}}` wins outright.

**Skills** emit into the shared `.agents/skills/<name>/SKILL.md` tree, which Kilo loads by default alongside its own `.kilo/skills/`. The render matches codex, amp, zed, crush, openhands, windsurf, and augment, so the tree dedupes. Kilo also scans `.claude/skills/` (the VS Code extension only with Claude Code Compatibility enabled). Point `outputs.kilo.skills-dir` anywhere else and the directory is added to `kilo.jsonc`'s `skills.paths` ([skills](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/skills.md)). Paths you added to `skills.paths` are kept, and `skills.urls` is left alone.

**Rules**: unscoped rules emit one file per rule under `.kilo/rules/`, each listed by explicit path (not a glob) in `kilo.jsonc`'s [`instructions`](https://kilo.ai/docs/customize/custom-rules) array. Scoped rules use nested `AGENTS.md` and stay out of that list.

Kilo's [precedence](https://kilo.ai/docs/customize/agents-md) is agent prompt > project `instructions` > `AGENTS.md` > global. `AGENTS.md` always loads when present, so full rule bodies also inline there as a fallback. The legacy `.kilocode/rules/` tree is still auto-included by Kilo, but this adapter never writes it.

**Commands** emit one file per command at `.kilo/commands/<name>.md`, Kilo's slash-command path ([workflows](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/workflows.md)). The filename sets the command name, so `name` is never written. Frontmatter carries `description`, `agent`, `model`, `variant`, and `subtask` when set, close to OpenCode's list. `variant` is a reasoning-effort override (e.g. `low` or `high`).

Kilo v7.6.0 reserves the name `goal` for commands and MCP prompts ([goals](https://kilo.ai/docs/code-with-ai/agents/goals)). A command spec named `goal` still emits, with a coverage note telling you to rename it. MCP prompt names come from the server at runtime, so agnostic-ai output cannot collide there.

**MCP** servers merge into the `mcp` map of `kilo.jsonc` (not the deprecated `mcpServers`):

- Stdio sets `type: "local"`, joins `command` and `args` into one `command` array, and uses `environment` for env vars.
- Remote sets `type: "remote"` with `url`/`headers`. `oauth: false` disables automatic OAuth.
- `disabled: true` writes `"enabled": false`; an enabled server gets no key.
- Both transports keep `timeout` in milliseconds, including zero. `x-kilo` can override `timeout` and `oauth`.

User-managed keys in `kilo.jsonc` survive every sync. Kilo documents comments in this file, so `sync` accepts JSONC: keys survive, but comments and trailing commas are dropped, with a warning.

**Hooks** emit one plugin module per hook spec at `.kilo/plugin/<name>.ts`, which Kilo auto-registers at startup ([plugins](https://kilo.ai/docs/automate/extending/plugins)). Kilo plugins behave like OpenCode's, so the mapping matches [OpenCode](@/docs/targets/opencode.md): `PreToolUse` and `PostToolUse` map to `tool.execute.before` and `tool.execute.after`, and other documented events use the `event` hook with an `event.type` guard. The module shape differs: Kilo needs `export default { id: "<name>", server }`. Hooks on `shell.env`, `experimental.session.compacting`, or an unmapped event drop with a coverage note. Matchers, command execution, exit code 2 blocking, and `disabled: true` behave as on OpenCode, since both share one renderer. `.kilo/plugin/` is not imported.

**Settings**: a settings spec's default `model` merges into the top level of `kilo.jsonc`. So does an `x-kilo` block on that spec, which is how a Kilo-only key such as `sandbox` reaches the file; `permission` instead merges tool by tool with the translated rules.

The portable `allow`, `deny`, and `ask` lists merge into `kilo.jsonc`'s `permission` key ([auto-approving actions](https://kilo.ai/docs/getting-started/settings/auto-approving-actions)):

- Each rule becomes one glob pattern under one tool key, matched against the tool's arguments. `Bash(npm run:*)` becomes `bash: {"npm run *": "allow"}`, `Bash(rm -rf /)` becomes `bash: {"rm -rf /": "deny"}`, `Read(docs/*)` becomes `read: {"docs/*": "allow"}`, a bare `Bash` becomes `bash: {"*": "allow"}`, and `mcp__github__list_issues` becomes `github_list_issues`.
- Keys sort alphabetically, so `*` comes before every exception, the order Kilo asks for since the last match wins.
- A rule in two lists resolves to the stricter action.
- `WebFetch(domain:example.test)` drops with a coverage note, because Kilo matches URLs. Unknown tool names drop too. Set `x-kilo.permission` on the settings spec to write Kilo's own map; it wins for the tool keys it names, and that spec's portable lists are skipped.
- Your own entries for tools agnostic-ai does not set survive the merge.
- `import kilo` reads the map back into `settings/permissions-kilo.yaml`. Keys with no portable spelling (`external_directory`, `lsp`, `doom_loop`, namespaced MCP keys) stay in the file.

Kilo also reads `.kilo/kilo.jsonc`, which this adapter does not write. It sits above the root `kilo.jsonc` in Kilo's [config precedence](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/getting-started/settings/index.md#config-file-precedence) and merges over it. A hand-written `.kilo/kilo.jsonc` that sets `mcp` or `instructions` shadows this adapter's output for those keys.

Ignore specs write project-root `.kilocodeignore`. Kilo's [migrator](https://kilo.ai/docs/customize/context/kilocodeignore) converts it into read/edit permission denials; this adapter does not translate ignore patterns into permission maps itself. The shared hand-authored-file protection applies.

`import kilo` imports `.kilocodeignore`, the default model (into `settings/kilo.yaml`), and the `permission` map. Other Kilo configuration is not imported.

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

## Verify

1. Install Kilo Code ([docs](https://kilo.ai/docs)).
2. Check the tree: `ls AGENTS.md .kilo/rules/ .kilo/agents/ .agents/skills/ .kilo/commands/ .kilo/plugin/ kilo.jsonc`, and `grep "Generated by agnostic-ai" .kilo/rules/*.md .kilo/agents/*.md .kilo/commands/*.md .kilo/plugin/*.ts` for the provenance header.
3. Open the project and confirm:
   - Each `.kilo/rules/<name>.md` in the `instructions` array appears in the loaded rules.
   - Each `.kilo/agents/<name>.md` appears in the agent picker.
   - Each `.agents/skills/<name>/` folder loads as a skill.
   - Each `.kilo/commands/<name>.md` runs as `/<name>`.
   - Each `.kilo/plugin/<name>.ts` loads at startup, and its matcher or event runs the command with no "schema mismatch" in the log.
   - Each `mcp.<name>` connects, and a disabled spec shows as disabled.
