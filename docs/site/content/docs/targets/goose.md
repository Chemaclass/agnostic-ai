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

Block [Goose](https://goose-docs.ai) reads both the root `AGENTS.md` and a `.goosehints` file. By default rule bodies inline into the shared `AGENTS.md` `## Rules` block, so Goose needs no extra file for rules. Set `outputs.goose.rules-file: .goosehints` to also write a concatenated `.goosehints` document. That file needs one thing on the Goose side to do anything: ":::info Developer extension required / To make use of the hints file, you need to have the `Developer` extension enabled" ([using-goosehints.md](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-goosehints.md)). With the extension off the file is written and inert.

Project agents load from `.agents/agents/<name>.md` with `name`, `description`, optional free-form `model`, and the prompt body. OpenHands reads the same primary path and fields, so both adapters use one byte-identical renderer and sync dedupes their writes. A generic `tools` list is omitted with a coverage note because Goose's documented project-agent fields do not include it. Target-specific `x-goose` fields remain available; set a different `outputs.goose.agents-dir` if they make the file differ from another target sharing the default path.

Skills load from `.agents/skills/`, [the recommended standard](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-skills.md) in Goose's own docs, ahead of a legacy `.goose/skills/`, `.claude/skills/`, and others it also discovers. The render is byte-identical to codex, amp, zed, and crush's, so the shared tree dedupes into one write. Goose stays opt-in regardless (see [Selecting targets](@/docs/targets/_index.md#selecting-targets)). MCP specs still skip with a warning.

Hooks emit as a complete [Open Plugins](https://goose-docs.ai/docs/guides/context-engineering/hooks/) package. The manifest lives at `.agents/plugins/agnostic-ai/plugin.json`, and the wrapped hook map at `hooks/hooks.json`. All 12 documented events pass through, `matcher` is a regular expression, `timeout` is seconds (vendor default 30), and `x-goose.on_failure` accepts `allow` or `block` for `PreToolUse` failure policy.

Skills are the plugin's other component: "A plugin can provide skills, hooks, or both", and "A plugin is a directory with a plugin manifest and optional component directories" ([plugins.md](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/plugins.md)). Set `outputs.goose.skills-dir: .agents/plugins/<name>/skills` to bundle skills as a plugin, and the manifest is written whether or not the project has hooks. It used to appear only as a side effect of emitting hooks, so a skills-only bundle was undiscoverable (target-audit 2026-09-18, #862). Goose namespaces a plugin's skills, loading `review` in `agnostic-ai` as `agnostic-ai:review`. One plugin carrying both components gets one manifest. A skills dir under a plugin root but outside its `skills/` directory is not a component Goose reads, so it gets no manifest.

Goose discovers additional context files (any of `CONTEXT_FILE_NAMES`, default `AGENTS.md` and `.goosehints`) as it reads or modifies files in nested subdirectories, not just at the working directory and repository root. Scoped rules use nested `AGENTS.md` by default. With the rules-file opt-in set, a rule carrying a source-layout or frontmatter scope (e.g. `backend/`) routes into a sibling `backend/.goosehints` instead of flattening into the root document. Rules sharing a scope concatenate into that scope's one file, the same "one file per scope" shape the root document already used (#608).

`goose review` reads `.agents/REVIEW.md` and `<scope>/.agents/REVIEW.md` from directories containing changed files and their ancestors, so root and scoped guidance compose. Same-scope bodies concatenate, and routing frontmatter is omitted because the loader reads plain text. The [v1.50.0 CLI reference](https://raw.githubusercontent.com/aaif-goose/goose/v1.50.0/documentation/docs/guides/goose-cli-commands.md) documents this surface. Agent-shaped check files remain outside this adapter.

## Import

`agnostic-ai import goose` reverses the Goose layout. Goose has no per-rule directory, so rules ride inside the shared `AGENTS.md`:

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

The `.goosehints` fallback is one-way rather than additive. With `outputs.goose.rules-file` set, sync writes the same rule bodies to both files, so reading both would import every rule twice.

`agnostic-ai import all` detects Goose from `.goosehints`, `.agents/plugins/`, or `.agents/REVIEW.md`. These stay deliberately narrow because `.agents/` is a shared convention: `.agents/agents/` and `.agents/skills/` would also match OpenHands, Antigravity and most of the registry. One gap follows from that. A project that syncs Goose with rules and nothing else writes only the shared `AGENTS.md`, carries no Goose-specific marker, and is not detected; run `agnostic-ai import goose` directly for it.

Every plugin is read, not just the `agnostic-ai` package this tool writes: a hand-installed one is exactly the configuration a migrating project wants picked up.

Lossy fields, none of which change Goose's output on the next sync: rules reach Goose through one inlined block per scope and only the root block is read back, the same line `import claude` holds on nested `CLAUDE.md` files; review specs sharing a scope concatenate into one file on emit and re-import as a single spec; a scoped `<scope>/.agents/REVIEW.md` is not read back for the same reason. See [scoped context](@/docs/scoped-context.md).

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
