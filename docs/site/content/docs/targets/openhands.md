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

[OpenHands](https://docs.openhands.dev/overview/skills) gets `AGENTS.md`, agents and skills under `.agents/`, hooks, and a setup script. MCP servers are added in OpenHands itself.

## First rule

1. Add `openhands` to `targets` in `agnostic-ai.yaml`.
2. Follow [First rule](@/docs/getting-started.md#first-rule).
3. Sync writes the commit-prefix rule to `AGENTS.md`.

Have existing files? Read [what `import openhands` reads](@/docs/targets/openhands-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Shared instructions and always-on rules | `AGENTS.md` (pointer body plus inlined always-on rules) |
| Agents | `.agents/agents/<name>.md` (shared with Goose) |
| Skills and path-triggered rules | `.agents/skills/<name>/SKILL.md` (shared tree) |
| Hooks | `.openhands/hooks.json`, when hook entries exist |
| Environment `install` | `.openhands/setup.sh`, when an environment spec sets it |
| MCP servers | No file. Add them in Agent Canvas under **Customize > MCP Servers** ([MCP](@/docs/targets/openhands-reference.md#mcp)) |

Overrides, field mappings, and import: [native reference](@/docs/targets/openhands-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Find the commit-prefix rule in `AGENTS.md`.
3. In OpenHands, ask for a commit subject for a small fix. It should start with `fix:`.

## Loading and limits

- Hooks load only from the working folder, so a root `.openhands/hooks.json` does not apply in a subfolder ([hooks](@/docs/targets/openhands-reference.md#hooks)).
- A generic `tools` list and a portable `color` are dropped with a sync note. Use `x-openhands.tools` and `x-openhands.color` ([agents](@/docs/targets/openhands-reference.md#agents)).
- `disable-model-invocation` is dropped with a note, and commands are skipped ([skills](@/docs/targets/openhands-reference.md#skills)).

{% <details summary="More limits"> %}
- A project or home MCP spec gets one sync note pointing to Agent Canvas. An agent's `mcpServers` still reach its `mcp_servers`.
- A Claude matcher such as `Bash` matches nothing, because OpenHands calls that tool `terminal`.
- Agents share Goose's folder and format.
- A scoped rule becomes a skill that OpenHands loads only when a matching file is touched ([path-triggered rules](@/docs/targets/openhands-reference.md#path-triggered-rules)).
- `terminals` gets a sync note, because the setup script runs once when the repo starts. With several environment specs, the last `install` wins ([environments](@/docs/targets/openhands-reference.md#environments)).
- `outputs.openhands.mcp-file` no longer has an effect. The next sync removes a `config.toml` an earlier release wrote ([legacy files](@/docs/targets/openhands-reference.md#mcp)).
{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
