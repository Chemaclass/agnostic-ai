+++
title = "Hooks"
description = "hooks/: commands the tool runs on lifecycle events, such as before a shell command or when a session starts."
weight = 50

[extra]
group = "Reference"
moved = { per-target-body-fences = "@/docs/spec-format/_index.md" }
+++

# Hooks

`hooks/` holds commands the AI tool runs at fixed points: when a session starts, before a tool call, after a file edit, when the agent stops. An instruction asks the model to do something; a hook makes it happen every time.

- **Guardrails.** Block a force push or a write to a generated file before it happens.
- **Automatic follow-up.** Format or lint a file right after the agent edits it.
- **Fresh context.** Print the branch, open issues, or service status when a session starts.
- **One script, many tools.** Tools that share event names, such as Claude Code and Codex, run one spec. `AGNOSTIC_AI_TARGET` tells a shared script which tool called it.

agnostic-ai never runs a hook itself; the configured tools do. Review hook specs like code.

## Write one

`agnostic-ai new hook session-status` creates `hooks/session-status.yaml`. Pure YAML, no markdown body.

```yaml
name: session-status
description: Show repository status when a session starts.
targets: [claude, codex]
event: SessionStart
command: "git status --short"
```

Format Go files after Claude Code edits them. The hook reads the edited path from the event JSON on stdin:

```yaml
name: gofmt-on-edit
description: Format a Go file after the agent edits it.
target: claude
event: PostToolUse
matcher: Edit|Write
command: 'f=$(jq -r .tool_input.file_path); case "$f" in *.go) gofmt -w "$f" ;; esac'
```

It is scoped to Claude Code because Codex reports edits as `apply_patch` with no `tool_input.file_path`; see the [Codex page](@/docs/targets/codex.md).

Run a script with exact arguments and no shell, so spaces and `$` pass through untouched:

```yaml
name: guard-commands
event: PreToolUse
matcher: Bash
target: claude
command: scripts/hooks/guard.sh
args: ["--deny", "git push --force"]
timeout: 10
```

Command hooks receive event JSON on stdin; read the edited path or shell command from the target's `tool_input` fields. `AGNOSTIC_AI_TARGET` names the target that ran the hook; see [which target ran a hook](#hook-target).

## Fields

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

## Events

`event` is written verbatim; names are never translated between tools. Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`, so one spec feeds both. Other tools need their own names, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`. `agnostic-ai validate` flags an event a target does not recognize. Targets without hook support log a warning and skip.

## Which target ran a hook {#hook-target}

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
