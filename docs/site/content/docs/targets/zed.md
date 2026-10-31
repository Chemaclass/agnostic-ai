+++
title = "Zed"
description = "What agnostic-ai writes for Zed: file paths, what it supports, and output settings."
weight = 110

[extra]
group = "Reference"
target_id = "zed"

[extra.moved]
set-up-this-tool = "@/docs/targets/zed.md#first-rule"
+++

# Zed (`zed`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Zed reads `.rules`, `.agents/skills/`, `.zed/settings.json` MCP servers, and optional tasks.

Set `outputs.zed.agents: skill` to write agents as on-demand skills; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills).

## First rule

Install the client using the [official setup guide](https://zed.dev), then open your project in it.

Select `zed` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `.rules`.

Hooks become optional Zed Tasks. They do not provide general before-tool enforcement.

For existing files, check [what `import zed` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `.rules` for the commit-prefix rule. Open the project in Zed and ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
.rules                                 # canonical entry-point pointer body + inlined rules (written by sync)
.agents/skills/<name>/SKILL.md         # one folder per skill (shared tree)
.zed/settings.json                     # when MCP entries exist (merged with existing user config)
.zed/tasks.json                        # one task per hook, only when tasks-file is set
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>


- **Rules**: Zed 1.4.2 retired its rules library, so `sync` writes the instructions that point to source specs and a marked `## Rules` block to `.rules`.
  - Zed reads [the first matching file](https://zed.dev/docs/ai/instructions) of `.rules`, `.cursorrules`, `.windsurfrules`, `.clinerules`, `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`. Nothing agnostic-ai writes ranks above `.rules`. `AGENTS.md` ranks below Copilot's file that only points to source specs. With copilot enabled, rules in `AGENTS.md` never reach Zed.
  - Zed calls `AGENTS.md` primary and `.rules` a compatibility file. If Zed drops `.rules`, the rules move back to `AGENTS.md`.
  - For older Zed, `outputs.zed.rules-file: .rules` replaces the instructions that point to source specs with the legacy single document (agent bodies included).
- **Skills**: [Zed skills](https://zed.dev/docs/ai/skills). Identical files are written once. Sync reports `x-zed` overrides that make two copies differ.
  - `disable-model-invocation: true` is written to `SKILL.md`. Zed then hides the skill from the agent's catalog, and it still runs as a slash command or @-mention ([Zed skills](https://zed.dev/docs/ai/skills#disable-model-invocation)). So each command reaches Zed as a skill the user runs by name ([tools without commands](@/docs/spec-format/commands.md#tools-without-commands)). When another target also writes the skills folder, the files must match, so the field is dropped with a note and commands are skipped. An `x-zed.disable-model-invocation` value wins.
  - A separate `outputs.zed.skills-dir` keeps the output apart during sync, but does not add a native search path. [Zed loads skills](https://zed.dev/docs/ai/skills#limitations) only as direct children of `~/.agents/skills/` or `<worktree>/.agents/skills/`. To use another backing folder, expose it through a symlink at a documented root; sync does not create the link.
  - Names must be 1-64 lowercase letters or digits, with single hyphens between segments. Sync fails on names such as `Deploy`, `my_skill`, and `my--skill` and states the format. It does not rename them.
- **Agents**: Zed has no per-agent support. Sync reports this unless `outputs.zed.rules-file` is set (the single document includes agent sections).
- **MCP**: servers go under `context_servers` (not `mcpServers`): flat `command`/`args`/`env` for stdio, `url`/`headers` for remote (HTTP / SSE). Your other keys (theme, buffer_font_size) stay.
  - `disabled: true` becomes `enabled: false`. Enabled servers get no key, because Zed defaults to true. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
  - Set `timeout` (both transports; seconds per tool call, default 60), `oauth` (HTTP), and `remote` (stdio and extension servers) through `x-zed`. No other tool has them.
- **Hooks**: with `outputs.zed.tasks-file` set, hooks become [Zed Tasks](https://zed.dev/docs/tasks) running `sh -c "<hook command>"`.
  - `WorktreeCreate` writes `hooks: ["create_worktree"]`, so the task runs after Zed creates a linked worktree. Import restores it. Other tasks import as `OnDemand` (command palette).
  - Sync controls `label`, `command`, `args`, and `hooks` (adding `create_worktree` keeps other hook names). `x-zed` sets other fields, such as `cwd`, `env`, `shell`, `reveal`, `hide`, `save`, `allow_concurrent_runs`, `use_new_terminal`, `tags`, and `reevaluate_context`.

<details>
<summary>Source for the MCP keys</summary>

[Zed's MCP docs](https://zed.dev/docs/ai/mcp) do not name the `enabled` toggle. Zed's settings source, [`crates/settings_content/src/project.rs`](https://github.com/zed-industries/zed/blob/main/crates/settings_content/src/project.rs), defines it, plus `timeout`, `oauth`, and `remote`.

</details>

</details>

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.zed.skills-dir` | `.agents/skills` | |
| `outputs.zed.agents` | unset (agents are dropped) | `skill` writes each agent as an on-demand skill. Sync keeps it off with a note while another tool with subagents writes the same skills folder; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills) |
| `outputs.zed.mcp-file` | `.zed/settings.json` | |
| `outputs.zed.tasks-file` | empty | opt-in |
| `outputs.zed.rules-file` | unset | writes the legacy single document and skips the write of instructions that point to source specs, so pointing it at `.rules` replaces the pointer file instead of clashing with it |

## Global instructions

`agnostic-ai sync --global --only zed` writes home instructions and rules to Zed's [personal instruction file](https://zed.dev/docs/ai/instructions). See the [global output table](@/docs/global-configuration.md#global-output) for each platform's path. Skills still use `~/.agents/skills/`.

On Windows, the first sync after this fix clears the managed block from the old XDG file only when the ledger records it. Personal text and private permissions remain. An old file not recorded by sync stays untouched. `explain --global rules/<name>.md` shows the current native file. Global instruction import is not supported; project `import zed` keeps its existing paths below.

## Import

`agnostic-ai import zed` reads `.rules`, `.zed/tasks.json` (as hook specs), and MCP servers from `.zed/settings.json`. It copies `.agents/skills/<name>/SKILL.md` with all bundled files.

## Protected paths

Not enforced. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).
