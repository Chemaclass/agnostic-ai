+++
title = "OpenCode"
description = "OpenCode: generated files, supported features, and output settings."
weight = 130

[extra]
group = "Reference"
target_id = "opencode"

[extra.moved]
set-up-this-tool = "@/docs/targets/opencode.md#first-rule"
native-mappings = "@/docs/targets/opencode-reference.md"
rules = "@/docs/targets/opencode-reference.md"
agents = "@/docs/targets/opencode-reference.md"
skills = "@/docs/targets/opencode-reference.md"
commands = "@/docs/targets/opencode-reference.md"
v1-hooks = "@/docs/targets/opencode-reference.md"
mcp = "@/docs/targets/opencode-reference.md"
settings = "@/docs/targets/opencode-reference.md"
permissions = "@/docs/targets/opencode-reference.md"
opencode-v2-hooks = "@/docs/targets/opencode-reference.md"
config-keys = "@/docs/targets/opencode-reference.md"
import = "@/docs/targets/opencode-reference.md"
+++

# OpenCode (`opencode`)

## Native mappings {#native-mappings .legacy-anchor}

## OpenCode V2 hooks {#opencode-v2-hooks .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

## Set up this tool {#set-up-this-tool .legacy-anchor}

OpenCode reads the root `AGENTS.md`, `.opencode/`, and `opencode.json`.

## First rule

Follow the [official setup guide](https://opencode.ai/docs/), then open your project in the tool.

Select `opencode` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `AGENTS.md`.

Generated hooks use the V1 plugin API by default. Select [V2 hooks](@/docs/targets/opencode-reference.md#opencode-v2-hooks) explicitly for OpenCode V2.

For existing files, check [what `import opencode` reads](@/docs/targets/opencode-reference.md#import) before syncing.

## Output

```
AGENTS.md                                 # entry-point pointer body + rules (written by sync)
.opencode/agents/<name>.md                # one subagent per agent
.opencode/skills/<name>/SKILL.md          # one folder per skill, bundled assets included
.opencode/commands/<name>.md              # one per command spec
.opencode/commands/skill-<name>.md        # extra command form, only when emit-skills-as-commands: true
.opencode/plugins/<name>.ts               # one plugin module per hook spec
opencode.json                             # when MCP entries exist (merged with user config)
```

For field mappings, output overrides, and import coverage, see the [OpenCode native reference](@/docs/targets/opencode-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Launch `opencode` from the project and ask for a commit subject for a small fix; it should use `fix:`.

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
