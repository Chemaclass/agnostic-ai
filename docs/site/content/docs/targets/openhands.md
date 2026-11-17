+++
title = "OpenHands"
description = "OpenHands: generated files, supported features, and output settings."
weight = 200

[extra]
group = "Reference"
target_id = "openhands"

[extra.moved]
set-up-this-tool = "@/docs/targets/openhands.md#first-rule"
native-mappings = "@/docs/targets/openhands-reference.md"
skills = "@/docs/targets/openhands-reference.md"
agents = "@/docs/targets/openhands-reference.md"
rules = "@/docs/targets/openhands-reference.md"
path-triggered-rules = "@/docs/targets/openhands-reference.md"
hooks = "@/docs/targets/openhands-reference.md"
mcp = "@/docs/targets/openhands-reference.md"
environments = "@/docs/targets/openhands-reference.md"
config-keys = "@/docs/targets/openhands-reference.md"
import = "@/docs/targets/openhands-reference.md"
+++

# OpenHands (`openhands`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## Skills {#skills .legacy-anchor}

## Agents {#agents .legacy-anchor}

## Rules {#rules .legacy-anchor}

## Path-triggered rules {#path-triggered-rules .legacy-anchor}

## Hooks {#hooks .legacy-anchor}

## MCP {#mcp .legacy-anchor}

## Environments {#environments .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

[OpenHands](https://docs.openhands.dev/overview/skills) gets `AGENTS.md`, agents and skills under `.agents/`, hooks, and a setup script. Add MCP servers in OpenHands itself.

## First rule

Select `openhands` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `AGENTS.md`.

MCP servers have no output. Add them in Agent Canvas, as [MCP](@/docs/targets/openhands-reference.md#mcp) describes. For existing files, check [what `import openhands` reads](@/docs/targets/openhands-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Shared instructions and always-on rules | `AGENTS.md` (pointer body plus inlined always-on rules; shared path) |
| Agents | `.agents/agents/<name>.md` (shared with Goose) |
| Skills and path-triggered rules | `.agents/skills/<name>/SKILL.md` (shared tree) |
| Hooks | `.openhands/hooks.json`, when hook entries exist |
| Environment `install` | `.openhands/setup.sh`, when an environment spec sets it |

For output overrides, field mappings, and import coverage, see the [native reference](@/docs/targets/openhands-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Open the project in OpenHands and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- MCP servers are not written to any file. Add them in Agent Canvas under **Customize > MCP Servers**; a project or home MCP spec gets one sync note pointing there. An agent's `mcpServers` still reach its `mcp_servers`.
- Hooks load only from the working folder, so a root `.openhands/hooks.json` does not apply when OpenHands works in a subfolder. A Claude matcher such as `Bash` matches nothing, because OpenHands calls that tool `terminal`.
- Agents share Goose's folder and format. A generic `tools` list and a portable `color` are dropped with a sync note; set `x-openhands.tools` or `x-openhands.color`. See [agents](@/docs/targets/openhands-reference.md#agents).
- OpenHands has no way to keep a skill user-only, so `disable-model-invocation` is dropped with a note, and commands are skipped.

{% <details summary="More limits"> %}
- A scoped rule becomes a path-triggered skill that OpenHands loads only when a matching file is touched; see [path-triggered rules](@/docs/targets/openhands-reference.md#path-triggered-rules).
- `terminals` gets a sync note, because the setup script runs once when the repo starts. With several environment specs, the last `install` wins. See [environments](@/docs/targets/openhands-reference.md#environments).
- `outputs.openhands.mcp-file` no longer has an effect, and the next sync removes a `config.toml` an earlier release wrote. See [legacy files](@/docs/targets/openhands-reference.md#mcp).
{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
