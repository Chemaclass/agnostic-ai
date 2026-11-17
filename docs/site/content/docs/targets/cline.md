+++
title = "Cline"
description = "Cline: generated files, supported features, and output settings."
weight = 70

[extra]
group = "Reference"
target_id = "cline"

[extra.moved]
set-up-this-tool = "@/docs/targets/cline.md#first-rule"
native-mappings = "@/docs/targets/cline-reference.md"
hooks = "@/docs/targets/cline-reference.md"
workflows = "@/docs/targets/cline-reference.md"
config-keys = "@/docs/targets/cline-reference.md"
global-hooks = "@/docs/targets/cline-reference.md"
import = "@/docs/targets/cline-reference.md"
vs-code-memory-startup = "@/docs/targets/cline-reference.md"
+++

# Cline (`cline`)

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Global hooks {#global-hooks .legacy-anchor}

## Import {#import .legacy-anchor}

## VS Code memory startup {#vs-code-memory-startup .legacy-anchor}

## Set up this tool {#set-up-this-tool .legacy-anchor}

Cline gets `AGENTS.md`, rules, agents, skills, and hook scripts. Workflows are opt-in.

## First rule

Follow the [official setup guide](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev), then open your project in the tool.

Select `cline` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `.clinerules/conventional-commits.md`, or shared `AGENTS.md` when another selected target already puts the rule there.

Keep `AGENTS.md` enabled in the Rules panel so Cline can read the shared copy.

For existing files, check [what `import cline` reads](@/docs/targets/cline-reference.md#import) before syncing.

## Output

```
AGENTS.md                            # pointer body, plus the rules block when another tool adds it
.clinerules/<name>.md                # one per rule that AGENTS.md does not carry, or that needs `paths`
.cline/agents/<name>.yml             # frontmatter over a Markdown system prompt
.clinerules/hooks/<Event>            # one executable script per hook event, no extension
.cline/skills/<name>/SKILL.md        # one folder per skill (Cline's recommended skills path)
.clinerules/workflows/<name>.md      # one per agent, only when workflows-dir is set
```

For field mappings, output overrides, and import coverage, see the [Cline native reference](@/docs/targets/cline-reference.md).

## Verify

Run `agnostic-ai sync --check` and find the commit-prefix rule in `.clinerules/conventional-commits.md` or the shared `AGENTS.md`. Open the project in Cline and confirm the rule is enabled in its Rules panel. Ask for a commit subject for a small fix; it should use `fix:`.

## Protected paths

Not enforced. Cline takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
