+++
title = "Goose"
description = "What agnostic-ai writes for Goose: file paths, what it supports, and output settings."
weight = 240

[extra]
group = "Reference"
target_id = "goose"
+++

# Goose (`goose`)

Block [Goose](https://goose-docs.ai) reads the root `AGENTS.md` and a `.goosehints` file. Goose is opt-in. See [Selecting targets](@/docs/configuration.md#targets).

## Output

```
AGENTS.md                          # entry-point pointer body + inlined rules (shared path)
.agents/agents/<name>.md           # one project agent (shared with OpenHands)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree)
.agents/plugins/agnostic-ai/plugin.json          # plugin manifest, whenever a component lands in the plugin
.agents/plugins/agnostic-ai/hooks/hooks.json
.agents/plugins/agnostic-ai/skills/<name>/SKILL.md  # only with skills-dir pointed at the plugin
.goosehints                        # opt-in concatenated rules, only when rules-file is set
.agents/REVIEW.md                  # root review instructions
<scope>/.agents/REVIEW.md          # directory-specific review instructions
```

- **Rules**: go inline into the `## Rules` block of `AGENTS.md`. `outputs.goose.rules-file: .goosehints` also writes all rules into one `.goosehints`, which Goose reads only with the `Developer` extension enabled ([using goosehints](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-goosehints.md)).
- **Scoped rules**: Goose reads context files (`CONTEXT_FILE_NAMES`, default `AGENTS.md` and `.goosehints`) in the subdirectories it works in. Scoped rules go to a nested `AGENTS.md`, or with `rules-file` set, to `backend/.goosehints` for a `backend/` scope. Rules with the same scope share one file.
- **Agents**: each file has `name`, `description`, an optional free-form `model`, and the prompt body. OpenHands reads the same path and fields, so the file is written once. A generic `tools` list is dropped with a coverage note, because Goose does not document it. `x-goose` fields work. If they make the file differ from what another tool gets, set a different `outputs.goose.agents-dir`.
- **Skills**: `.agents/skills/` is Goose's [recommended path](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-skills.md), read ahead of `.goose/skills/` and `.claude/skills/`. The other `.agents/skills/` tools get the same files, so the shared folder is written once.
- **Hooks**: an [Open Plugins](https://goose-docs.ai/docs/guides/context-engineering/hooks/) package (`.agents/plugins/agnostic-ai/plugin.json` plus `hooks/hooks.json`). All 12 documented events are supported. `matcher` is a regular expression. `timeout` is in seconds (default 30). `x-goose.on_failure` takes `allow` or `block` for `PreToolUse` failures. [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs these hooks with Goose's event data, shell, and timeout, so you can test them before a session does.
- **Plugin skills**: set `outputs.goose.skills-dir: .agents/plugins/<name>/skills` to bundle skills as a plugin ([plugins](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/plugins.md)), with or without hooks. Skills and hooks share one manifest. Goose prefixes names with the plugin name, so `review` in `agnostic-ai` loads as `agnostic-ai:review`.
- **Reviews**: `goose review` reads `.agents/REVIEW.md` and `<scope>/.agents/REVIEW.md` from every directory with changed files and every parent directory, so root and scoped guidance combine ([v1.50.0 CLI reference](https://raw.githubusercontent.com/aaif-goose/goose/v1.50.0/documentation/docs/guides/goose-cli-commands.md)). Specs with the same scope are joined into one file, without frontmatter, because Goose reads plain text. Agent-style check files are not written.
- **MCP**: skipped with a warning.

{% <details summary="Skills dir outside skills/"> %}
A skills directory inside a plugin folder but outside its `skills/` folder is not a Goose component and gets no manifest.
{% </details> %}

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.goose.agents-dir` | `.agents/agents` | |
| `outputs.goose.rules-file` | unset | opt-in, writes all rules into one `.goosehints` file |
| `outputs.goose.skills-dir` | `.agents/skills` | point it at `.agents/plugins/<name>/skills` to bundle skills as a plugin; the manifest follows |
| `outputs.goose.hooks-file` | `.agents/plugins/agnostic-ai/hooks/hooks.json` | overrides must keep the `<plugin>/hooks/hooks.json` suffix |
| `outputs.goose.review-file` | `.agents/REVIEW.md` | relative to each scope |

## Import

`agnostic-ai import goose` reads the Goose files back into specs. Rules come from the inline block in `AGENTS.md`:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.goosehints` | the same, read only when `AGENTS.md` carried no rules |
| `.agents/agents/<name>.md` | `<agents>/<name>.md`, unchanged except for the provenance header |
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied unchanged) |
| `.agents/plugins/<name>/skills/<skill>/SKILL.md` | the same, for every plugin in the project |
| `.agents/plugins/<name>/hooks/hooks.json` | one hook spec per matcher group, `on_failure` under `x-goose` |
| `.agents/REVIEW.md` | `<reviews>/review.md` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

`.goosehints` is a fallback. With `rules-file` set, both files hold the same rules.

Import reads every plugin, including hand-installed ones.

`agnostic-ai import all` detects Goose only from `.goosehints`, `.agents/plugins/`, or `.agents/REVIEW.md`, because OpenHands, Antigravity, and others share `.agents/agents/` and `.agents/skills/`. For a Goose project with only rules, run `agnostic-ai import goose` directly.

Import loses the following, but the next sync still writes the same Goose files:

- Only the rules block in the root file is read back, as with `import claude` and nested `CLAUDE.md` files.
- Review specs with the same scope come back as one spec.
- Scoped `<scope>/.agents/REVIEW.md` files are not read back.

See [scoped context](@/docs/scoped-context.md).

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Goose ([docs](https://goose-docs.ai)).
2. Check the files: `ls AGENTS.md .agents/agents/ .agents/skills/`, plus `.goosehints` when `outputs.goose.rules-file` is set.
3. Launch `goose`. It reads `AGENTS.md` (and `.goosehints` when present) as context. It lists each `.agents/agents/<name>.md` as a project agent and each `.agents/skills/<name>/` as a skill.
