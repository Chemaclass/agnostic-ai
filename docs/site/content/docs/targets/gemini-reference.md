+++
title = "Gemini CLI native reference"
description = "Native field mappings, output overrides, and import coverage for gemini."
weight = 31

[extra]
nav_title = "Gemini CLI reference"
group = "Reference"
+++

# Gemini CLI native reference

Start with the [Gemini CLI setup](@/docs/targets/gemini.md). For home configuration and ownership, see [global configuration](@/docs/global-configuration.md).

## Rules

Rules without a directory scope go into the root `GEMINI.md`.

- `scope: services/payments` sends a rule to `services/payments/GEMINI.md` instead.
- `globs: tests/payments/**` also adds it to `tests/payments/GEMINI.md`.
- `globs` alone creates no directory scope.
- External file filters and root selectors cannot go in directory documents with a scope. They follow `on-unsupported`.
- Scoped rules need any legacy `outputs.gemini.rules-file` override removed.
- See [scoped context](@/docs/scoped-context.md) for selector and runtime limits.

## Agents

Native [subagents](https://geminicli.com/docs/core/subagents.md), with automatic delegation, their own context window, `@name` invocation, and a `/agents` listing. The body is the system prompt.

- Frontmatter: `name` and `description` (required; default is the spec name), plus `kind`, `model`, `temperature`, `max_turns`, `timeout_mins` when set.
- Portable `maxTurns` is written as `max_turns`. A `max_turns` the spec sets wins.
- Per-agent MCP servers: set `x-gemini.mcp_servers` with snake_case keys such as `http_url` and `include_tools`. Gemini rejects `mcpServers` and skips the agent, so sync renames `x-gemini.mcpServers` with a note.
- Omit `tools` to inherit all tools.
- `x-gemini.tools` replaces the mapped list with Gemini's own names, including the wildcards `*`, `mcp_*`, and `mcp_<server>_*`.
- `outputs.gemini.emit-agents-as-commands: true` also writes each agent as a `<name>.toml` command (`/name`). Off, sync removes earlier generated TOML files.

The generic `tools` list maps to [Gemini's tool names](https://geminicli.com/docs/reference/tools). Other names drop with a sync note, since an unknown entry would limit the subagent to a tool that does not exist.

| Generic | Gemini |
|---|---|
| `Read` | `read_file` |
| `Write` | `write_file` |
| `Edit` | `replace` |
| `Glob` | `glob` |
| `Grep` | `grep_search` |
| `Bash` | `run_shell_command` |
| `WebFetch` | `web_fetch` |
| `WebSearch` | `google_web_search` |

## Skills

Native [Agent Skills](https://geminicli.com/docs/cli/skills/) at `.gemini/skills/<name>/SKILL.md`. Bundled files are copied as they are.

- Gemini CLI also reads `.agents/skills/`. It wins when a name is in both.
- `outputs.gemini.emit-skills-as-commands: true` adds one `skill-<name>.toml` command per skill.

## Commands

`.gemini/commands/<name>.toml`, the [project slash-command directory](https://geminicli.com/docs/cli/custom-commands.md). `description` becomes the TOML `description`. The body becomes `prompt`.

- A command and an agent can share a name.
- `git:commit` is written as `.gemini/commands/git/commit.toml` and runs as `/git:commit`.
- An agent is one file, `.gemini/agents/<name>.md`. On Windows, sync refuses an agent named with a colon ([path semantics](@/docs/configuration.md#path-semantics)).

## Settings

Portable `model` maps to `model.name`. Other model options and settings stay. `x-gemini` keys merge in. Portable permission rules raise a sync note.

At user scope, portable [voice settings](@/docs/spec-format/settings-reference.md#voice-dictation) write the experimental `experimental.voiceMode` and `experimental.voice.activationMode` keys. Gemini has no dictation language key.

## MCP

The `mcpServers` map.

| Spec | Written as |
|---|---|
| `type: http` (streamable HTTP) | `httpUrl` |
| `type: sse` | `url` |
| Stdio | Also accepts `cwd` |

- `timeout` (milliseconds), `trust` (skip tool-call confirmations), `description`, `includeTools`, and `excludeTools` are copied as written ([configuration reference](https://geminicli.com/docs/reference/configuration.md)).
- Import reads `httpUrl` back as `type: http` and a bare `url` as `type: sse`.
- OAuth options pass through `x-gemini.oauth` (`enabled`, `allowPrivateNetwork`, `authorizationResponseIssParameterSupported`, and others).
- Import keeps that object Gemini-specific, skips servers with literal OAuth credentials, and leaves credential references as references.

## Hooks

The `hooks` map, keyed by event. Each hook has a `matcher` and a nested `hooks` array of `{type: "command", command}` handlers ([hook reference](https://geminicli.com/docs/hooks/reference/)). With `builtins: [memory]`, a `SessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session.

Gemini documents 11 events: `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`.

- Timeouts convert from seconds to milliseconds (vendor default 60000 ms).
- A `command` list becomes separate handlers in one hook. `x-gemini.sequential: true` runs them in order.
- `description` is copied to each handler. `x-gemini.name` sets the display name, `x-gemini.env` the environment. Your keys stay.
- Gemini runs `command` through `bash -c` (PowerShell on Windows) and has no `args`. Sync folds `args` into `command`, single-quoted (`node 'guard.js'`). `import gemini` reads it back as one shell-form `command`.
- [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests a `BeforeTool`, `AfterTool`, `BeforeAgent`, or `SessionStart` hook before a session.

{% <details summary="Folded args edge cases"> %}
- PowerShell reads folded args the same way unless one holds an apostrophe.
- A quoted command path with a space runs in bash but not PowerShell. Use a shell-form `command` there.
- Gemini replaces `$GEMINI_*` path variables and `$CLAUDE_PROJECT_DIR` with its own quoted path, even inside those quotes. Such an argument splits when the project path has a space.
{% </details> %}

## Ignore

Ignore specs are joined into `.geminiignore` (gitignore syntax), the file [Gemini CLI reads](https://geminicli.com/docs/cli/gemini-ignore/). Override with `outputs.gemini.ignore-file`.

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
| `outputs.gemini.rules-file` | unset | writes legacy joined rules and skips instructions that point to source specs |
| `outputs.gemini.ignore-file` | `.geminiignore` | |

## Import

`import gemini` reads `.gemini/agents/*.md`, `.gemini/commands/**/*.toml`, `.gemini/skills/<name>/`, and the MCP, hooks, and default model in `.gemini/settings.json`.

- **Instructions**: the root `GEMINI.md` goes to `.agnostic-ai/AGNOSTIC_AI.md`. Only the rules block that `sync` appends becomes rules.
- **Nested files**: a hand-written `<dir>/GEMINI.md` becomes one rule with the whole file and `scope: <dir>`. It is named after the scope: `api.md` for `services/api/`, or `services-api.md` when another scope also ends in `api`.
- A directory with one rule syncs back unchanged. A nested file `sync` wrote splits back into its rules.
- **Commands**: `prompt` becomes the body, in either documented form (triple-quoted block or single-line string). `description` stays in frontmatter.
- A file in a subfolder, such as `git/commit.toml`, imports as the command `git:commit`.
- Older agent-as-command files and the `emit-*-as-commands` copies import too, and sync back unchanged.
- **Hooks**: nested hooks keep commands, matchers, names, descriptions, environment maps, timeouts, and sequential groups. Hooks with the same command and matcher import separately.
- A single handler's whole-second timeout imports in seconds. Otherwise `x-gemini.timeout` keeps the native milliseconds.
- Multiple handlers stay together under `x-gemini.hooks`, which replaces `command` for Gemini.
- Old flat hook files still import.
