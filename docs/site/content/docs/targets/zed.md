+++
title = "Zed"
description = "How agnostic-ai emits Zed configuration: native paths, capability limits, and output options."
weight = 110

[extra]
group = "Reference"
target_id = "zed"
+++

# Zed (`zed`)

## Output

```
.rules                                 # canonical entry-point pointer body + inlined rules (written by sync)
.agents/skills/<name>/SKILL.md         # one folder per skill (shared tree with codex/amp/crush)
.zed/settings.json                     # when MCP entries exist (merged with existing user config)
.zed/tasks.json                        # one task per hook, only when tasks-file is set
```

- **Rules**: Zed 1.4.2 retired its rules library, so project instructions live in an instruction file. `sync` writes the pointer body plus the sentinel-marked `## Rules` block to the root `.rules`. Zed reads [the first matching file](https://zed.dev/docs/ai/instructions) from `.rules`, `.cursorrules`, `.windsurfrules`, `.clinerules`, `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md` and stops there.
  - `.rules` ranks first, so nothing agnostic-ai emits can shadow it. `AGENTS.md` ranks behind Copilot's entry point, which carries only the pointer body, so rules written there would never reach Zed with copilot enabled.
  - Zed calls `AGENTS.md` its primary file and `.rules` a compatibility one; if Zed drops `.rules`, this output moves back. For older Zed versions, set `outputs.zed.rules-file: .rules` to replace the pointer body with the legacy merged document (which also carries agent bodies).
- **Skills**: native [Zed skills](https://zed.dev/docs/ai/skills) folders at `.agents/skills/<name>/SKILL.md`, the path codex, amp, and crush share. Identical rendered bytes dedupe into one write; divergent `x-zed` overrides surface through the collision check.
  - To hide a skill from the agent's catalog (slash command or @-mention only), set `x-zed: {disable-model-invocation: true}`. The renderer merges only `x-zed` keys, so a plain top-level `disable-model-invocation:` (the form Cursor promotes) has no effect here.
  - Skill names must be 1-64 lowercase letters or digits, with single hyphens between segments. Invalid names such as `Deploy`, `my_skill`, and `my--skill` fail sync with the name and required format; they are not renamed.
- **Agents**: no per-agent surface in current Zed. Sync prints a coverage note unless `outputs.zed.rules-file` is set (the merged document carries agent sections).
- **MCP**: written to `.zed/settings.json` under `context_servers` (not `mcpServers`). Stdio servers use flat `command`/`args`/`env`; remote (HTTP / SSE) servers use `url`/`headers`.
  - `disabled: true` emits as Zed's `enabled: false`; enabled servers get no key, since Zed defaults to true. The toggle is read from the file. [Zed's MCP docs](https://zed.dev/docs/ai/mcp) do not name it; the evidence is Zed's settings source, [`crates/settings_content/src/project.rs`](https://github.com/zed-industries/zed/blob/main/crates/settings_content/src/project.rs).
  - The same source defines `timeout` (both transports; seconds per tool call, default 60), `oauth` (HTTP), and `remote` (stdio and extension servers). Set these through `x-zed`, since no other target has a matching field.
  - User keys (theme, buffer_font_size) are kept. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).
- **Hooks**: when `outputs.zed.tasks-file` is set, hook specs emit as [Zed Tasks](https://zed.dev/docs/tasks) running `sh -c "<hook command>"`.
  - `WorktreeCreate` writes `hooks: ["create_worktree"]`, so Zed runs the task after creating a linked worktree; import restores that event. Tasks without that hook import as `OnDemand` and run from the command palette.
  - The adapter manages `label`, `command`, `args`, and `hooks` (so `WorktreeCreate` can add `create_worktree` without dropping other hook names). Other Zed Task fields pass through under `x-zed`, such as `cwd`, `env`, `shell`, `reveal`, `hide`, `save`, `allow_concurrent_runs`, `use_new_terminal`, `tags`, and `reevaluate_context`.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.zed.skills-dir` | `.agents/skills` | |
| `outputs.zed.mcp-file` | `.zed/settings.json` | |
| `outputs.zed.tasks-file` | empty | opt-in |
| `outputs.zed.rules-file` | unset | writes the legacy merged document and skips the pointer-body write, so pointing it at `.rules` replaces the entry-point rather than colliding with it |

## Import

`agnostic-ai import zed` reads `.rules`, `.zed/tasks.json` (into hook specs, see **Hooks**), and MCP servers from `.zed/settings.json`. It copies `.agents/skills/<name>/SKILL.md` with all bundled assets.

## Verify

1. Install Zed from [zed.dev](https://zed.dev).
2. Check the tree: `ls .rules .agents/skills/ .zed/settings.json .zed/tasks.json`, `test -f .agents/skills/*/SKILL.md`, `python -m json.tool .zed/settings.json > /dev/null`, `python -m json.tool .zed/tasks.json > /dev/null`.
3. Open the project. The agent panel reads `.rules` as project instructions and lists each `.agents/skills/<name>/` as a skill (`@skill` / slash command).
4. The MCP picker shows each `context_servers.<name>` ready.
5. The command palette runs every entry from `.zed/tasks.json` as a Zed Task.
