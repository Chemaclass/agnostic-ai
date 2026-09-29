+++
title = "Hook specs"
description = "Define hook events and handlers, and see which targets run them."
weight = 30

[extra]
group = "Reference"
+++

# Hook specs

## Hooks

Pure YAML, no markdown body.

```yaml
name: session-status
description: Show repository status when a session starts.
targets: [claude, codex]
event: SessionStart
command: "git status --short"
```

Command hooks receive event JSON on stdin; read the edited path or shell command from the target's `tool_input` fields. `AGNOSTIC_AI_TARGET` names the target that ran the hook; see [which target ran a hook](#hook-target).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Hook identifier. |
| `description` | no | empty | Free-form documentation. |
| `event` | yes | none | Hook event, written verbatim. See [events](#events). |
| `matcher` | no | empty | Regex on the tool name, or another event-specific selector. |
| `command` | command handlers only | none | Shell command, or a list where each entry becomes its own handler. |
| `args` | no | empty | Switches to **exec form**: `command` runs as an executable with `args` as its argument vector and no shell, so spaces, `$`, and backticks pass verbatim. Leave unset when the command needs a pipe or `&&`. Targets with no exec form (Codex, Gemini, Cursor) get the args folded into `command`, each quoted for a POSIX shell. |
| `type` | no | `command` | `command`, `http`, `mcp_tool`, or `prompt`, where the target supports it. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Some targets convert to milliseconds or apply their own default. |
| `disabled` | no | `false` | Keep the hook defined but stop it running. Antigravity and Kiro write `enabled: false`; OpenCode and Kilo write no plugin module; other targets emit the hook unchanged. |

Handler-specific fields emit only where the target's schema defines them:

- `server`, `tool`, `input` (`type: mcp_tool`): Claude Code, Codex.
- `url`, `headers`, `allowedEnvVars` (HTTP handler): Claude Code, Qoder, Copilot.
- `prompt`, `model` (prompt handler): Claude Code, Qoder, Cursor, Copilot (`sessionStart` only).
- `statusMessage`, `async`: Claude Code, Codex, Qoder.
- `asyncRewake`, `shell`, `if`: Claude Code, Qoder.
- `continueOnBlock`: Claude Code. `commandWindows`, `additionalContextLimit`: Codex. `failClosed`: Cursor. `loop_limit`: Cursor, Trae.
- `x-goose.on_failure` (Goose), `x-kiro.action` (Kiro), `x-gemini.hooks`, `x-gemini.sequential`, `x-gemini.name`, `x-gemini.env` (Gemini).

`command` is not needed for a non-command handler, a valid `x-kiro.action`, or a hook that sets `x-gemini.hooks`. Scope a non-command hook to the targets that support it with `target` or `targets`.

### Events

`event` is written verbatim; names are never translated between tools. Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`, so one spec feeds both. Other tools need their own names, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`. `agnostic-ai validate` flags an event a target does not recognize. Targets without hook support log a warning and skip.

### Which target ran a hook {#hook-target}

A shared script reads `AGNOSTIC_AI_TARGET` to pick the reply protocol (Claude Code and Codex read exit code 2 and stderr; Cursor reads JSON).

```sh
case "$AGNOSTIC_AI_TARGET" in
  cursor) echo '{"permission":"deny","agent_message":"Blocked."}' ;;
  *) echo "Blocked." >&2; exit 2 ;;
esac
```

A spec that sets `AGNOSTIC_AI_TARGET` in its own `env` keeps that value. Where sync cannot set it, the parent process's value stays, which can be `claude` for a tool started from Claude Code. Sync sets it per target:

- [Claude Code](@/docs/targets/claude.md): `env` in `.claude/settings.json` (`~/.claude/settings.json` for `sync --global`). Set for the whole session, so the Bash tool sees it too.
- [Cursor](@/docs/targets/cursor.md): a `sessionStart` hook returns the variable. `sessionStart` hooks and hooks that fire before it returns do not see it.
- [Codex](@/docs/targets/codex.md): `export AGNOSTIC_AI_TARGET=codex; ` before `command`. Not set on Windows (`commandWindows`), and needs a POSIX session shell (a `pwsh` or `nu` login shell breaks it).
- [Gemini](@/docs/targets/gemini.md), [Qoder](@/docs/targets/qoder.md), [Copilot](@/docs/targets/copilot.md): `env` on each command handler.
- [Goose](@/docs/targets/goose.md), [Crush](@/docs/targets/crush.md), [Cline](@/docs/targets/cline.md): `export` prefix or line.
- [OpenCode](@/docs/targets/opencode.md), [Kilo](@/docs/targets/kilo.md): `.env()` on each plugin command. [Zed](@/docs/targets/zed.md): `env` on each task.

Trae, Factory, OpenHands, Antigravity, Kiro, Windsurf, and Augment do not get the variable: none has a per-hook `env`, and a prefix would break hooks that work today. Tell them apart by their own variables, such as `TRAE_PROJECT_DIR`, `FACTORY_PROJECT_DIR`, `OPENHANDS_PROJECT_DIR`, `DEVIN_PROJECT_DIR`, or `AUGMENT_PROJECT_DIR`. Do not use `CLAUDE_PROJECT_DIR`: Cursor, Gemini, Qoder, Factory, and Trae set it too.

`sync --global` leaves a matching hand-written hook alone, so an adopted Codex, Gemini, or Qoder entry does not get the variable. Cursor and Copilot also run `.claude/settings.json` hooks but read no `env` from it: Cursor still gets `cursor` from `sessionStart`, Copilot gets nothing.

### Per-target body fences

To vary prose per target, wrap the divergent part in `::target` fences. Content outside a fence emits everywhere; content inside emits only to the listed targets. Marker lines never reach the output.

```md
Shared intro paragraph.

::target claude
Claude-only section.
::end

::targets codex gemini
Codex and Gemini section.
::end
```

| Syntax | Meaning |
|--------|---------|
| `::target <name>` | Opens a fence for one target. |
| `::targets <a> <b>` | Opens a fence for several targets. |
| `::end` | Closes the most recent fence. A missing `::end` runs to the end of the body. |

- `import` round-trips keep fences intact. `import codex` builds them when Claude and Codex ship the same agent or skill with different bodies.
- Fences also work in `.agnostic-ai/AGNOSTIC_AI.md`. A block reaches an entry-point file when any target that reads the file is listed. `AGENTS.md` is shared by the whole family, so `::target codex` content reaches every reader; a shared file is never split. See [Entry-point files](@/docs/configuration.md#entry-point-files).

