+++
title = "Goose"
description = "How agnostic-ai emits Goose configuration: native paths, capability limits, and output options."
weight = 240

[extra]
group = "Reference"
target_id = "goose"
+++

# Goose (`goose`)

Block [Goose](https://goose-docs.ai) reads the root `AGENTS.md` and a `.goosehints` file. Goose is opt-in (see [Selecting targets](@/docs/configuration.md#targets)).

## Output

```
AGENTS.md                          # entry-point pointer body + inlined rules (shared path)
.agents/agents/<name>.md           # one project agent (shared with OpenHands)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree with codex/amp/zed/crush)
.agents/plugins/agnostic-ai/plugin.json          # plugin manifest, whenever a component lands in the plugin
.agents/plugins/agnostic-ai/hooks/hooks.json
.agents/plugins/agnostic-ai/skills/<name>/SKILL.md  # only with skills-dir pointed at the plugin
.goosehints                        # opt-in concatenated rules, only when rules-file is set
.agents/REVIEW.md                  # root review instructions
<scope>/.agents/REVIEW.md          # directory-specific review instructions
```

- **Rules**: inline into the `AGENTS.md` `## Rules` block. `outputs.goose.rules-file: .goosehints` also writes a concatenated `.goosehints`, which Goose reads only with the `Developer` extension enabled ([using goosehints](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-goosehints.md)).
- **Scoped rules**: Goose reads context files (`CONTEXT_FILE_NAMES`, default `AGENTS.md` and `.goosehints`) in subdirectories it works in. Scoped rules go to a nested `AGENTS.md`, or with `rules-file` set, to `backend/.goosehints` for a `backend/` scope. Rules sharing a scope share one file.
- **Agents**: `name`, `description`, optional free-form `model`, and the prompt body. OpenHands reads the same path and fields, so sync writes each file once. A generic `tools` list is dropped with a coverage note; Goose does not document it. `x-goose` fields work. If they make the file differ from another target's, set a different `outputs.goose.agents-dir`.
- **Skills**: `.agents/skills/` is Goose's [recommended path](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-skills.md), read ahead of `.goose/skills/` and `.claude/skills/`. codex, amp, zed, and crush render the same files, so the shared tree dedupes.
- **Hooks**: an [Open Plugins](https://goose-docs.ai/docs/guides/context-engineering/hooks/) package (`.agents/plugins/agnostic-ai/plugin.json` plus `hooks/hooks.json`). All 12 documented events pass through. `matcher` is a regular expression. `timeout` is in seconds (default 30). `x-goose.on_failure` takes `allow` or `block` for `PreToolUse` failures. [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs these hooks with Goose's payload, shell, and timeout before a session does.
- **Plugin skills**: set `outputs.goose.skills-dir: .agents/plugins/<name>/skills` to bundle skills as a plugin ([plugins](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/plugins.md)), with or without hooks; both share one manifest. Goose namespaces them, so `review` in `agnostic-ai` loads as `agnostic-ai:review`.
- **Reviews**: `goose review` reads `.agents/REVIEW.md` and `<scope>/.agents/REVIEW.md` from directories with changed files and their ancestors, so root and scoped guidance compose ([v1.50.0 CLI reference](https://raw.githubusercontent.com/aaif-goose/goose/v1.50.0/documentation/docs/guides/goose-cli-commands.md)). Same-scope bodies concatenate, without routing frontmatter, since Goose reads plain text. Agent-shaped check files are not emitted.
- **MCP**: skipped with a warning.

{% <details summary="Skills dir outside skills/"> %}
A skills dir under a plugin root but outside its `skills/` directory is not a Goose component and gets no manifest.
{% </details> %}

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.goose.agents-dir` | `.agents/agents` | |
| `outputs.goose.rules-file` | unset | opt-in, writes a concatenated `.goosehints` document |
| `outputs.goose.skills-dir` | `.agents/skills` | point it at `.agents/plugins/<name>/skills` to bundle skills as a plugin; the manifest follows |
| `outputs.goose.hooks-file` | `.agents/plugins/agnostic-ai/hooks/hooks.json` | overrides must keep the `<plugin>/hooks/hooks.json` suffix |
| `outputs.goose.review-file` | `.agents/REVIEW.md` | relative to each scope |

## Import

`agnostic-ai import goose` reverses the Goose layout. Rules come from the inlined block in `AGENTS.md`:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.goosehints` | the same, read only when `AGENTS.md` carried no rules |
| `.agents/agents/<name>.md` | `<agents>/<name>.md`, byte-for-byte minus the provenance header |
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied byte-for-byte) |
| `.agents/plugins/<name>/skills/<skill>/SKILL.md` | the same, for every plugin in the project |
| `.agents/plugins/<name>/hooks/hooks.json` | one hook spec per matcher group, `on_failure` under `x-goose` |
| `.agents/REVIEW.md` | `<reviews>/review.md` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

`.goosehints` is a fallback: with `rules-file` set, both files hold the same rules.

Import reads every plugin, including hand-installed ones.

`agnostic-ai import all` detects Goose only from `.goosehints`, `.agents/plugins/`, or `.agents/REVIEW.md`, since OpenHands, Antigravity, and others share `.agents/agents/` and `.agents/skills/`. For a rules-only Goose project, run `agnostic-ai import goose` directly.

Lossy on import, with no change to Goose's output on the next sync:

- Only the root rules block is read back, the same as `import claude` with nested `CLAUDE.md` files.
- Review specs sharing a scope re-import as one spec.
- Scoped `<scope>/.agents/REVIEW.md` files are not read back.

See [scoped context](@/docs/scoped-context.md).

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Goose ([docs](https://goose-docs.ai)).
2. Check the tree: `ls AGENTS.md .agents/agents/ .agents/skills/`, plus `.goosehints` when `outputs.goose.rules-file` is set.
3. Launch `goose`. It reads `AGENTS.md` (and `.goosehints` when present) as context, lists each `.agents/agents/<name>.md` as a project agent, and each `.agents/skills/<name>/` as a skill.
