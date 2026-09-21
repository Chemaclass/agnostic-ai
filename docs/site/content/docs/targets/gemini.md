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
GEMINI.md                              # canonical entry-point pointer body (written by sync)
.gemini/agents/<name>.md               # one per agent, Gemini's native subagent surface
.gemini/commands/<name>.toml           # one per command
.gemini/skills/<name>/SKILL.md         # one folder per skill, bundled assets included
.gemini/commands/skill-<name>.toml     # additional command form, only when emit-skills-as-commands: true
.gemini/settings.json                  # when MCP, hook, or settings entries exist (merged with existing user config)
.geminiignore                          # when ignore entries exist
```

- **Rules**: unscoped rules inline into the root `GEMINI.md`. A rule with `scope: services/payments` instead reaches `services/payments/GEMINI.md`. A `globs` field alone does not create a directory scope. Remove legacy `outputs.gemini.rules-file` overrides before using scoped rules. See [scoped context](@/docs/scoped-context.md) for selector and runtime limits.
- **Agents**: one native subagent per agent at `.gemini/agents/<name>.md`. [Gemini CLI's subagents doc](https://geminicli.com/docs/core/subagents.md) states "Custom agents are defined as Markdown files (`.md`) with YAML frontmatter ... Project-level: `.gemini/agents/*.md` (Shared with your team)", cross-confirmed by `/agents reload`, which "Rescans agent directories (`~/.gemini/agents` and `.gemini/agents`)". That surface buys automatic delegation, an isolated context window, `@name` invocation, and the `/agents` listing.

  Until this release, agents emitted as a slash-command TOML instead. An agent was a prompt the user had to type rather than a subagent Gemini could delegate to, and it shared a directory and a `<name>.toml` filename with commands, so a same-named agent and command overwrote each other (target-audit 2026-09-11, #733).

  Frontmatter carries the two required fields, `name` and `description` (falling back to the spec name), plus `kind`, `model`, `temperature`, `max_turns`, and `timeout_mins` when declared; the body is the system prompt. `mcpServers` (inline per-agent MCP servers) is documented too and has no agnostic-ai field, so it reaches the file through `x-gemini`.

  A spec's generic `tools` list translates onto [Gemini's own tool names](https://geminicli.com/docs/reference/tools): `Read` to `read_file`, `Write` to `write_file`, `Edit` to `replace`, `Glob` to `glob`, `Grep` to `grep_search`, `Bash` to `run_shell_command`, `WebFetch` to `web_fetch`, `WebSearch` to `google_web_search`. A name outside that set is dropped with a coverage note rather than written unconfirmed: an unknown entry here restricts the subagent to a tool that does not exist, while an absent `tools` key inherits every tool from the parent session. Set `x-gemini.tools` to write Gemini's own vocabulary directly, including the documented `*`, `mcp_*`, and `mcp_<server>_*` wildcards; that override wins outright over the translated form.

  Set `outputs.gemini.emit-agents-as-commands: true` to also keep writing the old `<name>.toml` slash command, for a project that already types `/name`. With the key off, a managed TOML an earlier sync left there is swept.
- **Skills**: native [Agent Skills](https://geminicli.com/docs/cli/skills/) folders under `.gemini/skills/<name>/SKILL.md` (the workspace tier Gemini CLI scans). It also reads the cross-tool `.agents/skills/` alias, which takes precedence over `.gemini/skills/` within the same tier when a skill shares a name in both (target-audit 2026-08-08, #563). Gemini CLI resolves that conflict itself at session start, so no sync-time detection is needed here. Bundled sibling files propagate byte-for-byte. Set `outputs.gemini.emit-skills-as-commands: true` to additionally emit one `skill-<name>.toml` command per skill.
- **Commands**: one TOML per command spec under `.gemini/commands/<name>.toml`, [the directory Gemini reads project slash commands from](https://geminicli.com/docs/cli/custom-commands.md). `description` frontmatter maps to the TOML `description`; the body becomes the `prompt`. Agents left this directory in #733, so a command and an agent may now share a name without overwriting each other.
- **Settings**: the portable `model` maps to `model.name` in `.gemini/settings.json`. Sibling model options and unrelated settings survive. `x-gemini` settings keys merge into the same file; portable permission rules produce a coverage note. Import restores the default model.
- **MCP + hooks**: written into `.gemini/settings.json` (`mcpServers` map, `hooks` map). Gemini keys the endpoint by transport: streamable-HTTP servers (`type: http`) use `httpUrl`, SSE servers (`type: sse`) use `url`. The adapter routes each automatically. Stdio servers also accept `cwd` (working directory), the same cross-tool field Codex reads.

  Every server, any transport, also accepts `timeout` (milliseconds), `trust` (bypass tool-call confirmations), `description`, `includeTools`, and `excludeTools`; all five pass through verbatim ([geminicli.com/docs/reference/configuration](https://geminicli.com/docs/reference/configuration.md), #661).

  Hooks route by `event` frontmatter. Gemini CLI documents 11 events: `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`. Each definition contains `matcher` and a nested `hooks` array of `{type: "command", command}` handlers, as required by the [hook reference](https://geminicli.com/docs/hooks/reference/). Portable hook timeouts convert from seconds to milliseconds; the vendor default is 60000 ms. A `command` list becomes separate handlers kept in one definition. Set `x-gemini.sequential: true` to run them in order. `description` reaches each handler, `x-gemini.name` sets its display name, and `x-gemini.env` supplies per-handler environment variables. Pre-existing user keys survive syncs.
- **Ignore**: ignore specs emit as `.geminiignore` (gitignore syntax), the file [Gemini CLI reads](https://geminicli.com/docs/cli/gemini-ignore/). Multiple specs concatenate. Override via `outputs.gemini.ignore-file`. Up to v0.49 this wrote `.aiexclude`, which belongs to Gemini Code Assist and Gemini CLI never opens; a managed `.aiexclude` is removed on the next sync. (#625)
- **Import**: `import gemini` reads every native directory on its own pass. `.gemini/agents/*.md` becomes agent specs, `.gemini/commands/*.toml` becomes command specs, `.gemini/skills/<name>/` becomes skill folders, and `.gemini/settings.json` becomes MCP, hook, and default-model settings specs. A command's `prompt` becomes the spec body in either form Gemini documents, the triple-quoted block or the single-line string, and `description` stays in frontmatter.

  A project synced before #733 emitted its agents as command TOMLs; those import as commands, since `.gemini/commands/` is the slash-command directory, and a re-sync writes them back to the same path. The mirror TOMLs `emit-agents-as-commands` and `emit-skills-as-commands` write import as command specs too, for the same reason; the emitted bytes stay identical across the round-trip (#750).

  Native nested hooks preserve commands, matchers, names, descriptions, environment maps, timeouts, and sequential groups. Distinct definitions with the same command and matcher import into separate files. A single handler's timeout imports in seconds when it is a whole second; otherwise `x-gemini.timeout` keeps the native milliseconds. Multiple handlers stay together under `x-gemini.hooks`, which replaces `command` emission for Gemini; old flat hook files still import (#762).

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
2. Check the tree: `ls GEMINI.md .gemini/agents/ .gemini/commands/ .gemini/settings.json`, `head -2 .gemini/agents/*.md` (frontmatter first), `head -1 .gemini/commands/*.toml` for the provenance header, `python -m json.tool .gemini/settings.json > /dev/null`.
3. `gemini --list-commands` parses every `<name>.toml` with no "invalid TOML" / "unknown field" errors, and `/agents` lists every `.gemini/agents/<name>.md` as delegatable.
4. `gemini --list-mcp-servers` shows each `mcpServers.<name>` ready.
5. Trigger a hook by performing the matcher action (e.g. an `AfterTool`); the hook command runs.
