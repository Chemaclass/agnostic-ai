+++
title = "Hook reference"
description = "Portable event mappings, native handler fields, and event input contracts."
weight = 51

[extra]
group = "Reference"
+++

# Hook reference

## Portable events {#portable-events}

Write `on` and `match` instead of one tool's own event name. Sync writes each tool's native event and matcher, so one spec runs on every tool that has a mapping.

```yaml
name: no-force-push
description: Block git push --force.
on: before-tool
match: shell
command: .agnostic-ai/scripts/no-force-push.sh
```

`on` takes `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, or `session-end`. `after-edit` runs after a file edit, so it takes no `match`.

`match` takes a tool kind: `shell`, `edit`, `read`, `web`, `any`, or `mcp:<server>`. It applies only to `before-tool` and `after-tool`. Leave it out, or write `any`, to run on every tool. `edit` covers every edit tool, including ones some versions lack, so a guard misses none.

A tool gets only the events where exit codes work as in Claude Code: exit 0 goes on, and exit 2 blocks with stderr as the reason. `before-tool` blocks the tool call, `prompt-submit` blocks the prompt, and `stop` keeps the agent working. `after-tool` and `after-edit` map to the tool's after-tool event, and `any` writes no matcher. Copilot only warns on exit 2 outside `before-tool`, so it gets the session events and `before-tool`.

| Tool | `on` | `match` |
|---|---|---|
| CodeBuddy Code | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `^Bash$`, `edit` `^(Edit\|Write)$`, `read` `^Read$`, `web` `^(WebFetch\|WebSearch)$`, `mcp:<server>` `^mcp__<server>__.*$` |
| Claude Code | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `Bash`, `edit` `Edit\|MultiEdit\|Write\|NotebookEdit`, `read` `Read`, `web` `WebFetch\|WebSearch`, `mcp:<server>` `mcp__<server>__.*` |
| Codex | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `Bash`, `edit` `Edit\|Write`, `mcp:<server>` `mcp__<server>__.*` |
| Gemini | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `^run_shell_command$`, `edit` `^(write_file\|replace)$`, `read` `^(read_file\|read_many_files)$`, `web` `^(web_fetch\|google_web_search)$` |
| Qwen Code | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `^run_shell_command$`, `edit` `^(write_file\|edit)$`, `read` `^read_file$`, `web` `^(web_fetch\|web_search)$`, `mcp:<server>` `mcp__<server>__.*` |
| Factory | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `^Execute$`, `edit` `^(Create\|Edit\|ApplyPatch)$`, `read` `^Read$`, `web` `^(FetchUrl\|WebSearch)$` |
| Qoder | `session-start`, `prompt-submit`, `before-tool`, `stop`, `session-end` | `shell` `Bash`, `edit` `Edit\|Write\|NotebookEdit`, `read` `Read`, `web` `WebFetch\|WebSearch`, `mcp:<server>` `mcp__<server>__.*` |
| OpenHands | `session-start`, `prompt-submit`, `before-tool`, `stop`, `session-end` | `shell` `terminal` |
| Goose | `session-start`, `before-tool`, `stop`, `session-end` | `shell` `^shell$`, `edit` `^(write\|edit)$` |
| Augment | `session-start`, `before-tool`, `session-end` | `shell` `^launch-process$`, `edit` `^(str-replace-editor\|save-file)$`, `web` `^(web-fetch\|web-search)$` |
| Crush | `before-tool` | `shell` `^bash$`, `edit` `^(edit\|multiedit\|write)$` |
| Windsurf | `prompt-submit`, `before-tool`, `stop` | `shell` `^exec$`, `edit` `^(edit\|write\|apply_patch)$`, `read` `^read$`, `web` `^(webfetch\|web_search)$` |
| Cursor | `session-start`, `prompt-submit`, `before-tool`, `session-end` | `shell` `^Shell$`, `edit` `^Write$`, `read` `^Read$` |
| Copilot | `session-start`, `before-tool`, `session-end` | `shell` `Bash`, `edit` `Edit\|Write`, `read` `Read`, `web` `WebFetch\|WebSearch` |
| Cline | `before-tool` | `shell` `run_commands\|execute_command`, `edit` `editor\|apply_patch\|replace_in_file\|write_to_file`, `read` `read_files\|read_file`, `web` `fetch_web_content\|web_fetch\|web_search` |
| Kiro | `before-tool` | `shell` `^(execute_bash\|execute_pwsh)$`, `edit` `^(fs_write\|fs_append\|str_replace\|delete_file)$`, `read` `^(read_file\|list_directory\|file_search\|grep_search\|code\|tool_search\|introspect)$`, `web` `^(web_fetch\|remote_web_search)$`, `mcp:<server>` `@<server>/*` |

Cursor, Copilot, and Cline signal a block in their own way. For them, sync runs each portable `before-tool` command (on Cursor, each `prompt-submit` command too) through a wrapper that turns exit 2 into the tool's deny reply, with stderr as the reason:

- **Cursor** gets `"permission": "deny"`, with stderr as `user_message` and `agent_message`. At exit 0 the wrapper replies `"permission": "allow"`, or passes on the JSON reply the command prints. On `prompt-submit` it replies `"continue": false` with stderr as `user_message`, or `"continue": true`. Exit 1 goes on, as in Claude Code. The wrapper is `.cursor/hooks/agnostic-ai-portable-hook.sh`.
- **Copilot** gets exit 2 with `"permissionDecision": "deny"` and stderr as `permissionDecisionReason`. Exit 1 and any other failure deny the call too, where Claude Code reports an error and goes on. A guard with a typo or a missing dependency keeps blocking. The wrapper is `.github/hooks/scripts/agnostic-ai-portable-hook.sh`.
- **Cline** gets `{"cancel": true}` with stderr as `errorMessage`, written into its event script. A cancel also stops the run. Cline ignores exit 1, so the call goes on with no report. Cline has no matcher, so the script runs the command only when the event data names one of the kind's tools.

The wrappers are bash scripts. On Windows, where a tool runs hook commands through PowerShell, it cannot start them: Cursor goes on with an error, and Copilot denies the call. A hook written with `event` syncs as written, with no wrapper. `sync --global` writes the wrapper beside the user hooks file, such as `~/.cursor/hooks/agnostic-ai-portable-hook.sh`.

Cursor gets no `after-tool`, `after-edit`, or `stop`, because its `postToolUse`, `afterFileEdit`, and `stop` cannot block: exit 2 there is a failure Cursor moves past. `session-start` and `session-end` map without a wrapper. Cursor does not wait for either one, and on `session-start` it does not add plain stdout to the session as Claude Code does.

A spec sets `on` or `event`, never both. `match` goes with `on`, and `matcher` with `event`. `validate` and `lint` (LINT032) report:

- an unknown value
- both forms mixed in one spec
- an event that a tool the hook reaches reads differently, such as `on: stop` on Crush
- a tool kind that a tool the hook reaches lacks, such as `match: read` on Codex

Sync reports missing mappings. For a tool with no mapping, write `event` for the hook, or limit it with `targets`. Trae has no portable hook mapping yet.

On Kiro, only `on: before-tool` maps; use a command that exits 2 to block the tool call. Kiro CLI V3 cannot block on `after-tool` or `prompt-submit`, and its `stop` event uses exit 1 to keep the agent running, so none of them maps. `decision: stdout` has no Kiro mapping either.

- For Kiro's built-in tools, sync matches the documented tool IDs from start to end. Kiro's own category selectors also match parts of tool IDs, so a `read` selector could run on an MCP tool with `read` in its name; the portable mapping does not. A new built-in ID needs an updated mapping.
- The `edit` matcher leaves out `code`, which Kiro's own `write` category includes.
- `match: mcp:<server>` becomes `@<server>/*` and matches that server only, even when its name is `mcp`, `builtin`, or `powers`.

`agnostic-ai migrate --only hooks` rewrites `event` and `matcher` as `on` and `match` when the portable form gives every tool the hook reaches the same event and matcher. It leaves every other hook as written and says why. A Claude Code hook on `Edit|Write` stays native, since `match: edit` there also runs on `MultiEdit` and `NotebookEdit`. `lint` warns on each hook the migration would rewrite (LINT034). `agnostic-ai import` follows the same rule.

### Decision on stdout {#decision-on-stdout}

Set `decision: stdout` on a `before-tool` hook to decide with a JSON object instead of an exit code:

```yaml
name: no-force-push
on: before-tool
match: shell
decision: stdout
command: .agnostic-ai/scripts/no-force-push.sh
```

```sh
echo '{"decision": "deny", "reason": "Use --force-with-lease."}'
```

With `decision: stdout`, stdout carries only the decision. Print logs to stderr. At exit 0:

- `"decision": "deny"` blocks the call with `reason` as the message, the same as exit 2 with that message on stderr.
- `"ask"` blocks too, since not every tool can ask the user.
- `"allow"`, or empty stdout, goes on as a plain exit 0. It never grants more than the tool's own permission rules do.
- Anything else blocks, so a broken guard never lets a call through. That covers stdout that is not one JSON object, an object with no top-level `decision` or more than one, and a `decision` of another value.

Only the top level counts, so a nested `decision` never overrides the verdict. Raw NUL and SOH bytes block, and so does stdout over 1,000,000 bytes, more than 10,000 values, or more than 64 nested containers. A `\u` escape outside ASCII reads as `?`. Any exit code other than 0 keeps its usual meaning.

No tool reads this object itself. On every tool, sync runs the command through a bash wrapper that turns the decision into that tool's block, each `x-gemini.hooks` command handler included. So `decision: stdout` cannot go with `commandWindows` or `shell: powershell`. Augment runs only a bare script path, so it cannot run the wrapper, and `validate` names a `decision: stdout` hook that reaches it.

### Hooks Cursor and Copilot also read {#claude-settings-copies}

Cursor and Copilot also run `.claude/settings.json` hooks. When `cursor` is a target, sync puts `[ "$AGNOSTIC_AI_TARGET" = cursor ] && exit 0;` before the Claude Code copy of each portable hook that also reaches Cursor. Cursor's `sessionStart` hook sets that variable, so the hook runs once there, as Cursor's own copy. Claude Code sets it to `claude`, and an unset variable runs the hook.

A hook still runs twice on Cursor when:

- it fires before Cursor's `sessionStart` hook returns
- it has `args` or `shell: powershell`, which has no POSIX shell to run the check
- it is a user hook from `sync --global`, which adds no check

A hook synced to `claude` and `copilot` runs twice on Copilot, which sets no variable. Both copies block on exit 2.

## Native events

`agnostic-ai new hook session-status` creates `hooks/session-status.yaml`, plain YAML with no markdown body. Replace its placeholder with a native hook, such as:

```yaml
name: session-status
description: Show repository status when a session starts.
targets: [claude, codex]
event: SessionStart
command: "git status --short"
```

Format Go files after the agent edits them, on Claude Code and Codex alike. [`agnostic-ai hook paths`](@/docs/spec-format/hook-reference.md#edited-paths) reads the edited files from the event JSON on stdin, whichever tool sent it.

```yaml
name: gofmt-on-edit
description: Format the Go files the agent edits.
targets: [claude, codex]
event: PostToolUse
matcher: Edit|Write
command: 'files=$(agnostic-ai hook paths) || exit 1; printf "%s\n" "$files" | grep "\.go$" | while IFS= read -r f; do gofmt -w "$f"; done'
```

Codex takes `Edit` and `Write` as aliases for `apply_patch`, so the one matcher fires on both tools. `agnostic-ai` must be on the hook's `PATH`. The `|| exit 1` fails the hook when `hook paths` fails, for example on bad JSON or a missing binary; without it, the loop would exit 0.

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

OpenCode V2 hooks select `x-opencode.plugin-api: v2` and use native before and after events on macOS and Linux. The default stays V1. Windows needs V1 with the default V1 hooks; V2 cannot run V1 plugins. See [OpenCode V2 hooks](@/docs/targets/opencode.md#opencode-v2-hooks) for stdin, exit codes, timeouts, and how Windows refuses V2.

Command hooks receive event JSON on stdin. Read the shell command from `tool_input` and the edited paths with [`agnostic-ai hook paths`](@/docs/spec-format/hook-reference.md#edited-paths). `AGNOSTIC_AI_TARGET` names the tool that ran the hook; see [which target ran a hook](@/docs/spec-format/hook-reference.md#hook-target).

Sync keeps hook entries you wrote. In `.claude/settings.json`, `.codex/hooks.json`, `.cursor/hooks.json`, `.gemini/settings.json`, `.qoder/settings.json`, and `.factory/hooks.json`, it puts its own entries after yours in each event and changes or removes only those. An entry you wrote by hand that matches a spec's output, with the same matcher (for example after `import`), counts as the spec's own, so it is not written twice.

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Hook identifier. |
| `description` | no | empty | Free-form documentation. |
| `on` | `on` or `event` | none | Portable event, written as each tool's native event. See [portable events](@/docs/spec-format/hook-reference.md#portable-events). |
| `match` | no | every tool | Tool kind for `on: before-tool` or `after-tool`. See [portable events](@/docs/spec-format/hook-reference.md#portable-events). |
| `decision` | no | none | `stdout` reads a `{"decision": ..., "reason": ...}` object at exit 0 on `on: before-tool`. See [decision on stdout](@/docs/spec-format/hook-reference.md#decision-on-stdout). |
| `event` | `on` or `event` | none | Hook event, written as is. See [events](@/docs/spec-format/hook-reference.md#events). |
| `matcher` | no | empty | Regex on the tool name, or another selector the event uses. |
| `command` | command handlers only | none | Shell command, or a list where each entry becomes its own handler. |
| `args` | no | empty | Switches to **exec form**: `command` runs as a program with `args` as its arguments and no shell, so spaces, `$`, and backticks pass through unchanged. Leave unset when the command needs a pipe or `&&`. Claude Code, Qoder, Copilot, and OpenCode V2 on macOS and Linux keep `args` apart. The other tools run `command` in a shell, with the args added, each quoted for a POSIX shell. On Windows, Trae's PowerShell and OpenHands' cmd.exe can read those quotes differently, and sync notes it. Augment skips a hook with `args`, with a note. |
| `type` | no | `command` | `command`, `http`, `mcp_tool`, or `prompt`, where the tool supports it. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Some tools convert to milliseconds or apply their own default. |
| `disabled` | no | `false` | Keep the hook defined but stop it running. Antigravity and Kiro write `enabled: false`; OpenCode and Kilo write no plugin module; other tools write the hook unchanged. |

Handler-specific fields are written only where the tool's schema defines them:

- `server`, `tool`, `input` (`type: mcp_tool`): Claude Code, Codex.
- `url`, `headers`, `allowedEnvVars` (HTTP handler): Claude Code, Qoder, Copilot.
- `prompt`, `model` (prompt handler): Claude Code, Qoder, Cursor, Copilot (`sessionStart` only).
- `statusMessage`, `async`: Claude Code, Codex, Qoder.
- `asyncRewake`, `shell`, `if`: Claude Code, Qoder.
- `continueOnBlock`: Claude Code. `commandWindows`: Codex, Copilot. `additionalContextLimit`: Codex. `loop_limit`: Cursor, Trae.
- `failClosed`: Claude Code (command and HTTP handlers), Cursor. Set it to `true` to block when the hook cannot start, times out, or exits with a code other than 0 or 2. For Claude Code, use a synchronous `PreToolUse` or `UserPromptSubmit` hook. See [Claude Code failure behavior](@/docs/targets/claude.md) for ignored events, background hooks, `PermissionRequest` denial, and preserved imports. Cursor also blocks on exit 0 with no output, which Claude Code allows.
- `x-goose.on_failure` (Goose), `x-kiro.action` (Kiro), `x-gemini.hooks`, `x-gemini.sequential`, `x-gemini.name`, `x-gemini.env` (Gemini).

`command` is not needed for a non-command handler, a valid `x-kiro.action`, or a hook that sets `x-gemini.hooks`. Limit a non-command hook to the tools that support it with `target` or `targets`.

## Events

`event` is written as is, never translated between tools. For one spec across tools, use [portable events](@/docs/spec-format/hook-reference.md#portable-events).

- Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`, so one spec works on both.
- Other tools need their own names, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`.
- `agnostic-ai validate` flags an event a tool does not recognize.
- `sync` and `sync --check` stop before writing when an event looks like a typo of a known one, such as `PreToolUze`, and name the closest.
- Tools without hook support log a warning and skip the hook.

## Imported project-root paths

A shell-form command imported from Claude Code can use `$CLAUDE_PROJECT_DIR` or `${CLAUDE_PROJECT_DIR}` in a script path:

```yaml
name: guard
event: PreToolUse
command: 'sh "$CLAUDE_PROJECT_DIR/.claude/hooks/guard.sh"'
```

What `sync` writes for the variable:

- Claude Code, Cursor, and Trae provide it, so sync keeps it.
- Gemini, Qoder, and Factory use their own project-root variable.
- Other tools get `$(git rev-parse --show-toplevel)` in a POSIX shell. Codex gets it in `commandWindows` too.

If the configuration lives in a subdirectory of a Git worktree, the written path includes that subdirectory, so a hook started from a deeper directory still finds the project. Script paths also move to the tool's hook directory, such as `.claude/hooks/` to `.codex/hooks/`. Custom script paths keep their directory.

<details>
<summary>Global sync</summary>

`sync --global` uses the Git root at run time for those tools. It needs Git and a hook running inside the intended Git worktree. It cannot find a project outside Git, or tell apart several configured projects in one worktree.

</details>

These stay literal: exec-form `args`, escaped dollars, and single-quoted variables.

These get a note naming the hook when the tool cannot keep them:

- parameter operators such as `${CLAUDE_PROJECT_DIR:-fallback}` and <code>${&#35;CLAUDE_PROJECT_DIR}</code>
- nested substitutions and here-documents
- non-POSIX root syntax
- a project without a Git worktree

`on-unsupported: error` fails the sync; `silent` hides the note. For these cases, write a command for the specific tool or set `target: claude`. For Windows, set the tool's Windows command yourself.

## Which target ran a hook {#hook-target}

A shared script reads `AGNOSTIC_AI_TARGET` to pick how to reply (Claude Code and Codex read exit 2 and stderr; Cursor reads JSON).

```sh
case "$AGNOSTIC_AI_TARGET" in
  cursor) echo '{"permission":"deny","agent_message":"Blocked."}' ;;
  *) echo "Blocked." >&2; exit 2 ;;
esac
```

A spec that sets `AGNOSTIC_AI_TARGET` in its own `env` keeps that value, except OpenCode V2 sets it to `opencode` for each invocation. Where sync cannot set it, the parent process's value stays. Sync sets it for each tool:

- [Claude Code](@/docs/targets/claude.md): `env` in `.claude/settings.json` (`~/.claude/settings.json` for `sync --global`). Set for the whole session, so the Bash tool sees it too.
- [Cursor](@/docs/targets/cursor.md): a `sessionStart` hook returns the variable, so `sessionStart` hooks and hooks that fire before it returns do not see it.
- [Codex](@/docs/targets/codex.md): `export AGNOSTIC_AI_TARGET=codex; ` before `command`. Not set on Windows (`commandWindows`), and needs a POSIX session shell (a `pwsh` or `nu` login shell breaks it).
- [Gemini](@/docs/targets/gemini.md), [Qoder](@/docs/targets/qoder.md), [Copilot](@/docs/targets/copilot.md): `env` on each command handler.
- [Goose](@/docs/targets/goose.md), [Crush](@/docs/targets/crush.md), [Cline](@/docs/targets/cline.md): `export` prefix or line.
- [OpenCode](@/docs/targets/opencode.md) V1 and [Kilo](@/docs/targets/kilo.md): `.env()` on each plugin command. OpenCode V2 adds it to each child process's environment. [Zed](@/docs/targets/zed.md): `env` on each task.

Trae, Factory, OpenHands, Antigravity, Kiro, Windsurf, and Augment do not get the variable. Tell them apart by their own variables, such as `TRAE_PROJECT_DIR`, `FACTORY_PROJECT_DIR`, `OPENHANDS_PROJECT_DIR`, `DEVIN_PROJECT_DIR`, or `AUGMENT_PROJECT_DIR`. `CLAUDE_PROJECT_DIR` does not identify Claude Code, because Cursor and Trae provide it too.

`sync --global` leaves a matching hand-written hook alone, so an adopted Codex, Gemini, or Qoder entry does not get the variable. Copilot reads no `env` from `.claude/settings.json`, so its copy of a Claude Code hook gets no value.

## Read the edited paths {#edited-paths}

`agnostic-ai hook paths` reads the event data a hook gets on stdin and prints each file the edit leaves on disk, one per line.

- Paths print relative to the directory the hook runs in. A path outside it prints in full.
- A tool call that is not an edit prints nothing.
- The tool comes from `AGNOSTIC_AI_TARGET`, or from `--target`, which wins. Codex on Windows (`commandWindows`) gets no `AGNOSTIC_AI_TARGET`, so pass `--target codex` there.
- With no tool named, event data whose `tool_input` holds `file_path`, `edits`, or `notebook_path` reads as Claude Code, which covers Cursor and Copilot running `.claude/settings.json` hooks.
- Any other event data with no tool named fails, as do invalid JSON and an edit tool's `tool_input` that is not an object (exit 1).

```sh
agnostic-ai hook paths            # src/app.go
agnostic-ai hook paths --action   # update<TAB>src/app.go
agnostic-ai hook paths --json     # [{"action": "update", "path": "src/app.go"}]
```

A plain run skips deleted files and the source of a move, so a formatter sees only files that exist. `--action` and `--json` list every change as `add`, `update`, `delete`, or `move`.

- A move shows as a `delete` of its source and a `move` of its destination, which `--json` gives a `from`.
- When a tool does not say whether a write created the file, the change shows as `update`.
- A hook that runs before the edit finds no new file on disk, so run formatters after the edit.

| Target | What it reads |
|---|---|
| Claude Code | `tool_input.file_path` of `Edit`, `Write`, and `MultiEdit`; `tool_input.notebook_path` of `NotebookEdit` |
| Codex | The `*** Add File:`, `*** Update File:`, `*** Delete File:`, and `*** Move to:` headers in the `apply_patch` text in `tool_input.command`, so every file in the patch |
| Cursor | `file_path` of `afterFileEdit` and `afterTabFileEdit` only; other Cursor events print nothing |
| Gemini | `tool_input.file_path` of `write_file` and `replace`, a relative path starting at `cwd` |
| Factory | `tool_input.file_path` of `Create` and `Edit` |
| Qoder | `tool_input.file_path` of `Write`; `Edit` fails, since the docs show none of its `tool_input` fields |
| Augment | `file_changes[].path` with its `changeType`; before the edit, `tool_input.path` of `str-replace-editor` and `save-file` |

Factory and Augment do not get `AGNOSTIC_AI_TARGET`, so give their hooks their own spec and pass `--target`. Factory documents no input for `ApplyPatch`, so a Factory patch prints nothing.

<details>
<summary>Tools the command does not read</summary>

Their docs do not show an edit's event data, so the command fails:

- Windsurf, Trae, Copilot, Goose, Antigravity, OpenHands, Cline.
- Kiro: a `PostFileSave` command can use its `filePath` template variable.
- Crush: only `PreToolUse` runs, and it sets `CRUSH_TOOL_INPUT_FILE_PATH`.
- OpenCode V1 and Kilo: plugins get tool arguments as JavaScript objects, not as event data on stdin. OpenCode V2 sends native event JSON, which this command does not read.
- Zed: no hook fires on an edit.

</details>

## Spec guard hook {#spec-guard}

`agnostic-ai init --demo` seeds two hooks that check your specs while the agent works, on Claude Code, Codex, and Gemini:

- `spec-guard-edit` runs after each edit. When the agent edits a spec, it sees the lint errors in that file on its next turn. Any other edit prints nothing.
- `spec-guard-stop` runs when the agent stops. When specs changed without a sync, the agent sees one line: `Specs changed since the last sync: run agnostic-ai sync.` It never runs sync itself.

```yaml
name: spec-guard-edit
targets: [claude, codex, gemini]
on: after-edit
command: 'command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard after-edit'
```

Both fail open: with `agnostic-ai` missing from `PATH`, or event data [`hook guard`](@/docs/cli-reference/maintain.md#hook-guard) cannot read, they exit 0 with no output. Other tools keep the commit-time check of [`install-hook`](@/docs/cli-reference/maintain.md#install-hook).

Limits:

- Gemini on Windows runs hook commands in Windows PowerShell, which cannot parse the `command -v` line, so each run there fails with a non-blocking error. Remove `gemini` from the hooks' `targets` on Windows.
- The hooks leave out Factory, which runs `after-edit` and `stop` but does not document its hook shell.
- The stop notice covers specs that changed without a sync. `sync --check` catches a hand edit to a generated file.
