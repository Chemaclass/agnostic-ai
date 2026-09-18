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
AGENTS.md                          # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.agents/agents/<name>.md           # one project agent (shared with OpenHands)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree with codex/amp/zed/crush)
.agents/plugins/agnostic-ai/plugin.json          # plugin manifest, whenever a component lands in the plugin
.agents/plugins/agnostic-ai/hooks/hooks.json
.agents/plugins/agnostic-ai/skills/<name>/SKILL.md  # only with skills-dir pointed at the plugin
.goosehints                        # opt-in concatenated rules, only when rules-file is set
.agents/REVIEW.md                  # root review instructions
<scope>/.agents/REVIEW.md          # directory-specific review instructions
```

Block [Goose](https://goose-docs.ai) reads both the root `AGENTS.md` and a `.goosehints` file. By default rule bodies inline into the shared `AGENTS.md` `## Rules` block, so Goose needs no extra file for rules. Set `outputs.goose.rules-file: .goosehints` to also write a concatenated `.goosehints` document.

Project agents load from `.agents/agents/<name>.md` with `name`, `description`, optional free-form `model`, and the prompt body. OpenHands reads the same primary path and fields, so both adapters use one byte-identical renderer and sync dedupes their writes. A generic `tools` list is omitted with a coverage note because Goose's documented project-agent fields do not include it. Target-specific `x-goose` fields remain available; set a different `outputs.goose.agents-dir` if they make the file differ from another target sharing the default path.

Skills load from `.agents/skills/`, [the recommended standard](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-skills.md) in Goose's own docs, ahead of a legacy `.goose/skills/`, `.claude/skills/`, and others it also discovers. The render is byte-identical to codex, amp, zed, and crush's, so the shared tree dedupes into one write. Goose stays opt-in regardless (see [Selecting targets](@/docs/targets/_index.md#selecting-targets)). MCP specs still skip with a warning.

Hooks emit as a complete [Open Plugins](https://goose-docs.ai/docs/guides/context-engineering/hooks/) package. The manifest lives at `.agents/plugins/agnostic-ai/plugin.json`, and the wrapped hook map at `hooks/hooks.json`. All 12 documented events pass through, `matcher` is a regular expression, `timeout` is seconds (vendor default 30), and `x-goose.on_failure` accepts `allow` or `block` for `PreToolUse` failure policy.

Skills are the plugin's other component: "A plugin can provide skills, hooks, or both", and "A plugin is a directory with a plugin manifest and optional component directories" ([plugins.md](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/plugins.md)). Set `outputs.goose.skills-dir: .agents/plugins/<name>/skills` to bundle skills as a plugin, and the manifest is written whether or not the project has hooks. It used to appear only as a side effect of emitting hooks, so a skills-only bundle was undiscoverable (target-audit 2026-09-18, #862). Goose namespaces a plugin's skills, loading `review` in `agnostic-ai` as `agnostic-ai:review`. One plugin carrying both components gets one manifest. A skills dir under a plugin root but outside its `skills/` directory is not a component Goose reads, so it gets no manifest.

Goose discovers additional context files (any of `CONTEXT_FILE_NAMES`, default `AGENTS.md` and `.goosehints`) as it reads or modifies files in nested subdirectories, not just at the working directory and repository root. Scoped rules use nested `AGENTS.md` by default. With the rules-file opt-in set, a rule carrying a source-layout or frontmatter scope (e.g. `backend/`) routes into a sibling `backend/.goosehints` instead of flattening into the root document. Rules sharing a scope concatenate into that scope's one file, the same "one file per scope" shape the root document already used (#608).

`goose review` reads `.agents/REVIEW.md` and `<scope>/.agents/REVIEW.md` from directories containing changed files and their ancestors, so root and scoped guidance compose. Same-scope bodies concatenate, and routing frontmatter is omitted because the loader reads plain text. The [v1.50.0 CLI reference](https://raw.githubusercontent.com/aaif-goose/goose/v1.50.0/documentation/docs/guides/goose-cli-commands.md) documents this surface. Agent-shaped check files remain outside this adapter.

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
3. Launch `goose`; it reads `AGENTS.md` (and `.goosehints` when present) as context, each `.agents/agents/<name>.md` appears as a project agent, and each `.agents/skills/<name>/` appears as a skill.
