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


Write project instructions and rules under `.agnostic-ai/`. Sync gives Codex `AGENTS.md`, native skill folders, and any configured agents, hooks, MCP servers, or settings. The same sources can also feed Claude Code.

## First rule

Install the client using the [official setup guide](https://learn.chatgpt.com/docs/codex/cli), then open your project in it.

Select `codex` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). The shared commit-prefix rule becomes a section in root `AGENTS.md`. With Claude Code also selected, the same source becomes `.claude/rules/conventional-commits.md`.

Write shared project instructions in `.agnostic-ai/AGNOSTIC_AI.md`. Sync combines them with the Codex rules in `AGENTS.md`. Edit those sources for later changes.

Already configured? Check [import coverage](@/docs/targets/codex-reference.md#import) before syncing. Import reads root and nested instruction files, agents, skill folders, hooks, MCP servers, environments, and remaining project config into an overlay.

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

An empty kind creates no corresponding files. Prefer skills for prompt procedures: project custom-prompt output is a deprecated compatibility option that Codex does not discover. With `outputs.codex.commands-dir` set, commands go there instead of becoming skills. See [output overrides](@/docs/targets/codex-reference.md#config-keys).

## Loading and limits

- Codex [discovers instructions from the project root to the session's working directory](https://learn.chatgpt.com/docs/agent-configuration/agents-md). Start inside a subtree to load its nested `AGENTS.md`; editing there from a root-started session does not load it. Start a new session after changing instructions.
- Command and MCP-tool hooks require trust through `/hooks`, including after their configuration changes. Project hooks also need a trusted project. `agnostic-ai doctor -t codex` checks saved trust, not whether an already-running session bypasses it.
- Agents inherit the session sandbox. Codex v0.155.0 and later ignore an agent's `sandbox_mode`, so `readonly: true` does not enforce read-only behavior there. Use session limits and [exec policies](@/docs/targets/codex-reference.md#codex-exec-policies).

Codex [project config takes precedence over user config in a trusted project](https://learn.chatgpt.com/docs/config-file/config-basic). Keep user-level keys in the [home configuration](@/docs/global-configuration.md), and use the [Codex native reference](@/docs/targets/codex-reference.md#codex-config) for supported project fields and overlay precedence.

## Protected paths

A settings spec's `protected` block creates an `apply_patch` hook and `.codex/hooks/agnostic-ai-protect.sh`. It checks added, edited, deleted, and moved paths in a patch. `decision: ask` blocks and tells the agent to ask you; the hook cannot open an approval prompt.

The guard needs trusted hooks, `sh`, and `awk`. It does not see shell commands that write files. On Windows, if `sh` is missing, the hook cannot start and the edit goes through. See [protected paths](@/docs/spec-format/settings.md#protected-paths).

## Verify

Run `agnostic-ai sync --check` and inspect root `AGENTS.md` for the commit-prefix rule. Start Codex from the project root and ask for a commit subject for a small fix; it should use `fix:`.

For a scoped rule, start a new session inside its directory. `agnostic-ai explain --file services/api/handler.go --target codex` reports planned instruction files and discovery limits; it does not confirm the files loaded in a running session.

Next, [add a reusable skill](@/docs/spec-format/skills.md) or compare the same setup in [Claude Code](@/docs/targets/claude.md).
