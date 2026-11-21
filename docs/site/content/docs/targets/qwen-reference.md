+++
title = "Qwen Code native reference"
description = "Native field mappings, global configuration, config keys, and import coverage for qwen."
weight = 33

[extra]
nav_title = "Qwen Code reference"
group = "Reference"
+++

# Qwen Code native reference

Start with the [Qwen Code setup](@/docs/targets/qwen.md).

## Native mappings

### Rules

- Project-wide rules go into `QWEN.md`. `scope` rules go into `<scope>/QWEN.md`.
- A rule with `globs` or `paths` and no `scope` goes to `.qwen/rules/<name>.md` with a `paths:` list. Qwen leaves it out of the prompt until a tool reads or edits a matching file.
- `alwaysApply: true` keeps a rule in `QWEN.md`.
- See [scoped context](@/docs/scoped-context.md#qwen-code-rules-with-globs-alone) for the limits.

### Agents

- Agents get YAML frontmatter and a prompt body.
- Portable tool names map to Qwen names. Use `x-qwen.tools` for an explicit native list.
- `readonly: true` gives `disallowedTools: [write_file, edit, notebook_edit]`. The agent cannot write files, but shell stays allowed. An explicit `x-qwen.disallowedTools` wins.
- Import reads that exact list back as `readonly: true` and keeps any other blocklist under `x-qwen`.
- Native fields such as `approvalMode` and record-valued `mcpServers` belong under `x-qwen`.

### Skills

- Bundled files are kept.
- Qwen Code also reads `.agents/skills/`. Its own `.qwen/skills/` wins a same-name clash.
- `disable-model-invocation`, `user-invocable`, and `paths` (always a list) are written to `SKILL.md` as Qwen reads them, unless another target writes the same skills folder.
- `allowed-tools` is not written, since Qwen's own `allowedTools` grants permissions.
- Set `x-qwen.model` on a skill to select its native model. The shared skill `model` field is not promoted.

### Commands

- Markdown with optional `description` frontmatter.
- Qwen interprets `{% raw %}{{args}}{% endraw %}`, `!{shell}`, and `@{path}` in bodies. Sync keeps that text as written.
- Nested command paths become colon-separated names. A command named `git:commit` is written as `.qwen/commands/git/commit.md`, on Windows too.
- On Windows, a target that writes such a name as one file refuses it. See [path semantics](@/docs/configuration.md#path-semantics).

### MCP

Servers share `settings.json` under `mcpServers`. See the [native MCP reference](https://qwenlm.github.io/qwen-code-docs/en/users/features/mcp/).

- Stdio: `command`, `args`, `env`, optional `cwd`. HTTP: `httpUrl`. SSE: `url`.
- MCP timeouts are milliseconds.
- `disabled: true` servers are omitted, because Qwen ignores that per-server field.
- A same-name server in existing or inherited configuration can still load. Use native `mcp.excluded` to block it.
- Environment placeholders stay literal until Qwen resolves them.

### Hooks

Hooks share `settings.json` under `hooks`.

- Portable `on`, `match`, and `decision: stdout` map to Qwen events, tool names, and blocking decisions.
- Spec timeouts are seconds. Qwen reads a command hook timeout of 1000 or more as milliseconds. So sync multiplies a spec timeout of 1000 seconds or more by 1000 (`1200` becomes `1200000`), capped at 2147483647 ms (about 24.8 days, the longest delay Qwen can time). Lower values stay as they are.
- `x-qwen.timeout` and raw native handler groups are written as given, in Qwen's units.
- Sync quotes portable arguments into the shell command, because Qwen has no `args` field.
- Portable hooks that exit 2 block pre-tool, prompt-submit, and stop events. After-tool and after-edit stop further execution. Session-start and session-end keep their native behavior.

Native `FileChanged` hooks (Qwen Code 0.27.0):

- They watch the literal filenames in `matcher`, separated by `|`, in the working folder.
- They run only in local, non-managed sessions.
- A pattern only filters paths that are already watched. Add more through `watchPaths`.
- A `SessionStart` or `FileChanged` hook that runs synchronously and succeeds can return `hookSpecificOutput.watchPaths`, with its own event name in `hookEventName`.
- A list of absolute paths replaces the earlier list. An empty list clears it.
- Filenames named in `matcher` stay watched either way.
- Project import keeps native command, HTTP, and prompt handlers and their metadata.

### Settings

See [Qwen settings](https://qwenlm.github.io/qwen-code-docs/en/users/configuration/settings/).

- Portable `model` becomes `model.name`. Permission lists become `permissions.allow`, `permissions.ask`, `permissions.deny`.
- Settings `effort` becomes `model.reasoningEffort` (Qwen Code 0.28.0). It accepts `low`, `medium`, `high`, `xhigh`, and `max`; each provider maps the value to what the active model supports.
- `x-qwen.model.reasoningEffort` overrides the portable value.
- `x-qwen` keys keep their native spelling.
- Native `x-qwen.mcpServers` records override portable servers with the same name.
- Native and portable hook entries are combined.
- Sync keeps unrelated settings, including the active `disableAllHooks`.
- Legacy `hooks.enabled`, `hooks.disabled`, and `hooks.notifications` metadata is kept but does not disable hooks in this release.

## Global configuration

- Portable [voice settings](@/docs/spec-format/settings-reference.md#voice-dictation) write `general.voice.enabled`, `general.voice.mode`, and `general.voice.language`. Choose a native `voiceModel` before using dictation.
- `agnostic-ai sync --scope user -t qwen` writes instructions, agents, skills, MCP, hooks, and settings under `~/.qwen/`.
- `approvals` set to `review` for Qwen Code writes `tools.approvalMode: "auto"` to `~/.qwen/settings.json`, unless a settings spec sets `x-qwen.tools.approvalMode`. See [review through the tool's own reviewer](@/docs/global-configuration-reference.md#global-approvals-review).
- `approvals` set to `yolo`, with the `approvals` built-in in the home config, makes `sync --scope user` write a `PermissionRequest` hook to `~/.qwen/settings.json` that approves safe shell commands, reads, edits and writes. Any `permissions.ask` rule turns it off. See [Approvals](@/docs/approvals.md).
- `QWEN_HOME` replaces that whole directory. `QWEN_RUNTIME_DIR` moves runtime state, not configuration.
- `agnostic-ai import --scope user qwen` reads the user model, effort, permission lists, and MCP servers.
- See [global configuration](@/docs/global-configuration.md).

## Config keys

| Key | Default |
|---|---|
| `outputs.qwen.rules-file` | unset, uses instruction files |
| `outputs.qwen.rules-dir` | `.qwen/rules` |
| `outputs.qwen.agents-dir` | `.qwen/agents` |
| `outputs.qwen.skills-dir` | `.qwen/skills` |
| `outputs.qwen.commands-dir` | `.qwen/commands` |
| `outputs.qwen.conf-file` | `.qwen/settings.json` |
| `outputs.qwen.mcp-file` | `.qwen/settings.json` |
| `outputs.qwen.hooks-file` | `.qwen/settings.json` |
| `outputs.qwen.ignore-file` | `.qwenignore` |

Root instructions use the shared [entry-point configuration](@/docs/configuration-reference.md#outputs). Changing a path does not change where Qwen looks. Keep overrides aligned with your native configuration.

## Import

```bash
agnostic-ai import qwen
agnostic-ai lint
agnostic-ai sync -t qwen
agnostic-ai sync --check -t qwen
```

Import reads:

- Root and scoped instructions.
- Rule files directly under `.qwen/rules/`.
- Agents.
- Skill folders and resources from `.qwen/skills/` and `.agents/skills/` (`.qwen/skills/` wins a same-name clash).
- Markdown and legacy TOML commands. Imported legacy commands emit as Markdown.
- `.qwenignore`.
- Settings, hooks, and MCP servers in `.qwen/settings.json`.

Details:

- Supported settings effort becomes portable `effort`. A value shadowed by another settings spec stays under `x-qwen.model.reasoningEffort`.
- A rule file's `paths:` becomes portable `globs`.
- A rule file without `paths:` is named by import. Sync moves that rule into `QWEN.md`, so delete the original file.
- Rule files in subfolders are named and skipped. Import stops when a rule file shares its name with a rule in `QWEN.md`.
- Native fields stay under `x-qwen`. HTTP `httpUrl` becomes `type: http`. SSE `url` becomes `type: sse`.
- Import rejects credentials in retained agent and skill headers, hooks, and settings.
- Settings can refer to environment variables with `$NAME` or `${NAME}`. Qwen does not expand those in an agent's or skill's credential fields, so import rejects them there.
