+++
title = "Junie"
description = "What agnostic-ai writes for Junie: file paths, what Junie cannot do, and output options."
weight = 150

[extra]
group = "Reference"
target_id = "junie"
+++

# Junie (`junie`)

JetBrains Junie reads `.junie/AGENTS.md` (pointer body and rules), the rest of `.junie/`, and `.aiignore`.

## Output

```
AGENTS.md                        # root pointer body (shared path; fallback)
.junie/AGENTS.md                 # preferred entry point: pointer body + rules
.junie/agents/<name>.md          # one file per subagent
.junie/skills/<name>/SKILL.md    # one folder per skill, plus any bundled assets
.junie/commands/<name>.md        # one file per slash command
.junie/mcp/mcp.json              # when MCP entries exist
.junie/config.json               # when a settings spec selects a default model
.aiignore                        # when ignore entries exist
```

Junie uses the first guidelines source it finds, with no merging ([IDE plugin](https://junie.jetbrains.com/docs/junie-ide-plugin.html), [guidelines](https://junie.jetbrains.com/docs/guidelines-and-memory.html)):

1. `.junie/AGENTS.md` (preferred).
2. The root `AGENTS.md` plus `.junie/playbook.md` and every `.junie/rules/*.md`, when `.junie/` has nothing.
3. The legacy `.junie/guidelines.md` / `.junie/guidelines/`.

`sync` always writes `.junie/AGENTS.md`, so Junie [uses it exclusively](https://junie.jetbrains.com/docs/environment-variables.html) and step 2 never applies.

- **Rules**: a `## Rules` block right after the pointer body, marked so sync can find it. Each rule is a `### <name>` section (source comment, optional description, full body), the same shape every tool that inlines rules uses.
- **Agents** ([docs](https://junie.jetbrains.com/docs/junie-cli-subagents.html)): Junie reads `.junie/agents/` (the vendor's preferred location and the default here) or `.agents/`. Junie CLI offers to import `.cursor/agents/`, `.claude/agents/`, and `.codex/agents/` into it. Set `outputs.junie.agents-dir: .agents` for the shared alternative, like Codex's `outputs.codex.agents-dir: .agents/agents`.
  - Names, and filenames when `name` is missing, must match `[a-z][a-z0-9_-]*` (underscores allowed, unlike the OpenCode and Zed skill rule). An invalid name such as `Deploy_Bot` fails sync, and the error shows the required format. Sync does not rename it.
  - Frontmatter is copied as written. The documented fields (`name`, `description`, `tools`, `disallowedTools`, `mcpServers`, `model`, `permissionMode`, `reasoningLevel`, `maxTurns`, `skills`, `allowPromptArgument`) use the same spelling as specs, and `tools` and `disallowedTools` stay YAML lists.
  - Tool group labels match the Claude names for each documented group, plus `AskUserQuestion`. There is no `WebFetch`, `Task`, `TodoWrite`, or `NotebookEdit`.
  - `reasoningLevel` also accepts the alias `effort`, which wins when both are set. Both are copied as written.
  - Agent bodies are not copied into `.junie/AGENTS.md`, as with Augment and Kilo Code.
  - Early Access covers only the automatic model-selection toggle (`/settings → Subagents`), not the file format or how Junie finds agents.
- **Commands** ([project commands folder](https://junie.jetbrains.com/docs/custom-slash-commands.html)): Junie documents `description` and `allowPromptArgument` (free-form text through a `$prompt` placeholder). Other keys are copied as written. Junie fills `$argumentName` placeholders in the body when you run the command.
- **CLI only**: the IDE plugin docs mention neither subagents nor slash commands.
- **Skills**: bundled assets are copied as they are, and a flat file never loads as a skill. Junie CLI also loads `.agents/skills/` in a trusted project ([agent-skills](https://junie.jetbrains.com/docs/agent-skills.html)), so another tool that writes it gives Junie a harmless second copy.
- **MCP**: `.junie/mcp/mcp.json` in the standard `mcpServers` schema (`command`/`args`/`env` local, `url`/`headers` remote). Those are the only [documented keys](https://junie.jetbrains.com/docs/junie-cli-mcp-configuration.html), so `disabled` and `description` are dropped with a coverage note. Junie enables imported servers by default. Disable one with `/mcp`, then **→ Disable**. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
- **Settings**: a settings spec's default `model` merges into `.junie/config.json`, keeping other keys. `x-junie` adds project-config keys that have no portable field.
- **Ignore**: `.aiignore` in the project root, in `.gitignore` syntax ([IDE plugin](https://junie.jetbrains.com/docs/junie-ide-plugin.html)). Specs are joined. Override the path with `outputs.junie.ignore-file`.
  - Junie asks before it views or edits a listed file instead of blocking it, and it protects contents, not names.
  - Brave Mode, or an allowlisted command that names the path, skips the prompt.
  - Only the IDE plugin docs mention `.aiignore`. No CLI page does.

With `sync.target-overview` off and another tool that reads `AGENTS.md` enabled, `.junie/AGENTS.md` and the root `AGENTS.md` are identical.

{% <details summary="IDE plugin Custom path"> %}
The IDE plugin reads an optional **Custom path** (Settings | Tools | Junie | Project Settings) before `.junie/AGENTS.md`. The CLI does not, and the setting is rarely committed, so it seldom changes which file wins.
{% </details> %}

{% <details summary="Files from older versions"> %}
Older versions wrote rules and agents to `.junie/rules/`, which `.junie/AGENTS.md` hides. Sync removes generated leftovers there and keeps hand-written files. It also drops an older `## Agents` block, because it rewrites `.junie/AGENTS.md` in full.
{% </details> %}

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.junie.agents-dir` | `.junie/agents` | |
| `outputs.junie.skills-dir` | `.junie/skills` | |
| `outputs.junie.commands-dir` | `.junie/commands` | |
| `outputs.junie.mcp-file` | `.junie/mcp/mcp.json` | |
| `outputs.junie.ignore-file` | `.aiignore` | |
| `outputs.junie.rules-dir` | `.junie/rules` | legacy: only changes where leftover files are removed, for projects that set it |

`.junie/AGENTS.md` is a fixed path.

## Import

`agnostic-ai import junie` reads:

- The Rules block in `.junie/AGENTS.md`.
- Agents from `.junie/agents/<name>.md` (or `.agents/<name>.md`).
- Commands from `.junie/commands/<name>.md`.
- Skills, with bundled assets, from `.junie/skills/<name>/SKILL.md`, then `.agents/skills/` for names not already found.
- The default `model` in `.junie/config.json`, restored to `settings/junie.yaml`.

Older layouts still import:

- With no native agent files, the older Agents block in `.junie/AGENTS.md`.
- `.junie/rules/`, when it exists, wins over `.junie/AGENTS.md`. Each file is sorted by [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification).
- A legacy flat `.junie/rules/skill-<name>.md` imports as a skill.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the Junie plugin in a JetBrains IDE (or the Junie CLI, [docs](https://junie.jetbrains.com/docs/)).
2. Check the tree:
   - `ls AGENTS.md .junie/AGENTS.md .junie/agents/ .junie/skills/ .junie/commands/`
   - `grep "Generated by agnostic-ai" .junie/AGENTS.md`
   - `python -m json.tool .junie/mcp/mcp.json > /dev/null`
3. Ask Junie to list its guidelines. The rules apply, each `.junie/agents/<name>.md` is a delegatable subagent, each `.junie/skills/<name>/` a skill, and each `.junie/commands/<name>.md` a `/name` slash command.
