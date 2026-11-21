+++
title = "Kiro"
description = "Kiro: generated files, supported features, and output settings."
weight = 160

[extra]
group = "Reference"
target_id = "kiro"

[extra.moved]
set-up-this-tool = "@/docs/targets/kiro.md#first-rule"
native-mappings = "@/docs/targets/kiro-reference.md"
commands = "@/docs/targets/kiro-reference.md"
config-keys = "@/docs/targets/kiro-reference.md"
import = "@/docs/targets/kiro-reference.md"
global-voice-language = "@/docs/targets/kiro-reference.md"
+++

# Kiro (`kiro`)

## Native mappings {#native-mappings .legacy-anchor}

## Commands {#commands .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

## User-level voice language {#global-voice-language .legacy-anchor}

## Set up this tool {#set-up-this-tool .legacy-anchor}

agnostic-ai writes AWS Kiro steering files, native skills, agents, commands, and hooks under `.kiro/`, plus MCP servers and `.kiroignore`.

## First rule

Follow the [official setup guide](https://kiro.dev), then open your project in the tool.

Select `kiro` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `.kiro/steering/conventional-commits.md`, or shared `AGENTS.md` when another selected target already puts the rule there.

File-matching steering rules need the IDE or CLI V3. Earlier CLI versions load only always-on rules.

For existing files, check [what `import kiro` reads](@/docs/targets/kiro-reference.md#import) before syncing.

## Output

```
AGENTS.md                          # entry-point pointer body, plus the rules block when another tool adds it
.kiro/steering/<name>.md           # one per rule (inclusion: always, or fileMatch + fileMatchPattern from globs)
.kiro/skills/<name>/SKILL.md       # one per skill (+ bundled scripts/, references/, assets/)
.kiro/agents/<name>.md             # one per agent
.kiro/prompts/<name>.md            # one per command (slash commands in CLI V3)
.kiro/hooks/<name>.json            # one per hook
.kiro/settings/mcp.json            # when MCP entries exist
.kiro/settings/cli.json            # chat.defaultModel, when a settings spec sets model
.kiroignore                        # when ignore entries exist
```

For field mappings, output overrides, and import coverage, see the [Kiro native reference](@/docs/targets/kiro-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Find the commit-prefix rule in `.kiro/steering/conventional-commits.md` or the shared `AGENTS.md`.
3. Open the project in Kiro. Trust the workspace if asked (CLI 2.33 shows a trust screen before it loads workspace configuration).
4. Confirm the steering rule or shared instructions loaded, then ask for a commit subject for a small fix. It should use `fix:`.

## Protected paths

Not enforced. Sync prints a note for a `protected` block; Kiro has no edit guard it writes. User-level voice language uses the separate [home settings mapping](@/docs/targets/kiro-reference.md#global-voice-language). See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
