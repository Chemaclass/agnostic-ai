+++
title = "Gemini CLI"
description = "What agnostic-ai writes for Gemini CLI: file paths, what Gemini CLI cannot do, and output options."
weight = 30

[extra]
group = "Reference"
target_id = "gemini"
+++

# Gemini CLI (`gemini`)

agnostic-ai writes `GEMINI.md`, native subagents, commands, and skills under `.gemini/`, and merges MCP, hooks, and settings into `.gemini/settings.json`.

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

- **Rules**: unscoped rules go into the root `GEMINI.md`. `scope: services/payments` sends a rule to `services/payments/GEMINI.md` instead, and `globs: tests/payments/**` adds it to `tests/payments/GEMINI.md`. `globs` alone creates no directory scope. Directory documents cannot hold external file filters or root selectors with a scope, so those follow `on-unsupported`. Scoped rules need any legacy `outputs.gemini.rules-file` override removed. See [scoped context](@/docs/scoped-context.md) for selector and runtime limits.
- **Agents**: native [subagents](https://geminicli.com/docs/core/subagents.md) in the project directory Gemini CLI scans. They get automatic delegation, their own context window, `@name` invocation, and a `/agents` listing. Frontmatter carries the required `name` and `description` (default: spec name), plus `kind`, `model`, `temperature`, `max_turns`, and `timeout_mins` when set. The body is the system prompt.
  - Per-agent MCP servers have no agnostic-ai field. Set `x-gemini.mcp_servers`, with snake_case keys such as `http_url` and `include_tools`. Gemini rejects the documented `mcpServers` key and skips the agent, so sync renames `x-gemini.mcpServers` to `mcp_servers` with a note.
  - A generic `tools` list maps to [Gemini's tool names](https://geminicli.com/docs/reference/tools): `Read`: `read_file`, `Write`: `write_file`, `Edit`: `replace`, `Glob`: `glob`, `Grep`: `grep_search`, `Bash`: `run_shell_command`, `WebFetch`: `web_fetch`, `WebSearch`: `google_web_search`. Other names drop with a coverage note, because an unknown entry would limit the subagent to a tool that does not exist. Omit `tools` to inherit all tools.
  - `x-gemini.tools` replaces the translated list with Gemini's own names, including the wildcards `*`, `mcp_*`, and `mcp_<server>_*`.
  - `outputs.gemini.emit-agents-as-commands: true` also writes each agent as a `<name>.toml` slash command, so you can type `/name`. With it off, sync removes earlier generated TOML files.
- **Skills**: native [Agent Skills](https://geminicli.com/docs/cli/skills/) at `.gemini/skills/<name>/SKILL.md`, with bundled files copied as they are. Gemini CLI also reads `.agents/skills/`, which wins when a name appears in both. `outputs.gemini.emit-skills-as-commands: true` adds one `skill-<name>.toml` command per skill.
- **Commands**: `.gemini/commands/<name>.toml`, the [project slash-command directory](https://geminicli.com/docs/cli/custom-commands.md). `description` becomes the TOML `description` and the body becomes `prompt`. A command and an agent can share a name.
- **Settings**: the portable `model` maps to `model.name`. Other model options and settings stay. `x-gemini` keys merge in. Portable permission rules raise a coverage note.
- **MCP**: the `mcpServers` map. Streamable-HTTP servers (`type: http`) use `httpUrl`. SSE servers (`type: sse`) use `url`. Stdio servers also accept `cwd`. `timeout` (milliseconds), `trust` (skip tool-call confirmations), `description`, `includeTools`, and `excludeTools` are copied as written ([configuration reference](https://geminicli.com/docs/reference/configuration.md)). Import reads `httpUrl` back as `type: http` and a bare `url` as `type: sse`.
- **Hooks**: the `hooks` map, keyed by event. Gemini documents 11 events: `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`. Each hook has a `matcher` and a nested `hooks` array of `{type: "command", command}` handlers ([hook reference](https://geminicli.com/docs/hooks/reference/)). With `builtins: [memory, memory-hook]`, a `SessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session.
  - Timeouts convert from seconds to milliseconds (vendor default 60000 ms).
  - A `command` list becomes separate handlers in one hook. `x-gemini.sequential: true` runs them in order.
  - `description` is copied to each handler, `x-gemini.name` sets its display name, and `x-gemini.env` sets its environment variables. Your existing keys stay.
  - Gemini runs `command` through `bash -c` (PowerShell on Windows) and has no `args` field. Sync folds `args` into `command`, each single-quoted (`node 'guard.js'`). `import gemini` reads it back as one shell-form `command`.
  - [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs a `BeforeTool`, `AfterTool`, `BeforeAgent`, or `SessionStart` hook the same way, so you can test it before a session.
- **Ignore**: Ignore specs are joined into `.geminiignore` (gitignore syntax), the file [Gemini CLI reads](https://geminicli.com/docs/cli/gemini-ignore/). Override with `outputs.gemini.ignore-file`.

{% <details summary="Folded args edge cases"> %}
PowerShell reads folded args the same way unless one holds an apostrophe. A quoted command path with a space runs in bash but not PowerShell, so use a shell-form `command` there. Gemini replaces `$GEMINI_*` path variables and `$CLAUDE_PROJECT_DIR` with its own quoted path, even inside those quotes. Such an argument splits when the project path has a space.
{% </details> %}

{% <details summary="Old .aiexclude files"> %}
Older versions wrote `.aiexclude`, which Gemini CLI never reads. Sync removes a generated one.
{% </details> %}

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.gemini.agents-dir` | `.gemini/agents` | |
| `outputs.gemini.commands-dir` | `.gemini/commands` | |
| `outputs.gemini.skills-dir` | `.gemini/skills` | |
| `outputs.gemini.mcp-file` | `.gemini/settings.json` | also holds hooks and settings |
| `outputs.gemini.emit-skills-as-commands` | `false` | |
| `outputs.gemini.emit-agents-as-commands` | `false` | |
| `outputs.gemini.rules-file` | unset | writes legacy joined rules and skips the pointer-body write |
| `outputs.gemini.ignore-file` | `.geminiignore` | |

## Import

`import gemini` reads `.gemini/agents/*.md`, `.gemini/commands/*.toml`, `.gemini/skills/<name>/`, and the MCP, hooks, and default model in `.gemini/settings.json`.

- **Instructions**: the root `GEMINI.md` goes to `.agnostic-ai/AGNOSTIC_AI.md`. Only the rules block that `sync` appends becomes rules.
- **Nested files**: a hand-written `<dir>/GEMINI.md` becomes one rule with the whole file and `scope: <dir>`, named after the scope: `api.md` for `services/api/`, or `services-api.md` when another scope also ends in `api`. A directory with one rule syncs back unchanged. A nested file that `sync` wrote splits back into its rules.
- **Commands**: `prompt` becomes the body in either documented form (triple-quoted block or single-line string). `description` stays in frontmatter. This also covers older agent-as-command files and the `emit-agents-as-commands` and `emit-skills-as-commands` copies, which sync back unchanged.
- **Hooks**: nested hooks keep commands, matchers, names, descriptions, environment maps, timeouts, and sequential groups. Hooks with the same command and matcher import separately. A single handler's whole-second timeout imports in seconds. Otherwise `x-gemini.timeout` keeps the native milliseconds. Multiple handlers stay together under `x-gemini.hooks`, which replaces `command` for Gemini. Old flat hook files still import.

## Protected paths

Enforced (hook). Gemini CLI's policy engine can deny `write_file` and `replace`, but its workspace tier "is currently non-functional" ([policy engine](https://geminicli.com/docs/reference/policy-engine/)). So sync writes `.gemini/hooks/agnostic-ai-protect.sh` and a `BeforeTool` hook on `^(write_file|replace)$` in `.gemini/settings.json`. For a protected `tool_input.file_path`, the script exits 2 with the reason on stderr. Gemini CLI then blocks the call and shows the reason ([hooks reference](https://geminicli.com/docs/hooks/reference/)). The script needs only `sh` and `awk`.

`decision: ask` also blocks and tells the agent to ask. Gemini CLI runs project hooks only in a trusted folder and reads `.gemini/settings.json` from the directory you start in, so start at the project root. The hook does not catch shell commands that write files.

{% <details summary="Script failures and odd paths"> %}
Gemini CLI lets an edit through on exit 1 or a timeout, so the script blocks when it cannot run: no `awk`, an unreadable project root, or an `awk` failure. It ignores case, reads `\` as a path separator, and checks a path with a leading `@` both ways, because Gemini CLI strips the `@`. `replace` searches the workspace for a relative path that is missing from the project root, so the hook blocks that call and asks for the full path. On Windows it needs `sh` on `PATH` (Git for Windows has one), or the edit goes through.
{% </details> %}

See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

## Verify

1. Install: `npm install -g @google/gemini-cli` ([docs](https://geminicli.com/docs/)).
2. Check the tree with `ls GEMINI.md .gemini/agents/ .gemini/commands/ .gemini/settings.json` and `head -2 .gemini/agents/*.md` (frontmatter first). Check the generated header with `head -1 .gemini/commands/*.toml`. Validate JSON with `python -m json.tool .gemini/settings.json > /dev/null`.
3. `gemini --list-commands` parses every `<name>.toml` with no "invalid TOML" or "unknown field" errors, and `/agents` lists every `.gemini/agents/<name>.md`.
4. `gemini --list-mcp-servers` shows each `mcpServers.<name>` ready.
5. Trigger a hook, such as an `AfterTool`, and confirm it runs.
