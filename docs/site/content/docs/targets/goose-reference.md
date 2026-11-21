+++
title = "Goose native reference"
description = "Native field mappings and import coverage for goose."
weight = 241

[extra]
nav_title = "Goose reference"
group = "Reference"
+++

# Goose native reference

Start with the [Goose setup](@/docs/targets/goose.md). Output keys are listed under [output settings](@/docs/targets/goose.md#output).

## Native mappings

### Rules

- Rules go in the `## Rules` block of `AGENTS.md`.
- `outputs.goose.rules-file: .goosehints` also writes all rules into one `.goosehints`. Goose reads it only with the `Developer` extension enabled ([using goosehints](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-goosehints.md)).
- Goose reads context files (`CONTEXT_FILE_NAMES`, default `AGENTS.md` and `.goosehints`) in the subdirectories it works in.
- Scoped rules go to a nested `AGENTS.md`. With `rules-file` set, a `backend/` scope goes to `backend/.goosehints`.
- Rules with the same scope share one file.

### Agents

- Each file has `name`, `description`, an optional free-form `model`, and the prompt body.
- OpenHands reads the same path and fields, so the file is written once.
- A generic `tools` list is dropped with a note, because Goose does not document it.
- `x-goose` fields work. If they make the file differ from what another tool gets, set a different `outputs.goose.agents-dir`.

### Skills

- `.agents/skills/` is Goose's [recommended path](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-skills.md). Goose reads it ahead of `.goose/skills/` and `.claude/skills/`.
- Other `.agents/skills/` tools, Crush included, get the same files, so the shared folder is written once. Shared rendering drops Crush invocation fields; separate its `skills-dir` to keep them.
- Goose lists every skill for the model, and `/<skill-name>` loads one by hand. It has no user-only field, so `disable-model-invocation` is dropped with a note and commands are skipped.

### Hooks

- Written as an [Open Plugins](https://goose-docs.ai/docs/guides/context-engineering/hooks/) package: `.agents/plugins/agnostic-ai/plugin.json` plus `hooks/hooks.json`.
- All 12 documented events work. `matcher` is a regular expression. `timeout` is in seconds (default 30).
- `x-goose.on_failure` takes `allow` or `block` for `PreToolUse` failures.
- [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests hooks with Goose's event data, shell, and timeout.
- `sync --scope user` writes a separate package to `~/.agents/plugins/agnostic-ai-global/`, which Goose loads for every project.
- The global name differs because Goose keeps only the first plugin of a name, and a project `disabledPlugins` entry would turn off both.
- Global sync ignores `outputs.goose.hooks-file` and an absolute `GOOSE_PATH_ROOT`, for hooks and agents, and writes under `~`.

### Plugin skills

- Set `outputs.goose.skills-dir: .agents/plugins/<name>/skills` to bundle skills as a [plugin](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/plugins.md), with or without hooks. Skills and hooks share one manifest.
- Goose prefixes names with the plugin name: `review` in `agnostic-ai` loads as `agnostic-ai:review`.
- A skills directory inside a plugin folder but outside its `skills/` folder is not a Goose component and gets no manifest.
- Sync keeps an existing `plugin.json` it did not write and prints a note. Manifests from an earlier sync stay managed.

### Reviews

Goose [1.60.0 removed `goose review`](https://github.com/aaif-goose/goose/releases/tag/v1.60.0). Review specs are reported as unsupported, and sync writes no root or scoped review files. `outputs.goose.review-file` remains accepted but has no effect.

Portable Review specs stay valid for other supported targets. `agnostic-ai import goose` still reads a legacy root `.agents/REVIEW.md` into a portable review spec. Scoped review files need manual reuse. Sync leaves user-owned review files untouched; previously generated review files follow the normal cleanup of managed output.

There is no automatic rewrite because Goose has no equivalent native review surface.

### MCP

Skipped with a warning.

## Import

`agnostic-ai import goose` reads Goose files back into specs.

| Source | Becomes |
|--------|---------|
| `AGENTS.md` embedded `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.goosehints` | the same, read only when `AGENTS.md` carried no rules |
| `.agents/agents/<name>.md` | `<agents>/<name>.md`, unchanged except for the generated-file header |
| `.agents/skills/<name>/SKILL.md` (+ bundled files) | `<skills>/<name>/SKILL.md` (folder copied unchanged) |
| `.agents/plugins/<name>/skills/<skill>/SKILL.md` | the same, for every enabled plugin in the project |
| `.agents/plugins/<name>/hooks/hooks.json` | one hook spec per matcher group and distinct `timeout` and `on_failure` pair, `on_failure` under `x-goose` |
| Legacy `.agents/REVIEW.md` | `<reviews>/review.md` for supported review targets |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

- `.goosehints` is a fallback. With `rules-file` set, both files hold the same rules.
- Import reads every enabled plugin, hand-installed ones too.
- A plugin in `disabledPlugins` in the user, project, or local Goose settings is skipped. A plugin named in `enabledPlugins` in a closer file stays enabled.
- The `plugins` switch in Goose's `config.yaml` is not read, so a plugin turned off there still imports.
- `agnostic-ai import all` detects Goose only from `.goosehints`, `.agents/REVIEW.md`, or a plugin with `hooks/hooks.json` under `.agents/plugins/`. Antigravity shares `.agents/plugins/`, and OpenHands, Antigravity, and others share `.agents/agents/` and `.agents/skills/`, so those alone do not count. For a project with only rules or plugin skills, run `agnostic-ai import goose`.

Import has these limits:

- Only the rules block in the root file is read back, as with `import claude` and nested `CLAUDE.md` files.
- A legacy root review file becomes one portable spec. Goose no longer emits it.
- Scoped `<scope>/.agents/REVIEW.md` files are not read back.
- A plugin's `scripts/` folder is not copied. Import warns when a written hook command uses `$PLUGIN_ROOT` or `${PLUGIN_ROOT}`, outside the plugin sync writes to. Copy the scripts there before it can run.
- A hook group splits into one spec per distinct `timeout` and `on_failure` pair, so each command keeps its values. The emitted file merges them back into one group.

See [scoped context](@/docs/scoped-context.md).
