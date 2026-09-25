+++
title = "Goose"
description = "How agnostic-ai emits Goose configuration: native paths, capability limits, and output options."
weight = 240

[extra]
group = "Reference"
target_id = "goose"
+++

# Goose (`goose`)

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

Block [Goose](https://goose-docs.ai) reads the root `AGENTS.md` and a `.goosehints` file. By default, rule bodies inline into the `AGENTS.md` `## Rules` block. Set `outputs.goose.rules-file: .goosehints` to also write a concatenated `.goosehints`. Goose reads that file only with the `Developer` extension enabled ([using goosehints](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-goosehints.md)); otherwise it is inert.

Project agents load from `.agents/agents/<name>.md` with `name`, `description`, optional free-form `model`, and the prompt body. OpenHands reads the same path and fields, so both adapters share one renderer and sync dedupes the writes. A generic `tools` list is dropped with a coverage note, since Goose does not document it. `x-goose` fields still work; if they make the file differ from another target on the same path, set a different `outputs.goose.agents-dir`.

Skills load from `.agents/skills/`, Goose's [recommended path](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-skills.md), ahead of legacy paths like `.goose/skills/` and `.claude/skills/`. The render matches codex, amp, zed, and crush, so the shared tree dedupes. Goose stays opt-in (see [Selecting targets](@/docs/configuration.md#targets)). MCP specs skip with a warning.

Hooks emit as an [Open Plugins](https://goose-docs.ai/docs/guides/context-engineering/hooks/) package: the manifest at `.agents/plugins/agnostic-ai/plugin.json` and the hook map at `hooks/hooks.json`. All 12 documented events pass through. `matcher` is a regular expression, `timeout` is in seconds (default 30), and `x-goose.on_failure` takes `allow` or `block` for `PreToolUse` failures.

A plugin can also carry skills ([plugins](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/plugins.md)). Set `outputs.goose.skills-dir: .agents/plugins/<name>/skills` to bundle skills as a plugin; the manifest is written with or without hooks. Goose namespaces plugin skills, so `review` in `agnostic-ai` loads as `agnostic-ai:review`. A plugin with both skills and hooks gets one manifest. A skills dir under a plugin root but outside its `skills/` directory is not a Goose component and gets no manifest.

Goose also picks up context files (`CONTEXT_FILE_NAMES`, default `AGENTS.md` and `.goosehints`) in nested subdirectories as it reads or edits files there. Scoped rules use nested `AGENTS.md` by default. With `rules-file` set, a scoped rule (e.g. `backend/`) goes to `backend/.goosehints` instead. Rules sharing a scope concatenate into one file.

`goose review` reads `.agents/REVIEW.md` and `<scope>/.agents/REVIEW.md` from directories with changed files and their ancestors, so root and scoped guidance compose ([v1.50.0 CLI reference](https://raw.githubusercontent.com/aaif-goose/goose/v1.50.0/documentation/docs/guides/goose-cli-commands.md)). Same-scope bodies concatenate, and routing frontmatter is omitted because Goose reads plain text. Agent-shaped check files are not emitted.

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

`.goosehints` is a fallback, not an addition: with `rules-file` set, both files hold the same rules, so reading both would import each rule twice.

Every plugin is read, including hand-installed ones, not only the `agnostic-ai` package.

`agnostic-ai import all` detects Goose only from `.goosehints`, `.agents/plugins/`, or `.agents/REVIEW.md`, because `.agents/agents/` and `.agents/skills/` are shared with OpenHands, Antigravity, and others. A project that syncs only rules to Goose has no Goose marker and is not detected; run `agnostic-ai import goose` directly.

Lossy on import, with no change to Goose's output on the next sync:

- Only the root rules block is read back, the same as `import claude` with nested `CLAUDE.md` files.
- Review specs sharing a scope re-import as one spec.
- Scoped `<scope>/.agents/REVIEW.md` files are not read back.

See [scoped context](@/docs/scoped-context.md).

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.goose.agents-dir` | `.agents/agents` | |
| `outputs.goose.rules-file` | unset | opt-in, writes a concatenated `.goosehints` document |
| `outputs.goose.skills-dir` | `.agents/skills` | point it at `.agents/plugins/<name>/skills` to bundle skills as a plugin; the manifest follows |
| `outputs.goose.hooks-file` | `.agents/plugins/agnostic-ai/hooks/hooks.json` | overrides must keep the `<plugin>/hooks/hooks.json` suffix |
| `outputs.goose.review-file` | `.agents/REVIEW.md` | relative to each scope |

## Verify

1. Install Goose ([docs](https://goose-docs.ai)).
2. Check the tree: `ls AGENTS.md .agents/agents/ .agents/skills/`, plus `.goosehints` when `outputs.goose.rules-file` is set.
3. Launch `goose`. It reads `AGENTS.md` (and `.goosehints` when present) as context, lists each `.agents/agents/<name>.md` as a project agent, and each `.agents/skills/<name>/` as a skill.
