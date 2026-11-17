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

Block [Goose](https://goose-docs.ai) reads the root `AGENTS.md` and a `.goosehints` file. Goose is opt-in. See [Selecting targets](@/docs/configuration.md#targets).

## First rule

Select `goose` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `AGENTS.md`.

Project MCP specs have no output for Goose. Configure servers in Goose itself.

For existing files, check [what `import goose` reads](@/docs/targets/goose-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Instructions and rules | `AGENTS.md` (entry-point pointer body plus inlined rules; shared path) |
| Agents | `.agents/agents/<name>.md` (shared with OpenHands) |
| Skills | `.agents/skills/<name>/SKILL.md` (shared tree) |
| Hooks | `.agents/plugins/agnostic-ai/plugin.json` and `.agents/plugins/agnostic-ai/hooks/hooks.json`; the manifest is written whenever a component lands in the plugin |
| Plugin skills | `.agents/plugins/agnostic-ai/skills/<name>/SKILL.md`, only with `skills-dir` pointed at the plugin |
| Concatenated rules | `.goosehints`, opt-in, only when `rules-file` is set |
| Reviews | `.agents/REVIEW.md`, and `<scope>/.agents/REVIEW.md` for directory-specific instructions |

{% <details summary="Output settings"> %}

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.goose.agents-dir` | `.agents/agents` | |
| `outputs.goose.rules-file` | unset | opt-in, writes all rules into one `.goosehints` file |
| `outputs.goose.skills-dir` | `.agents/skills` | point it at `.agents/plugins/<name>/skills` to bundle skills as a plugin; the manifest follows |
| `outputs.goose.hooks-file` | `.agents/plugins/agnostic-ai/hooks/hooks.json` | overrides must keep the `<plugin>/hooks/hooks.json` suffix |
| `outputs.goose.review-file` | `.agents/REVIEW.md` | relative to each scope |

{% </details> %}

Field mappings are in the [Goose native reference](@/docs/targets/goose-reference.md#native-mappings).

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Launch `goose` from the project and ask for a commit subject for a small fix; it should use `fix:`.

For hooks, [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests them before a session, using Goose's event data, shell, and timeout.

## Loading and limits

- Goose reads `.goosehints` only with the `Developer` extension enabled.
- Goose reads context files (`CONTEXT_FILE_NAMES`, default `AGENTS.md` and `.goosehints`) in the subdirectories it works in. Scoped rules go to a nested `AGENTS.md`.
- Goose documents no field that keeps a skill user-only, so `disable-model-invocation` is dropped with a note and commands are skipped.
- Project hooks load as an Open Plugins package; `sync --global` writes a separate package that loads for every project.

{% <details summary="More limits"> %}

- With `rules-file` set, scoped rules go to `backend/.goosehints` for a `backend/` scope. Rules with the same scope share one file.
- Agents: a generic `tools` list is dropped with a sync note, because Goose does not document it. OpenHands reads the same path and fields, so the file is written once; if `x-goose` fields make it differ from what another tool gets, set a different `outputs.goose.agents-dir`.
- Skills in the shared folder are written once for every `.agents/skills/` tool, including Crush. Shared rendering drops Crush invocation fields; separate its `skills-dir` to retain them.
- Global sync ignores `outputs.goose.hooks-file` and an absolute `GOOSE_PATH_ROOT`, for hooks and agents alike, and writes under `~`.
- MCP specs are skipped with a warning.
- Sync keeps an existing `plugin.json` it did not write and prints a note. Manifests recorded by an earlier sync remain managed. A skills directory inside a plugin folder but outside its `skills/` folder is not a Goose component and gets no manifest.

{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md) or see the [Goose native reference](@/docs/targets/goose-reference.md).
