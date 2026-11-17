+++
title = "Continue"
description = "Continue: generated files, supported features, and output settings."
weight = 90

[extra]
group = "Reference"
target_id = "continue"

[extra.moved]
set-up-this-tool = "@/docs/targets/continue.md#first-rule"
native-mappings = "@/docs/targets/continue-reference.md"
config-keys = "@/docs/targets/continue.md#output"
import = "@/docs/targets/continue-reference.md"
+++

# Continue (`continue`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

Continue is [no longer actively maintained](https://github.com/continuedev/continue#what-is-continue). agnostic-ai still writes its rules, skills, and MCP servers under `.continue/`, plus optional assistant files.

## First rule

Follow the [official setup guide](https://marketplace.visualstudio.com/items?itemName=Continue.continue), then open your project in the tool.

Select `continue` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `.continue/rules/conventional-commits.md`.

Agents become rule content by default. Optional assistant files are standalone configs, not automatically discovered subagents.

For existing files, check [what `import continue` reads](@/docs/targets/continue-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Rules (and agents as rule content) | `.continue/rules/<name>.md` |
| Skills and bundled assets | `.continue/skills/<name>/SKILL.md` |
| MCP servers | `.continue/mcpServers/<name>.yaml`, one per entry |
| Agents as assistants | `.continue/assistants/<name>.yaml`, only when `assistants-dir` is set |

{% <details summary="Output settings"> %}

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.continue.rules-dir` | `.continue/rules` | One `.md` per rule and agent. |
| `outputs.continue.skills-dir` | `.continue/skills` | |
| `outputs.continue.mcp-dir` | `.continue/mcpServers` | |
| `outputs.continue.assistants-dir` | empty | opt-in |

{% </details> %}

Field mappings are in the [Continue native reference](@/docs/targets/continue-reference.md#native-mappings).

## Verify

Run `agnostic-ai sync --check` and inspect `.continue/rules/conventional-commits.md` for the commit-prefix rule. Open the project in Continue. Find the rule in the rules picker, then ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- Continue offers every skill to the model, so none can be kept user-only. `disable-model-invocation` is dropped with a note, and commands are skipped.
- With the `claude` target also selected, each skill shows up twice, because Continue reads `.continue/skills/` and `.claude/skills/` and does not merge names across them.
- A scoped rule matches its scope directory plus its `globs` or `paths`, and gets no `alwaysApply`. See [file-pattern limits for directory rules](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- MCP files must be Continue blocks, and no file is written, with a sync note, for `type: ws` or for an entry missing `command` (stdio) or `url` (remote).

{% <details summary="More limits"> %}

- A rule with no directory scope keeps `globs`, `alwaysApply`, and `description`. A comma-separated `globs` string becomes an array, because Continue reads a string as one pattern. `x-continue.regex` works only without `scope`.
- Remote MCP servers drop `env` (stdio only) with a sync note.
- Assistant files are standalone `config.yaml` files with no models or rules, so your defaults apply. Continue's docs do not describe a workspace folder for them, so local profile loading rests on the source only. The rule form (`.continue/rules/agent-<name>.md`) is written either way.
- Sync removes the `.continue/rules/skill-<name>.md` files it wrote in older versions. Import still reads a `skill-<name>.md` rule as a skill.

{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md) or see the [Continue native reference](@/docs/targets/continue-reference.md).
