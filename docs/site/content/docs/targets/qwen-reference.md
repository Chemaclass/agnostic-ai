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

Project-wide rules go into `QWEN.md`, and `scope` rules go into `<scope>/QWEN.md`. A rule with `globs` or `paths` and no `scope` goes to `.qwen/rules/<name>.md` with a `paths:` list. Qwen leaves that rule out of the prompt until a tool reads or edits a matching file. `alwaysApply: true` keeps a rule in `QWEN.md`. See [scoped context](@/docs/scoped-context.md#qwen-code-rules-with-globs-alone) for the limits.

Agents get YAML frontmatter and a prompt body. Portable tool names map to Qwen names; use `x-qwen.tools` for an explicit native list. An agent with `readonly: true` gets `disallowedTools: [write_file, edit, notebook_edit]`, so it cannot write files; shell stays allowed, and an explicit `x-qwen.disallowedTools` wins. Import reads that exact list back as `readonly: true` and keeps any other blocklist under `x-qwen`. Native agent fields such as `approvalMode` and record-valued `mcpServers` belong under `x-qwen`.

Skills keep their bundled files. Qwen Code also reads `.agents/skills/`, and its own `.qwen/skills/` wins a same-name clash. `disable-model-invocation`, `user-invocable`, and `paths` (always as a list) are written to `SKILL.md` as Qwen reads them, unless another target writes the same skills folder; `allowed-tools` is not, since Qwen's own `allowedTools` grants permissions. Set `x-qwen.model` on a skill to select its native model; the shared skill `model` field is not promoted.

Commands use Markdown with optional `description` frontmatter. Qwen interprets `{% raw %}{{args}}{% endraw %}`, `!{shell}`, and `@{path}` in command bodies; sync keeps that text as written. Nested command paths become colon-separated command names, and sync writes a command named `git:commit` back as `.qwen/commands/git/commit.md`, on Windows too. On Windows, a target that writes such a name as one file refuses it; see [path semantics](@/docs/configuration.md#path-semantics).

MCP servers share `settings.json` under `mcpServers`. Stdio uses `command`, `args`, `env`, and optional `cwd`; HTTP uses `httpUrl`, and SSE uses `url`. MCP timeouts are milliseconds. Servers marked `disabled: true` are omitted from this output because Qwen ignores that per-server field. A same-name server in existing or inherited configuration can still load; use native `mcp.excluded` to block it. Environment placeholders stay literal until Qwen resolves them. See the [native MCP reference](https://qwenlm.github.io/qwen-code-docs/en/users/features/mcp/).

Hooks share `settings.json` under `hooks`. Portable `on`, `match`, and `decision: stdout` map to Qwen events, tool names, and blocking decisions. Command hook timeouts are seconds in the spec. Qwen reads a command hook timeout of 1000 or more as milliseconds, so sync writes a spec timeout of 1000 seconds or more times 1000 (`1200` becomes `1200000`), capped at 2147483647 milliseconds (about 24.8 days), the longest delay Qwen can time, and leaves lower values as they are. `x-qwen.timeout` and raw native handler groups are written as given, in Qwen's own units. Sync quotes portable arguments into the shell command because Qwen has no separate `args` field. Portable hooks that exit 2 block pre-tool, prompt-submit, and stop events; after-tool and after-edit stop further execution. Session-start and session-end keep their native lifecycle behavior.

Native `FileChanged` hooks (Qwen Code 0.27.0) watch the literal filenames in `matcher`, separated by `|`, in the working folder. They run only in local, non-managed sessions. A pattern only filters paths that are already watched; add more through `watchPaths`. A `SessionStart` or `FileChanged` hook that runs synchronously and succeeds can return `hookSpecificOutput.watchPaths`, with its own event name in `hookEventName`. A list of absolute paths replaces the earlier list, and an empty list clears it. Filenames named in `matcher` stay watched either way. Project import preserves native command, HTTP, and prompt handlers and their metadata.

Settings map portable `model` to `model.name` and permission lists to `permissions.allow`, `permissions.ask`, and `permissions.deny`. Keys under `x-qwen` keep their native spelling. Native `x-qwen.mcpServers` records override portable servers with the same name. Native and portable hook entries are combined. Sync preserves unrelated settings, including the active `disableAllHooks` setting. Legacy `hooks.enabled`, `hooks.disabled`, and `hooks.notifications` metadata is preserved but does not disable hooks in this release. See [Qwen settings](https://qwenlm.github.io/qwen-code-docs/en/users/configuration/settings/).

## Global configuration

Portable [voice settings](@/docs/spec-format/settings.md#voice-dictation) write `general.voice.enabled`, `general.voice.mode`, and `general.voice.language`; choose a native `voiceModel` before using dictation.

`agnostic-ai sync --global -t qwen` writes instructions, agents, skills, MCP, hooks, and settings under `~/.qwen/`. Set `QWEN_HOME` to replace that whole directory. `QWEN_RUNTIME_DIR` moves runtime state, not configuration. `agnostic-ai import --global qwen` reads the user model, permission lists, and MCP servers. See [global configuration](@/docs/global-configuration.md).

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

Root instructions use the shared [entry-point configuration](@/docs/configuration-reference.md#outputs). Changing a path does not change where Qwen looks for it. Keep overrides aligned with your native configuration.

## Import

```bash
agnostic-ai import qwen
agnostic-ai lint
agnostic-ai sync -t qwen
agnostic-ai sync --check -t qwen
```

Import reads root and scoped instructions, rule files directly under `.qwen/rules/`, agents, skill folders and resources from `.qwen/skills/` and `.agents/skills/` (the `.qwen/skills/` folder wins a same-name clash), Markdown and legacy TOML commands, `.qwenignore`, and settings, hooks, and MCP servers in `.qwen/settings.json`.

A rule file's `paths:` becomes portable `globs`. Import names each rule file without `paths:`: sync moves that rule into `QWEN.md`, so delete the original file. It also names and skips rule files in subfolders, and stops when a rule file shares its name with a rule in `QWEN.md`.

Native fields stay under `x-qwen`; HTTP `httpUrl` becomes portable `type: http`, while SSE `url` becomes `type: sse`. Imported legacy commands emit as Markdown.

Import rejects credentials in retained agent and skill headers, hooks, and settings. Settings can refer to environment variables with `$NAME` or `${NAME}`. Qwen does not expand those references in an agent's or skill's credential fields, so import rejects them there.
