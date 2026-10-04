+++
title = "Zed"
description = "How agnostic-ai emits Zed configuration: native paths, capability limits, and output options."
weight = 110

[extra]
group = "Reference"
target_id = "zed"
+++

# Zed (`zed`)

Zed reads `.rules`, `.agents/skills/`, `.zed/settings.json` MCP servers, and optional tasks.

Set `outputs.zed.agents: skill` to write agents as on-demand skills; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills).

## Output

```
.rules                                 # canonical entry-point pointer body + inlined rules (written by sync)
.agents/skills/<name>/SKILL.md         # one folder per skill (shared tree)
.zed/settings.json                     # when MCP entries exist (merged with existing user config)
.zed/tasks.json                        # one task per hook, only when tasks-file is set
```

- **Rules**: Zed 1.4.2 retired its rules library, so `sync` writes the pointer body and sentinel-marked `## Rules` block to `.rules`.
  - Zed reads [the first matching file](https://zed.dev/docs/ai/instructions) of `.rules`, `.cursorrules`, `.windsurfrules`, `.clinerules`, `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`. Nothing agnostic-ai emits outranks `.rules`. `AGENTS.md` ranks behind Copilot's pointer-only entry point. With copilot enabled, rules there never reach Zed.
  - Zed calls `AGENTS.md` primary and `.rules` a compatibility file. If Zed drops `.rules`, this output moves back.
  - For older Zed, `outputs.zed.rules-file: .rules` replaces the pointer body with the legacy merged document (agent bodies included).
- **Skills**: [Zed skills](https://zed.dev/docs/ai/skills). Identical files are written once. The collision check reports divergent `x-zed` overrides.
  - `x-zed: {disable-model-invocation: true}` hides a skill from the agent's catalog (slash command or @-mention only). The renderer merges only `x-zed` keys. A top-level `disable-model-invocation:` (Cursor's form) does nothing here.
  - Names must be 1-64 lowercase letters or digits, with single hyphens between segments. Sync fails on names such as `Deploy`, `my_skill`, and `my--skill` and states the format. It never renames them.
- **Agents**: Zed has no per-agent surface. Sync prints a coverage note unless `outputs.zed.rules-file` is set (the merged document carries agent sections).
- **MCP**: `context_servers` (not `mcpServers`): flat `command`/`args`/`env` for stdio, `url`/`headers` for remote (HTTP / SSE). User keys (theme, buffer_font_size) stay.
  - `disabled: true` writes `enabled: false`. Enabled servers get no key, because Zed defaults to true. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
  - Set `timeout` (both transports; seconds per tool call, default 60), `oauth` (HTTP), and `remote` (stdio and extension servers) through `x-zed`. No other target has them.
- **Hooks**: with `outputs.zed.tasks-file` set, hooks become [Zed Tasks](https://zed.dev/docs/tasks) running `sh -c "<hook command>"`.
  - `WorktreeCreate` writes `hooks: ["create_worktree"]`, so the task runs after Zed creates a linked worktree. Import restores it. Other tasks import as `OnDemand` (command palette).
  - The adapter manages `label`, `command`, `args`, and `hooks` (adding `create_worktree` keeps other hook names). `x-zed` passes through other fields, such as `cwd`, `env`, `shell`, `reveal`, `hide`, `save`, `allow_concurrent_runs`, `use_new_terminal`, `tags`, and `reevaluate_context`.

{% <details summary="Source for the MCP keys"> %}
[Zed's MCP docs](https://zed.dev/docs/ai/mcp) do not name the `enabled` toggle Zed reads. Its settings source, [`crates/settings_content/src/project.rs`](https://github.com/zed-industries/zed/blob/main/crates/settings_content/src/project.rs), defines it, plus `timeout`, `oauth`, and `remote`.
{% </details> %}

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.zed.skills-dir` | `.agents/skills` | |
| `outputs.zed.mcp-file` | `.zed/settings.json` | |
| `outputs.zed.tasks-file` | empty | opt-in |
| `outputs.zed.rules-file` | unset | writes the legacy merged document and skips the pointer-body write, so pointing it at `.rules` replaces the entry-point rather than colliding with it |

## Import

`agnostic-ai import zed` reads `.rules`, `.zed/tasks.json` (as hook specs), and MCP servers from `.zed/settings.json`. It copies `.agents/skills/<name>/SKILL.md` with all bundled assets.

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Zed from [zed.dev](https://zed.dev).
2. Check the tree: `ls .rules .agents/skills/ .zed/settings.json .zed/tasks.json`, `ls .agents/skills/*/SKILL.md >/dev/null`, `python -m json.tool .zed/settings.json > /dev/null`, `python -m json.tool .zed/tasks.json > /dev/null`.
3. Open the project. The agent panel reads `.rules` as project instructions and lists each `.agents/skills/<name>/` as a skill (`@skill` / slash command).
4. The MCP picker shows each `context_servers.<name>` ready.
5. The command palette runs every entry from `.zed/tasks.json` as a Zed Task.
