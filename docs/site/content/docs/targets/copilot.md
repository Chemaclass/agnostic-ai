+++
title = "GitHub Copilot"
description = "GitHub Copilot: generated files, supported features, and output settings."
weight = 50

[extra]
group = "Reference"
target_id = "copilot"

[extra.moved]
set-up-this-tool = "@/docs/targets/copilot.md#first-rule"
native-mappings = "@/docs/targets/copilot-reference.md"
instructions = "@/docs/targets/copilot-reference.md"
rules = "@/docs/targets/copilot-reference.md"
agents = "@/docs/targets/copilot-reference.md"
skills = "@/docs/targets/copilot-reference.md"
old-copies = "@/docs/targets/copilot-reference.md"
chat-modes = "@/docs/targets/copilot-reference.md"
settings = "@/docs/targets/copilot-reference.md"
mcp = "@/docs/targets/copilot-reference.md"
hooks = "@/docs/targets/copilot-reference.md"
config-keys = "@/docs/targets/copilot-reference.md"
import = "@/docs/targets/copilot-reference.md"
+++

# GitHub Copilot (`copilot`)

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

## Set up this tool {#set-up-this-tool .legacy-anchor}

GitHub Copilot reads most project configuration from `.github/`, and VS Code reads MCP servers from `.vscode/mcp.json`.

## First rule

Follow the [official setup guide](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot), then open your project in the tool.

Select `copilot` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `.github/instructions/conventional-commits.instructions.md`.

VS Code, Copilot CLI, and the cloud agent read different settings and MCP files. The [native reference](@/docs/targets/copilot-reference.md) names the surface for each output.

For existing files, check [what `import copilot` reads](@/docs/targets/copilot-reference.md#import) before syncing.

## Output

```
.github/copilot-instructions.md                            # entry-point file with the AGNOSTIC_AI.md body
.github/instructions/<name>.instructions.md                # scoped rule per file
.github/agents/<name>.agent.md                             # one custom-agent profile per agent
.github/skills/<name>/SKILL.md                             # one folder per skill, bundled assets included
.vscode/mcp.json                                           # when MCP entries exist; VS Code's file (deprecated in 1.140), servers key
.github/mcp.json                                           # when MCP entries exist; Copilot CLI's file, mcpServers key
.mcp.json                                                  # only with outputs.copilot.root-mcp-file; mcpServers key
.github/hooks/agnostic-ai.json                              # when hook entries exist
.github/hooks/scripts/agnostic-ai-portable-hook.sh          # when a portable before-tool hook exists
.github/copilot/settings.json                              # when a Settings model or a disabled MCP server exists
.vscode/settings.json                                      # chat.useAgentsMdFile: false, when another target writes AGENTS.md
```

For field mappings, output overrides, and import coverage, see the [GitHub Copilot native reference](@/docs/targets/copilot-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `.github/instructions/conventional-commits.instructions.md` for the commit-prefix rule. Open the project in VS Code with Copilot. Ask for a commit subject for a small fix; it should use `fix:`.

## Protected paths

Not enforced. Copilot takes deny and ask rules only from device settings managed by your organization, not from a repository file, so sync prints a note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

Next, [add a skill](@/docs/spec-format/skills.md).
