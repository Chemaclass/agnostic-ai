+++
title = "Goose"
description = "Goose: generated files, supported features, and output settings."
weight = 240

[extra]
group = "Reference"
target_id = "goose"

[extra.moved]
native-mappings = "@/docs/targets/goose-reference.md"
import = "@/docs/targets/goose-reference.md"
set-up-this-tool = "@/docs/targets/goose.md#first-rule"
config-keys = "@/docs/targets/goose.md#output"
+++

# Goose (`goose`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

[Goose](https://goose-docs.ai) reads the root `AGENTS.md` and a `.goosehints` file. It is opt-in: see [Selecting targets](@/docs/configuration.md#targets).

## First rule

1. Select `goose` in `agnostic-ai.yaml`.
2. Follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. It lands in `AGENTS.md`.

Already have Goose files? See [what `import goose` reads](@/docs/targets/goose-reference.md#import) first. Project MCP specs have no Goose output: configure servers in Goose.

## Output

| Source | Default output |
|---|---|
| Instructions and rules | `AGENTS.md` (pointer body plus inlined rules; shared path) |
| Agents | `.agents/agents/<name>.md` (shared with OpenHands) |
| Skills | `.agents/skills/<name>/SKILL.md` (shared tree) |
| Hooks | `.agents/plugins/agnostic-ai/plugin.json` and `.agents/plugins/agnostic-ai/hooks/hooks.json` |
| Plugin skills | `.agents/plugins/agnostic-ai/skills/<name>/SKILL.md`, only with `skills-dir` pointed at the plugin |
| Concatenated rules | `.goosehints`, only when `rules-file` is set |

The plugin manifest is written whenever a component lands in the plugin.

{% <details summary="Output settings"> %}

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.goose.agents-dir` | `.agents/agents` | |
| `outputs.goose.rules-file` | unset | opt-in; writes all rules into one `.goosehints` |
| `outputs.goose.skills-dir` | `.agents/skills` | point at `.agents/plugins/<name>/skills` to bundle skills as a plugin; the manifest follows |
| `outputs.goose.hooks-file` | `.agents/plugins/agnostic-ai/hooks/hooks.json` | an override must keep the `<plugin>/hooks/hooks.json` suffix |
| `outputs.goose.review-file` | no output | accepted for compatibility; has no effect |

{% </details> %}

Field mappings: [Goose native reference](@/docs/targets/goose-reference.md#native-mappings).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `AGENTS.md` for the commit-prefix rule.
3. Launch `goose` and ask for a commit subject for a small fix. It should use `fix:`.

To test hooks before a session, run [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run). It uses Goose's event data, shell, and timeout.

## Loading and limits

- Goose reads `.goosehints` only with the `Developer` extension enabled.
- Goose reads context files in the subdirectories it works in, so scoped rules go to a nested `AGENTS.md`. See [scoped rules](@/docs/targets/goose-reference.md#native-mappings).
- Project hooks load as an Open Plugins package. `sync --global` writes a separate package for every project.
- Review specs are unsupported after Goose 1.60 removed `goose review`. See [legacy review guidance](@/docs/targets/goose-reference.md#reviews).

{% <details summary="More limits"> %}
- No skill can be kept user-only, so `disable-model-invocation` is dropped with a note and commands are skipped.
- A generic agent `tools` list is dropped with a note, because Goose does not document it.
- Skills in the shared folder are written once for every `.agents/skills/` tool, Crush included. Shared rendering drops Crush invocation fields; give Crush its own `skills-dir` to keep them.
- Global sync ignores `outputs.goose.hooks-file` and an absolute `GOOSE_PATH_ROOT`, for hooks and agents, and writes under `~`.
- MCP specs are skipped with a warning.
- An existing `plugin.json` that sync did not write is kept, with a note. Manifests from an earlier sync stay managed.
{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md) or see the [Goose native reference](@/docs/targets/goose-reference.md).
