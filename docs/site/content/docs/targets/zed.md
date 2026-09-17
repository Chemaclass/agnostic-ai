+++
title = "Zed"
description = "How agnostic-ai emits Zed configuration: native paths, capability limits, and output options."
weight = 110

[extra]
group = "Reference"
target_id = "zed"
+++

# Zed (`zed`)

```
.rules                                 # canonical entry-point pointer body + inlined rules (written by sync)
.agents/skills/<name>/SKILL.md         # one folder per skill (shared tree with codex/amp/crush)
.zed/settings.json                     # when MCP entries exist (merged with existing user config)
.zed/tasks.json                        # one task per hook, only when tasks-file is set
```

- **Rules**: Zed 1.4.2 retired its rules library, so always-on project instructions are now an instruction file. `sync` writes the pointer body plus the sentinel-marked `## Rules` block to the root `.rules`. Zed reads [the first matching file](https://zed.dev/docs/ai/instructions) from `.rules`, `.cursorrules`, `.windsurfrules`, `.clinerules`, `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md` and stops there.
  - `AGENTS.md` is rank 7, behind Copilot's pointer-only entry-point at rank 5, so writing the rules to `AGENTS.md` left Zed with none of them whenever copilot was enabled too (target-audit 2026-08-27, #624). `.rules` is rank 1 and nothing agnostic-ai emits can outrank it.
  - Zed still calls `AGENTS.md` its primary instruction file and `.rules` a compatibility one, so a Zed release dropping `.rules` moves this back. Set `outputs.zed.rules-file: .rules` to replace the pointer body with the legacy merged document (which also carries agent bodies) for older Zed versions.
- **Skills**: native [Zed skills](https://zed.dev/docs/ai/skills) folders at `.agents/skills/<name>/SKILL.md`, the cross-tool path codex, amp, and crush emit too. Identical rendered bytes dedupe into one write; divergent `x-zed` overrides surface through the collision check. Zed also documents `disable-model-invocation` ("Set to true to hide from the agent's catalog (invocable via slash command or @-mention only)"): it reaches the emitted file via `x-zed: {disable-model-invocation: true}`, since this renderer merges only `x-zed` keys. A plain top-level `disable-model-invocation:` field, the form Cursor promotes to a native key, has no effect here.
- **Agents**: no per-agent surface in current Zed; sync prints a coverage note unless `outputs.zed.rules-file` is set (the merged document carries agent sections).
- **MCP**: written into `.zed/settings.json` under `context_servers` (Zed's key, not `mcpServers`). Stdio servers use a flat `command`/`args`/`env` shape; remote (HTTP / SSE) servers use a native `url`/`headers` shape. A spec's `disabled: true` emits as Zed's own `enabled: false`; an enabled server gets no key, since Zed defaults it to true.
  - That toggle is file-backed, not UI-only. The evidence is Zed's Rust settings struct rather than its docs: every variant of `ContextServerSettingsContent` in [`crates/settings_content/src/project.rs`](https://github.com/zed-industries/zed/blob/main/crates/settings_content/src/project.rs) carries `/// Whether the context server is enabled.` over `#[serde(default = "default_true")] enabled: bool`, and `context_servers` is a field of `ProjectSettingsContent`, the struct `.zed/settings.json` deserializes into (target-audit 2026-08-27, #641). [zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp) names none of it; its only `enabled`-family key is `enable_all_context_servers`, an agent-profile key rather than a per-server one, which is why earlier audits missed this.
  - The same source carries three more per-server fields no doc page names: `timeout` on either transport, `oauth` on an HTTP server, and `remote` on stdio and extension servers. Those reach the file through `x-zed`, since no other target documents a same-named field with the same meaning. `timeout` is on both transports because the `Stdio` variant `#[serde(flatten)]`s `ContextServerCommand`, whose own field reads "Timeout for tool calls in seconds. Defaults to 60 if not specified" (target-audit 2026-09-11, #737).
  - User-managed keys (theme, buffer_font_size) are preserved. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).
- **Hooks**: when `outputs.zed.tasks-file` is set, hook specs emit as [Zed Tasks](https://zed.dev/docs/tasks) using `sh -c "<hook command>"`. `WorktreeCreate` writes `hooks: ["create_worktree"]`, so Zed runs the task after creating a linked worktree; import restores that event. Tasks without that hook import as `OnDemand` and run from the command palette. Any other documented Zed Task field passes through under `x-zed`.

Zed skill names must contain 1-64 lowercase letters or digits, with single hyphens between segments. Invalid names fail sync with the name and required format; they are not renamed. `import zed` copies `.agents/skills/<name>/SKILL.md` and bundled assets.

Config keys:

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.zed.skills-dir` | `.agents/skills` | |
| `outputs.zed.mcp-file` | `.zed/settings.json` | |
| `outputs.zed.tasks-file` | empty | opt-in |
| `outputs.zed.rules-file` | unset | writes the legacy merged document and skips the pointer-body write, so pointing it at `.rules` replaces the entry-point rather than colliding with it |

Verify with the real editor:

1. Install Zed from [zed.dev](https://zed.dev).
2. Check the tree: `ls .rules .agents/skills/ .zed/settings.json .zed/tasks.json`, `test -f .agents/skills/*/SKILL.md`, `python -m json.tool .zed/settings.json > /dev/null`, `python -m json.tool .zed/tasks.json > /dev/null`.
3. Open the project. The agent panel reads `.rules` as project instructions and lists each `.agents/skills/<name>/` as a skill (`@skill` / slash command).
4. The MCP picker shows each `context_servers.<name>` ready.
5. The command palette runs every entry from `.zed/tasks.json` as a Zed Task.
