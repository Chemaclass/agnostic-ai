+++
title = "Gemini CLI"
description = "How agnostic-ai emits Gemini CLI configuration: native paths, capability limits, and output options."
weight = 30

[extra]
group = "Reference"
target_id = "gemini"
+++

# Gemini CLI (`gemini`)

## Output

```
GEMINI.md                              # entry-point pointer body (written by sync)
.gemini/agents/<name>.md               # one subagent per agent
.gemini/commands/<name>.toml           # one per command
.gemini/skills/<name>/SKILL.md         # one folder per skill, bundled assets included
.gemini/commands/skill-<name>.toml     # extra command form, only when emit-skills-as-commands: true
.gemini/settings.json                  # when MCP, hook, or settings entries exist (merged with user config)
.geminiignore                          # when ignore entries exist
```

- **Rules**: unscoped rules inline into the root `GEMINI.md`. A rule with `scope: services/payments` reaches `services/payments/GEMINI.md` instead. A `globs` field alone does not create a directory scope. Remove legacy `outputs.gemini.rules-file` overrides before using scoped rules. See [scoped context](@/docs/scoped-context.md) for selector and runtime limits.
- **Agents**: one native [subagent](https://geminicli.com/docs/core/subagents.md) per agent at `.gemini/agents/<name>.md`, the project-level directory Gemini CLI scans. Subagents get automatic delegation, an isolated context window, `@name` invocation, and a `/agents` listing.

  Frontmatter carries the required `name` and `description` (falling back to the spec name), plus `kind`, `model`, `temperature`, `max_turns`, and `timeout_mins` when set. The body is the system prompt. Per-agent `mcpServers` has no agnostic-ai field, so set it through `x-gemini`.

  A generic `tools` list maps onto [Gemini's tool names](https://geminicli.com/docs/reference/tools): `Read` to `read_file`, `Write` to `write_file`, `Edit` to `replace`, `Glob` to `glob`, `Grep` to `grep_search`, `Bash` to `run_shell_command`, `WebFetch` to `web_fetch`, `WebSearch` to `google_web_search`. Other names are dropped with a coverage note, because an unknown entry would restrict the subagent to a tool that does not exist (no `tools` key inherits every tool). `x-gemini.tools` writes Gemini's own names directly, including the `*`, `mcp_*`, and `mcp_<server>_*` wildcards, and replaces the translated list.

  Set `outputs.gemini.emit-agents-as-commands: true` to also write each agent as a `<name>.toml` slash command, for projects that already type `/name`. With the key off, managed TOMLs from earlier syncs are swept.
- **Skills**: native [Agent Skills](https://geminicli.com/docs/cli/skills/) folders at `.gemini/skills/<name>/SKILL.md`, with bundled files copied byte-for-byte. Gemini CLI also reads `.agents/skills/`, which wins over `.gemini/skills/` for a skill with the same name; Gemini resolves that itself at session start. Set `outputs.gemini.emit-skills-as-commands: true` to also emit one `skill-<name>.toml` command per skill.
- **Commands**: one TOML per command at `.gemini/commands/<name>.toml`, the [project slash-command directory](https://geminicli.com/docs/cli/custom-commands.md). `description` maps to the TOML `description` and the body becomes `prompt`. A command and an agent can share a name.
- **Settings**: the portable `model` maps to `model.name` in `.gemini/settings.json`. Other model options and unrelated settings survive. `x-gemini` settings keys merge into the same file. Portable permission rules produce a coverage note. Import restores the default model.
- **MCP + hooks**: written to `.gemini/settings.json` (`mcpServers` and `hooks` maps). Streamable-HTTP servers (`type: http`) use `httpUrl` and SSE servers (`type: sse`) use `url`; the adapter picks the right one. Stdio servers also accept `cwd`.

  Every server also accepts `timeout` (milliseconds), `trust` (skip tool-call confirmations), `description`, `includeTools`, and `excludeTools`, all passed through verbatim ([configuration reference](https://geminicli.com/docs/reference/configuration.md)).

  Hooks route by `event`. Gemini CLI documents 11 events: `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`. Each definition has a `matcher` and a nested `hooks` array of `{type: "command", command}` handlers ([hook reference](https://geminicli.com/docs/hooks/reference/)).
  - Timeouts convert from seconds to milliseconds (vendor default 60000 ms).
  - A `command` list becomes separate handlers in one definition. `x-gemini.sequential: true` runs them in order.
  - `description` reaches each handler, `x-gemini.name` sets its display name, and `x-gemini.env` sets per-handler environment variables.
  - Existing user keys survive syncs.
- **Ignore**: ignore specs emit as `.geminiignore` (gitignore syntax), the file [Gemini CLI reads](https://geminicli.com/docs/cli/gemini-ignore/). Multiple specs concatenate. Override with `outputs.gemini.ignore-file`. Older versions wrote `.aiexclude`, which Gemini CLI never reads; sync removes a managed one.
- **Import**: `import gemini` reads `.gemini/agents/*.md` as agents, `.gemini/commands/*.toml` as commands, `.gemini/skills/<name>/` as skills, and `.gemini/settings.json` as MCP, hook, and default-model settings specs. A command's `prompt` becomes the body in either documented form (triple-quoted block or single-line string), and `description` stays in frontmatter.

  Command TOMLs import as commands, including agents emitted as commands by older versions and the mirrors that `emit-agents-as-commands` and `emit-skills-as-commands` write. A re-sync writes them back to the same path with identical bytes.

  Nested hooks keep commands, matchers, names, descriptions, environment maps, timeouts, and sequential groups. Distinct definitions with the same command and matcher import into separate files. A single handler's timeout imports in seconds when it is a whole second; otherwise `x-gemini.timeout` keeps the native milliseconds. Multiple handlers stay together under `x-gemini.hooks`, which replaces `command` emission for Gemini. Old flat hook files still import.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.gemini.agents-dir` | `.gemini/agents` | |
| `outputs.gemini.commands-dir` | `.gemini/commands` | |
| `outputs.gemini.skills-dir` | `.gemini/skills` | |
| `outputs.gemini.mcp-file` | `.gemini/settings.json` | also holds hooks and settings |
| `outputs.gemini.emit-skills-as-commands` | `false` | |
| `outputs.gemini.emit-agents-as-commands` | `false` | |
| `outputs.gemini.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |
| `outputs.gemini.ignore-file` | `.geminiignore` | |

## Verify

1. Install: `npm install -g @google/gemini-cli` ([docs](https://geminicli.com/docs/)).
2. Check the tree: `ls GEMINI.md .gemini/agents/ .gemini/commands/ .gemini/settings.json`, `head -2 .gemini/agents/*.md` (frontmatter first), `head -1 .gemini/commands/*.toml` for the provenance header, and `python -m json.tool .gemini/settings.json > /dev/null`.
3. `gemini --list-commands` parses every `<name>.toml` with no "invalid TOML" or "unknown field" errors, and `/agents` lists every `.gemini/agents/<name>.md`.
4. `gemini --list-mcp-servers` shows each `mcpServers.<name>` ready.
5. Perform a hook's matcher action (e.g. an `AfterTool`). The hook command runs.
