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
AGENTS.md                        # root entry-point pointer body (written by sync, shared path; fallback)
.junie/AGENTS.md                 # preferred entry-point: pointer body + inlined rules (written by this adapter)
.junie/agents/<name>.md          # one file per subagent
.junie/skills/<name>/SKILL.md    # one folder per skill, plus any bundled assets
.junie/commands/<name>.md        # one file per slash command
.junie/mcp/mcp.json              # when MCP entries exist
.junie/config.json               # when a settings spec selects a default model
.aiignore                        # when ignore entries exist
```

Junie's guidelines lookup is strict precedence, first match wins, not a merge (junie.jetbrains.com/docs/junie-ide-plugin.html and guidelines-and-memory.html, target-audit 2026-08-08, #552):

1. `.junie/AGENTS.md` ("the most preferred standard location").
2. The root `AGENTS.md`, combined with `.junie/playbook.md` and every `.junie/rules/*.md` file, "if no file is found in the `.junie` folder".
3. The legacy `.junie/guidelines.md` / `.junie/guidelines/`.

`sync` always writes `.junie/AGENTS.md`, so step 1 always matches. Step 2, including `.junie/playbook.md` and `.junie/rules/*.md`, is pre-empted outright in a synced project (junie.jetbrains.com/docs/environment-variables.html: "If this file exists, it is used exclusively; no other guidelines files are combined with it.").

Rule bodies therefore inline directly into `.junie/AGENTS.md`, under a sentinel-marked `## Rules` block immediately after the pointer body, using the same `### <name>` shape (source comment, optional description, full body) every other inlining target uses.

A prior version of this adapter instead flattened rules and agents to one `.md` file each under `.junie/rules/`. That directory is read at step 2 alongside `.junie/playbook.md`, but `.junie/AGENTS.md` always wins step 1 once `sync` has run, so a hand-authored file left there is shadowed rather than unread. Any agnostic-ai-managed leftovers there are swept on sync; hand-authored files survive.

Subagents and slash commands are both CLI-only surfaces (`junie-ide-plugin.html` mentions neither, confirmed by full-text search) that shipped after this adapter's original write-up and predate its own tracking issue: junie-cli-subagents.html has been live since 2026-03-10, custom-slash-commands.html since 2026-04-13 (target-audit 2026-08-11, #604 and #605).

Subagents emit one file per agent at `.junie/agents/<name>.md`: "Subagents are Markdown files with YAML metadata stored in the `.junie/agents/` or `.agents/` directory." This adapter defaults to `.junie/agents/`, the vendor's own preferred location (the same page says Junie CLI detects `.cursor/agents/`, `.claude/agents/`, and `.codex/agents/` on open and offers to import them specifically into `.junie/agents/`), rather than the shared `.agents/` tree several other targets already write skills, rules, commands, or an MCP file into. No registered target defaults an agent file into `.agents/` itself today, so there is nothing to dedupe with either way. Set `outputs.junie.agents-dir: .agents` for the shared alternative, the same pattern Codex uses for its own `outputs.codex.agents-dir: .agents/agents` community layout.

Agent names must match `[a-z][a-z0-9_-]*`: a lowercase letter first, then lowercase letters, digits, hyphens, or underscores. The same table states that rule for `name`, and "If missing, the file name (without extension) is used", so the filename carries it too. An invalid name such as `Deploy_Bot` fails sync with the name and required format; it is not renamed (#857). The rule is looser than the OpenCode and Zed skill rule, which forbids underscores.

Frontmatter passes through verbatim: the vendor's documented fields (`name`, `description`, `tools`, `disallowedTools`, `mcpServers`, `model`, `permissionMode`, `reasoningLevel`, `maxTurns`, `skills`, `allowPromptArgument`) are already spelled the way a spec author writes them, so nothing here is translated. `tools` and `disallowedTools` pass through as YAML lists (#604). Junie's built-in group labels match the Claude-style names for every group it documents, plus `AskUserQuestion`, but it documents fewer groups: no `WebFetch`, `Task`, `TodoWrite`, or `NotebookEdit`. `reasoningLevel` also accepts `effort` as an alias, taking precedence when both are set; a spec author can write either key and both pass through unchanged.

Agent bodies no longer inline into `.junie/AGENTS.md` now that this native destination exists, the same rule Augment and Kilo Code follow for their own native agents directories. `.junie/AGENTS.md` is fully regenerated from the canonical pointer body on every sync rather than patched in place, so a project still carrying the pre-#604 inlined `## Agents` block loses it on its very next sync with no extra sweep step. With `sync.target-overview` off, `.junie/AGENTS.md` and the shared root `AGENTS.md` render byte-identical content whenever another AGENTS.md-family target is also enabled.

One sub-feature on the subagents page, an auto model-selection policy toggle read from `/settings → Subagents`, is marked Early Access; the base file format and discovery are not caveated.

Slash commands emit one file per command at `.junie/commands/<name>.md`: "Project-specific commands are stored as Markdown files in the `.junie/commands` folder at your project's root directory." The vendor documents two frontmatter fields for commands, `description` and `allowPromptArgument` (accepts free-form text via a `$prompt` placeholder); every other key still passes through verbatim, same as agents. The body may reference `$argumentName` placeholders Junie substitutes at invocation.

Skills are unaffected by any of the above: they emit into their own native folder tree at `.junie/skills/<name>/SKILL.md` (Junie's Native Agent Skills feature, shipped 2026-07-31). Junie CLI also loads skills from `<projectRoot>/.agents/skills/` in a trusted project, "so skills shared through the cross-agent `.agents` convention are picked up as well" ([agent-skills.html](https://junie.jetbrains.com/docs/agent-skills.html)). Enabling junie alongside a target that writes the shared `.agents/skills/` tree therefore gives Junie two copies of the same skill; nothing is lost, the duplicate is the cost. A flat file never loads as a skill, and bundled sibling assets copy byte-for-byte. MCP servers use the standard `mcpServers` schema at `.junie/mcp/mcp.json` (`command`/`args`/`env` local, `url`/`headers` remote). Those are the only keys the vendor's structure block carries, so `disabled` and `description` are stripped with a coverage note instead of written ([junie-cli-mcp-configuration.html](https://junie.jetbrains.com/docs/junie-cli-mcp-configuration.html), target-audit 2026-09-18, #858). `disabled` would reverse meaning on arrival: "Manually added configurations are imported to the list of MCP servers and enabled by default." Disable a server with `/mcp` then **→ Disable**. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

The IDE plugin doc alone now adds a **Custom path** step ahead of `.junie/AGENTS.md`, read from Settings | Tools | Junie | Project Settings. The CLI-facing doc has no such step, and since that per-workspace IDE preference is not usually committed to the repo, it rarely changes which file wins in a synced project (target-audit 2026-08-09, #590).

A settings spec's default `model` merges into `.junie/config.json`, preserving unrelated native keys. An `x-junie` block on that spec merges into the same file, for the project-config keys this tool does not model.

Ignore specs emit as `.aiignore` in the project root: "You can restrict Junie from processing the contents of specific files or folders by creating and configuring an `.aiignore` file in the project root directory" and "The `.aiignore` file follows the same syntax and pattern format as the `.gitignore` file" ([junie-ide-plugin.html](https://junie.jetbrains.com/docs/junie-ide-plugin.html), target-audit 2026-09-11, #728).

The guarantee is softer than a block: Junie "will ask for explicit approval before viewing or editing" a listed file rather than refuse it. Only the contents are protected (file and folder names stay visible), and Brave Mode or an allowlisted command naming the path skips the prompt. This is also the one junie surface documented on the IDE-plugin page alone, with no Junie CLI page naming it.

Multiple specs concatenate. Override via `outputs.junie.ignore-file`.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.junie.agents-dir` | `.junie/agents` | |
| `outputs.junie.skills-dir` | `.junie/skills` | |
| `outputs.junie.commands-dir` | `.junie/commands` | |
| `outputs.junie.mcp-file` | `.junie/mcp/mcp.json` | |
| `outputs.junie.ignore-file` | `.aiignore` | |
| `outputs.junie.rules-dir` | `.junie/rules` | no longer controls where anything is written; only redirects the legacy-tree sweep above, for a project that customized it before this fix |

`.junie/AGENTS.md` is a fixed path, not configurable.

## Import

Skills import from `.junie/skills/` and `.agents/skills/`, with the native directory first for duplicate names. Each selected skill keeps its bundled assets.

`agnostic-ai import junie` reads the sentinel-marked Rules block in `.junie/AGENTS.md`, the file Junie's guidelines lookup opens first and `sync` always writes (#552). It reads agents from `.junie/agents/<name>.md` (or `.agents/<name>.md`) and commands from `.junie/commands/<name>.md`. Skills come from `.junie/skills/<name>/SKILL.md` folders, with bundled sibling assets copied byte-for-byte. The default `model` in `.junie/config.json` is restored to `settings/imported.yaml`.

Older layouts still import:

- A project synced between #552 and #604 has no native agent file yet. Import falls back to the pre-#604 sentinel-marked Agents block in `.junie/AGENTS.md`.
- A project synced before #552 still has content flattened under `.junie/rules/`. When that directory exists, it takes precedence over `.junie/AGENTS.md`, and each file is reclassified by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification).
- A legacy flat `.junie/rules/skill-<name>.md`, from a project synced before Native Agent Skills shipped, still imports as a skill.

## Verify

1. Install the Junie plugin in a JetBrains IDE (or the Junie CLI, [docs](https://junie.jetbrains.com/docs/)).
2. Check the tree: `ls AGENTS.md .junie/AGENTS.md .junie/agents/ .junie/skills/ .junie/commands/`, `grep "Generated by agnostic-ai" .junie/AGENTS.md`, `python -m json.tool .junie/mcp/mcp.json > /dev/null`.
3. Ask Junie to list its guidelines; the rule bodies inlined in `.junie/AGENTS.md` apply, each `.junie/agents/<name>.md` appears as a delegatable subagent, each `.junie/skills/<name>/` folder appears as a skill, and each `.junie/commands/<name>.md` appears as a `/name` slash command.
