+++
title = "Junie"
description = "How agnostic-ai emits Junie configuration: native paths, capability limits, and output options."
weight = 150

[extra]
group = "Reference"
target_id = "junie"
+++

# Junie (`junie`)

## Output

```
AGENTS.md                        # root pointer body (shared path; fallback)
.junie/AGENTS.md                 # preferred entry point: pointer body + inlined rules
.junie/agents/<name>.md          # one file per subagent
.junie/skills/<name>/SKILL.md    # one folder per skill, plus any bundled assets
.junie/commands/<name>.md        # one file per slash command
.junie/mcp/mcp.json              # when MCP entries exist
.junie/config.json               # when a settings spec selects a default model
.aiignore                        # when ignore entries exist
```

Junie picks one guidelines source, first match wins, with no merging ([IDE plugin](https://junie.jetbrains.com/docs/junie-ide-plugin.html), [guidelines](https://junie.jetbrains.com/docs/guidelines-and-memory.html)):

1. `.junie/AGENTS.md` (the preferred location).
2. The root `AGENTS.md`, combined with `.junie/playbook.md` and every `.junie/rules/*.md` file, when nothing is found in `.junie/`.
3. The legacy `.junie/guidelines.md` / `.junie/guidelines/`.

`sync` always writes `.junie/AGENTS.md`, so step 1 always matches and Junie [uses it exclusively](https://junie.jetbrains.com/docs/environment-variables.html). Step 2, including `.junie/playbook.md` and `.junie/rules/*.md`, never applies in a synced project. The IDE plugin also reads an optional **Custom path** (Settings | Tools | Junie | Project Settings) before `.junie/AGENTS.md`. The CLI has no such step, and the setting is rarely committed, so it seldom changes which file wins.

Rule bodies inline into `.junie/AGENTS.md` in a sentinel-marked `## Rules` block right after the pointer body, using the same `### <name>` shape (source comment, optional description, full body) as every inlining target.

Older versions wrote rules and agents to `.junie/rules/`. Files there are shadowed by `.junie/AGENTS.md`. Sync sweeps agnostic-ai-managed leftovers there and keeps hand-authored files.

Subagents and slash commands are Junie CLI features; the IDE plugin docs mention neither.

Subagents emit to `.junie/agents/<name>.md`. [Junie](https://junie.jetbrains.com/docs/junie-cli-subagents.html) reads subagents from `.junie/agents/` or `.agents/`, and this adapter defaults to `.junie/agents/`, the vendor's preferred location (Junie CLI offers to import `.cursor/agents/`, `.claude/agents/`, and `.codex/agents/` into it). Set `outputs.junie.agents-dir: .agents` for the shared alternative, like Codex's `outputs.codex.agents-dir: .agents/agents` layout.

- Agent names must match `[a-z][a-z0-9_-]*`. When `name` is missing, Junie uses the filename, so the filename follows the same rule. An invalid name such as `Deploy_Bot` fails sync with the name and required format; it is not renamed. This rule allows underscores, unlike the OpenCode and Zed skill rule.
- Frontmatter passes through verbatim. The documented fields (`name`, `description`, `tools`, `disallowedTools`, `mcpServers`, `model`, `permissionMode`, `reasoningLevel`, `maxTurns`, `skills`, `allowPromptArgument`) already match spec spelling. `tools` and `disallowedTools` pass through as YAML lists.
- Junie's tool group labels match the Claude names for each group it documents, plus `AskUserQuestion`. It has no `WebFetch`, `Task`, `TodoWrite`, or `NotebookEdit`.
- `reasoningLevel` also accepts `effort` as an alias, which wins when both are set. Both pass through unchanged.
- Agent bodies do not inline into `.junie/AGENTS.md`, the same rule Augment and Kilo Code follow for their native agents directories. `.junie/AGENTS.md` is fully regenerated on each sync, so an older inlined `## Agents` block disappears on the next sync.
- With `sync.target-overview` off, `.junie/AGENTS.md` and the root `AGENTS.md` are byte-identical when another AGENTS.md-family target is enabled.
- The auto model-selection policy toggle (`/settings → Subagents`) is Early Access. The file format and discovery are not.

Slash commands emit to `.junie/commands/<name>.md`, the [project commands folder](https://junie.jetbrains.com/docs/custom-slash-commands.html). Junie documents two frontmatter fields, `description` and `allowPromptArgument` (free-form text via a `$prompt` placeholder). Other keys pass through verbatim, as for agents. The body may use `$argumentName` placeholders that Junie fills at invocation.

Skills emit to `.junie/skills/<name>/SKILL.md`. Junie CLI also loads `.agents/skills/` in a trusted project ([agent-skills](https://junie.jetbrains.com/docs/agent-skills.html)), so enabling junie with a target that writes `.agents/skills/` gives Junie two copies of each skill. Nothing is lost. A flat file never loads as a skill, and bundled assets copy byte-for-byte.

MCP servers use the standard `mcpServers` schema at `.junie/mcp/mcp.json` (`command`/`args`/`env` local, `url`/`headers` remote). Those are the only [documented keys](https://junie.jetbrains.com/docs/junie-cli-mcp-configuration.html), so `disabled` and `description` are stripped with a coverage note. Junie enables imported servers by default, so a `disabled` server would arrive enabled. Disable a server with `/mcp` then **→ Disable**. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

A settings spec's default `model` merges into `.junie/config.json`, keeping unrelated native keys. An `x-junie` block on that spec merges into the same file, for project-config keys this tool does not model.

Ignore specs emit as `.aiignore` in the project root, which uses `.gitignore` syntax ([IDE plugin](https://junie.jetbrains.com/docs/junie-ide-plugin.html)). It is softer than a block: Junie asks for approval before viewing or editing a listed file. Only contents are protected (names stay visible), and Brave Mode or an allowlisted command naming the path skips the prompt. Only the IDE plugin docs mention `.aiignore`; no Junie CLI page does. Multiple specs concatenate. Override via `outputs.junie.ignore-file`.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.junie.agents-dir` | `.junie/agents` | |
| `outputs.junie.skills-dir` | `.junie/skills` | |
| `outputs.junie.commands-dir` | `.junie/commands` | |
| `outputs.junie.mcp-file` | `.junie/mcp/mcp.json` | |
| `outputs.junie.ignore-file` | `.aiignore` | |
| `outputs.junie.rules-dir` | `.junie/rules` | legacy: only redirects the leftover sweep for projects that customized it |

`.junie/AGENTS.md` is a fixed path, not configurable.

## Import

`agnostic-ai import junie` reads:

- The sentinel-marked Rules block in `.junie/AGENTS.md`.
- Agents from `.junie/agents/<name>.md` (or `.agents/<name>.md`).
- Commands from `.junie/commands/<name>.md`.
- Skills from `.junie/skills/<name>/SKILL.md` and `.agents/skills/`, native directory first for duplicate names. Bundled assets copy byte-for-byte.
- The default `model` in `.junie/config.json`, restored to `settings/junie.yaml`.

Older layouts still import:

- With no native agent files, import falls back to the older sentinel-marked Agents block in `.junie/AGENTS.md`.
- Content flattened under `.junie/rules/` by older versions takes precedence over `.junie/AGENTS.md` when that directory exists. Each file is reclassified by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification).
- A legacy flat `.junie/rules/skill-<name>.md` imports as a skill.

## Verify

1. Install the Junie plugin in a JetBrains IDE (or the Junie CLI, [docs](https://junie.jetbrains.com/docs/)).
2. Check the tree: `ls AGENTS.md .junie/AGENTS.md .junie/agents/ .junie/skills/ .junie/commands/`, `grep "Generated by agnostic-ai" .junie/AGENTS.md`, `python -m json.tool .junie/mcp/mcp.json > /dev/null`.
3. Ask Junie to list its guidelines. The rules inlined in `.junie/AGENTS.md` apply, each `.junie/agents/<name>.md` is a delegatable subagent, each `.junie/skills/<name>/` is a skill, and each `.junie/commands/<name>.md` is a `/name` slash command.
