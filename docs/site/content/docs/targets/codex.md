+++
title = "Codex"
description = "Share project instructions and skills with Codex, then check the native files it loads."
weight = 20

[extra]
group = "Reference"
target_id = "codex"

[extra.moved]
config-keys = "@/docs/targets/codex-reference.md"
codex-config = "@/docs/targets/codex-reference.md"
keys-codex-ignores-in-a-project-config = "@/docs/targets/codex-reference.md"
codex-exec-policies = "@/docs/targets/codex-reference.md"
translate-bash-permissions = "@/docs/targets/codex-reference.md"
import = "@/docs/targets/codex-reference.md"
set-up-this-tool = "@/docs/targets/codex.md#first-rule"
+++

# Codex (`codex`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Codex config {#codex-config .legacy-anchor}

## Keys codex ignores in a project config {#keys-codex-ignores-in-a-project-config .legacy-anchor}

## Codex exec policies {#codex-exec-policies .legacy-anchor}

## Translate bash permissions {#translate-bash-permissions .legacy-anchor}

## Import {#import .legacy-anchor}


Write instructions and rules once under `.agnostic-ai/`. Sync writes `AGENTS.md`, skill folders, and any agents, hooks, MCP servers, or settings. Select the Claude Code target to use the same sources there. For Codex through Composer, follow the [Composer setup guide](@/docs/composer.md).

## First rule

1. Follow the [official setup guide](https://learn.chatgpt.com/docs/codex/cli).
2. Select `codex` in `agnostic-ai.yaml`.
3. Follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes a section in root `AGENTS.md`. With Claude Code also selected, it becomes `.claude/rules/conventional-commits.md` too.

Write shared instructions in `.agnostic-ai/AGNOSTIC_AI.md`. Sync merges them with the Codex rules in `AGENTS.md`. Already configured? Read [import coverage](@/docs/targets/codex-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Instructions and project-wide rules | `AGENTS.md` |
| Directory-scoped rules | `<scope>/AGENTS.md` |
| Skills and bundled files | `.agents/skills/<name>/SKILL.md` |
| Commands | `.agents/skills/<name>/SKILL.md`, as [skills the user runs by name](@/docs/spec-format/commands.md#tools-without-commands) |
| Agents | `.codex/agents/<name>.toml` |
| Hooks | `.codex/hooks.json`, when configured |
| Settings and MCP servers | `.codex/config.toml`, when configured |
| App environment | `.codex/environments/environment.toml`, when configured |
| Exec policies | `.codex/rules/default.rules`, opt-in |

An empty kind writes no files. Prefer skills for prompt procedures: project custom-prompt output is deprecated and Codex does not discover it. With `outputs.codex.commands-dir` set, commands go there instead of becoming skills. See [output overrides](@/docs/targets/codex-reference.md#config-keys).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check root `AGENTS.md` for the commit-prefix rule.
3. Start Codex in the project root and ask for a commit subject for a small fix. It should use `fix:`.

For a scoped rule, start a new session inside its directory. `agnostic-ai explain --file services/api/handler.go --target codex` lists planned instruction files and discovery limits. It does not confirm what a running session loaded.

## Loading and limits

- Codex [reads instructions from the project root down to the working directory](https://learn.chatgpt.com/docs/agent-configuration/agents-md). Start inside a subtree to load its nested `AGENTS.md`. Start a new session after changing instructions.
- Command and MCP-tool hooks need trust through `/hooks`, again after each configuration change. Project hooks also need a trusted project.
- Agents inherit the session sandbox. Codex v0.155.0 and later ignore an agent's `sandbox_mode`, so `readonly: true` is not enforced. Use session limits and [exec policies](@/docs/targets/codex-reference.md#codex-exec-policies).

{% <details summary="Trust checks and project config"> %}
- `agnostic-ai doctor -t codex` checks saved trust, not whether a running session bypasses it.
- Codex [project config wins over user config in a trusted project](https://learn.chatgpt.com/docs/config-file/config-basic).
- Keep user-level keys in the [home configuration](@/docs/global-configuration.md).
- Supported project fields and overlay precedence: [Codex native reference](@/docs/targets/codex-reference.md#codex-config).
- Editing a nested `AGENTS.md` from a root-started session does not load it.
{% </details> %}

## Protected paths

A settings spec's `protected` block writes an `apply_patch` hook and `.codex/hooks/agnostic-ai-protect.sh`. It checks added, edited, deleted, and moved paths in a patch.

- `decision: ask` blocks and tells the agent to ask you. The hook cannot open an approval prompt.
- The guard needs trusted hooks, `sh`, and `awk`.
- It does not see shell commands that write files.
- On Windows without `sh`, the hook cannot start and the edit goes through.

See [protected paths](@/docs/spec-format/settings.md#protected-paths).

Next, [add a reusable skill](@/docs/spec-format/skills.md) or compare [Claude Code](@/docs/targets/claude.md).
