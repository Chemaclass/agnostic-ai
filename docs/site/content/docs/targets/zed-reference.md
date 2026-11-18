+++
title = "Zed native reference"
description = "Native field mappings, config keys, global instructions, and import coverage for zed."
weight = 111

[extra]
nav_title = "Zed reference"
group = "Reference"
+++

# Zed native reference

Start with the [Zed setup](@/docs/targets/zed.md).

## Native mappings

### Rules

Zed 1.4.2 retired its rules library. Sync writes the instructions that point to source specs, plus a marked `## Rules` block, to `.rules`.

Zed reads [the first matching file](https://zed.dev/docs/ai/instructions) in this order:

1. `.rules`
2. `.cursorrules`
3. `.windsurfrules`
4. `.clinerules`
5. `.github/copilot-instructions.md`
6. `AGENT.md`
7. `AGENTS.md`
8. `CLAUDE.md`
9. `GEMINI.md`

- Nothing agnostic-ai writes ranks above `.rules`.
- `AGENTS.md` ranks below Copilot's file that only points to source specs. With copilot enabled, rules in `AGENTS.md` never reach Zed.
- Zed calls `AGENTS.md` primary and `.rules` a compatibility file. If Zed drops `.rules`, the rules move back to `AGENTS.md`.
- For older Zed, `outputs.zed.rules-file: .rules` replaces the pointer instructions with the legacy single document, agent bodies included.

### Skills

[Zed skills](https://zed.dev/docs/ai/skills). Identical files are written once. Sync reports `x-zed` overrides that make two copies differ.

- `disable-model-invocation: true` is written to `SKILL.md`. Zed hides the skill from the agent's catalog, and it still runs as a slash command or @-mention ([docs](https://zed.dev/docs/ai/skills#disable-model-invocation)).
- Commands become skills you run by name ([tools without commands](@/docs/spec-format/commands.md#tools-without-commands)).
- When another target also writes the skills folder, files must match. The field is dropped with a note and commands are skipped.
- An `x-zed.disable-model-invocation` value wins.
- A separate `outputs.zed.skills-dir` keeps output apart during sync but adds no native search path. [Zed loads skills](https://zed.dev/docs/ai/skills#limitations) only as direct children of `~/.agents/skills/` or `<worktree>/.agents/skills/`.
- To use another backing folder, link it from a documented root. Sync does not create the link.
- Names are 1-64 lowercase letters or digits, with single hyphens between segments. `Deploy`, `my_skill`, and `my--skill` fail sync with the format. Sync does not rename them.

### Agents

Zed has no per-agent support. Sync reports this unless `outputs.zed.rules-file` is set (the single document includes agent sections).

### MCP

Servers go under `context_servers`, not `mcpServers`. Stdio uses flat `command`/`args`/`env`. Remote (HTTP or SSE) uses `url`/`headers`. Your other keys (theme, `buffer_font_size`) stay.

- `disabled: true` becomes `enabled: false`. Enabled servers get no key, because Zed defaults to true. See [`disabled` support by target](@/docs/spec-format/mcps-reference.md#disabled-support-by-target).
- Set these through `x-zed`; no other tool has them:
  - `timeout`: both transports, seconds per tool call, default 60.
  - `oauth`: HTTP.
  - `remote`: stdio and extension servers.

<details>
<summary>Source for the MCP keys</summary>

[Zed's MCP docs](https://zed.dev/docs/ai/mcp) do not name the `enabled` toggle. Zed's settings source, [`crates/settings_content/src/project.rs`](https://github.com/zed-industries/zed/blob/main/crates/settings_content/src/project.rs), defines it, plus `timeout`, `oauth`, and `remote`.

</details>

### Hooks

With `outputs.zed.tasks-file` set, hooks become [Zed Tasks](https://zed.dev/docs/tasks) that run `sh -c "<hook command>"`.

- `WorktreeCreate` writes `hooks: ["create_worktree"]`, so the task runs after Zed creates a linked worktree. Import restores it.
- Since Zed 1.28.1, that task is skipped in [restricted mode](https://zed.dev/docs/worktree-trust). Trust the project first.
- Other tasks import as `OnDemand` (command palette).
- Sync controls `label`, `command`, `args`, and `hooks`. Adding `create_worktree` keeps other hook names.
- `x-zed` sets other fields: `cwd`, `env`, `shell`, `reveal`, `hide`, `save`, `allow_concurrent_runs`, `use_new_terminal`, `tags`, `reevaluate_context`.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.zed.skills-dir` | `.agents/skills` | |
| `outputs.zed.agents` | unset (agents are dropped) | `skill` writes each agent as an on-demand skill. Sync keeps it off with a note while another tool with subagents writes the same skills folder; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills) |
| `outputs.zed.mcp-file` | `.zed/settings.json` | |
| `outputs.zed.tasks-file` | empty | opt-in |
| `outputs.zed.rules-file` | unset | writes the legacy single document and skips the pointer instructions, so pointing it at `.rules` replaces the pointer file instead of clashing with it |

## Global instructions

`agnostic-ai sync --global --only zed` writes home instructions and rules to Zed's [personal instruction file](https://zed.dev/docs/ai/instructions). Each platform's path is in the [global output table](@/docs/global-configuration-reference.md#global-output). Skills still use `~/.agents/skills/`.

| Platform | File |
|---|---|
| macOS | `~/.config/zed/AGENTS.md` (ignores `XDG_CONFIG_HOME`) |
| Linux and FreeBSD | `$FLATPAK_XDG_CONFIG_HOME/zed/AGENTS.md` when set, else `$XDG_CONFIG_HOME/zed/AGENTS.md`, else `~/.config/zed/AGENTS.md` |
| Windows | `%APPDATA%\Zed\AGENTS.md` |

These roots follow [Zed's native path selection](https://github.com/zed-industries/zed/blob/ba63a022c9a22ca66d68ef2a8f0bb9b5b0a9984d/crates/paths/src/paths.rs). An empty or relative Flatpak root is rejected, so nothing is written under the working directory.

After a path correction:

- The first sync clears the managed block from the old XDG file, only when the ledger records it.
- Personal text and private permissions remain. A link to the active file is kept.
- Old files the ledger has not recorded stay unchanged.
- `explain --global rules/<name>.md` shows the current native file.
- Global instruction import is not supported. Project `import zed` keeps its paths below.

## Import

`agnostic-ai import zed` reads:

- `.rules`.
- `.zed/tasks.json`, as hook specs.
- MCP servers from `.zed/settings.json`.
- `.agents/skills/<name>/SKILL.md`, copied with all bundled files.

For MCP servers:

- `enabled: false` imports as `disabled: true`.
- `timeout`, `oauth`, and `remote` move under `x-zed`.
- Removing a server from `.zed/settings.json` drops it on the next import.
- A server whose `oauth` holds a client secret is left out with a warning, so no secret reaches a spec.
