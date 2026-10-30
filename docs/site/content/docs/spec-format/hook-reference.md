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

Cursor, Copilot, and Cline signal a block in their own way. Sync runs each portable `before-tool` command (and, on Cursor, each `prompt-submit` command) through a bash wrapper that turns exit 2 into the tool's deny reply, with stderr as the reason. Exit 1 differs: Cursor goes on, as Claude Code does; Copilot denies the call, so a guard with a typo or a missing dependency keeps blocking; Cline ignores it with no report. On Windows, where the tool runs hook commands through PowerShell, the wrapper cannot start: Cursor goes on with an error, and Copilot denies the call. A hook written with `event` gets no wrapper. See [Cursor](@/docs/targets/cursor.md#hooks), [Copilot](@/docs/targets/copilot.md#hooks), and [Cline](@/docs/targets/cline.md#hooks) for the reply each wrapper writes.

On Cursor, the deny reply carries stderr as `user_message` and `agent_message`, or on `prompt-submit` is `"continue": false` with `user_message`. At exit 0 the wrapper passes on the JSON reply the command prints. Cursor gets no `after-tool`, `after-edit`, or `stop`, because exit 2 on `postToolUse`, `afterFileEdit`, and `stop` is a failure Cursor moves past. Cursor does not wait for `session-start` or `session-end`, and does not add `session-start` stdout to the session as Claude Code does.

A spec sets `on` or `event`, never both; `match` goes with `on`, and `matcher` with `event`. `validate` and `lint` (LINT032) report an unknown value, both forms mixed, an event that a reached tool reads differently (such as `on: stop` on Crush), and a tool kind a reached tool lacks (such as `match: read` on Codex).

Sync reports missing mappings. For a tool with no mapping, write `event` for the hook, or limit it with `targets`. Trae has no portable hook mapping yet.

Kiro maps only `on: before-tool`: CLI V3 cannot block on `after-tool` or `prompt-submit`, and its `stop` uses exit 1 to keep the agent running. `decision: stdout` has no Kiro mapping. The `edit` matcher leaves out `code`, which Kiro's own `write` category includes. See [Kiro](@/docs/targets/kiro.md) for how tool IDs and `mcp:<server>` map.

`agnostic-ai migrate --only hooks` rewrites `event` and `matcher` as `on` and `match` when the portable form gives every reached tool the same event and matcher, and says why it leaves any other hook as written. A Claude Code hook on `Edit|Write` stays native, since `match: edit` there also runs on `MultiEdit` and `NotebookEdit`. `lint` warns on each hook the migration would rewrite (LINT034), and `agnostic-ai import` follows the same rule.

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

Stdout carries only the decision, so print logs to stderr. At exit 0:

- `"decision": "deny"` blocks the call with `reason` as the message, the same as exit 2 with that message on stderr.
- `"ask"` blocks too, since not every tool can ask the user.
- `"allow"`, or empty stdout, goes on as a plain exit 0. It never grants more than the tool's own permission rules do.
- Anything else blocks, so a broken guard never lets a call through: stdout that is not one JSON object, an object with no top-level `decision` or more than one, or another `decision` value.

Only the top level counts, so a nested `decision` never overrides the verdict. Raw NUL and SOH bytes block, and so does stdout over 1,000,000 bytes, more than 10,000 values, or more than 64 nested containers. A `\u` escape outside ASCII reads as `?`. A non-zero exit keeps its usual meaning.

No tool reads this object itself: on every tool, each `x-gemini.hooks` command handler included, sync runs the command through a bash wrapper that turns the decision into that tool's block. So `decision: stdout` cannot go with `commandWindows` or `shell: powershell`. Augment runs only a bare script path, so it cannot run the wrapper, and `validate` names a `decision: stdout` hook that reaches it.

### Hooks Cursor and Copilot also read {#claude-settings-copies}

Cursor and Copilot also run `.claude/settings.json` hooks. When `cursor` is a target, sync puts `[ "$AGNOSTIC_AI_TARGET" = cursor ] && exit 0;` before the Claude Code copy of each portable hook that also reaches Cursor. Cursor's `sessionStart` hook sets that variable, so the hook runs once there, as Cursor's own copy. Claude Code sets it to `claude`, and an unset variable runs the hook. A hook still runs twice on Cursor when it fires before Cursor's `sessionStart` hook returns, when it has `args` or `shell: powershell` (no POSIX shell to run the check), or when it is a user hook from `sync --global`, which adds no check.

Copilot sets no variable, so a hook synced to `claude` and `copilot` [runs twice there](@/docs/targets/copilot.md#hooks); both copies block on exit 2.

## Native events

`event` and `matcher` take a tool's own names. This hook formats the Go files the agent edits, on Claude Code and Codex alike:

```yaml
name: gofmt-on-edit
description: Format the Go files the agent edits.
targets: [claude, codex]
event: PostToolUse
matcher: Edit|Write
command: 'files=$(agnostic-ai hook paths) || exit 1; printf "%s\n" "$files" | grep "\.go$" | while IFS= read -r f; do gofmt -w "$f"; done'
```

Codex takes `Edit` and `Write` as aliases for `apply_patch`, so the one matcher fires on both tools. [`agnostic-ai hook paths`](@/docs/spec-format/hook-reference.md#edited-paths) reads the edited files from the event JSON on stdin and must be on the hook's `PATH`. The `|| exit 1` fails the hook when `hook paths` fails, for example on bad JSON or a missing binary; without it, the loop would exit 0.

Command hooks receive event JSON on stdin; read the shell command from `tool_input`. `AGNOSTIC_AI_TARGET` names the tool that ran the hook; see [which target ran a hook](@/docs/spec-format/hook-reference.md#hook-target). OpenCode runs hooks as plugins; see [OpenCode V2 hooks](@/docs/targets/opencode.md#opencode-v2-hooks) for the V1 and V2 runtimes.

Sync keeps hook entries you wrote. In `.claude/settings.json`, `.codex/hooks.json`, `.cursor/hooks.json`, `.gemini/settings.json`, `.qoder/settings.json`, and `.factory/hooks.json`, it puts its own entries after yours in each event and changes or removes only those. A hand-written entry that matches a spec's output, with the same matcher (for example after `import`), counts as the spec's own, so it is not written twice.

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
| `args` | no | empty | **Exec form**: `command` runs as a program with these arguments and no shell, so spaces, `$`, and backticks pass through. Leave unset when the command needs a pipe or `&&`. Claude Code, Qoder, Copilot, and OpenCode V2 on macOS and Linux keep `args` apart; other tools append them to `command`, each quoted for a POSIX shell, which Trae's PowerShell and OpenHands' cmd.exe on Windows can read differently (sync notes it). Augment skips a hook with `args`, with a note. |
| `type` | no | `command` | `command`, `http`, `mcp_tool`, or `prompt`, where the tool supports it. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Tool defaults are in the [shell and timeout table](@/docs/cli-reference/hook-diagnostics.md#hook-run). |
| `disabled` | no | `false` | Keep the hook defined but stop it running. Antigravity and Kiro write `enabled: false`; OpenCode and Kilo write no plugin module; other tools write the hook unchanged. |

Handler-specific fields are written only where the tool's schema defines them:

- `server`, `tool`, `input` (`type: mcp_tool`): Claude Code, Codex.
- `url`, `headers`, `allowedEnvVars` (HTTP handler): Claude Code, Qoder, Copilot.
- `prompt`, `model` (prompt handler): Claude Code, Qoder, Cursor, Copilot (`sessionStart` only). CodeBuddy takes `prompt` without `model`, on `Stop`, `UserPromptSubmit`, and `PreToolUse`.
- `statusMessage`, `async`: Claude Code, Codex, Qoder.
- `asyncRewake`, `shell`, `if`: Claude Code, Qoder.
- `continueOnBlock`: Claude Code, CodeBuddy. `commandWindows`: Codex, Copilot. `additionalContextLimit`: Codex. `loop_limit`: Cursor, Trae.
- `failClosed`: Claude Code (command and HTTP handlers), Cursor. `true` blocks when the hook cannot start, times out, or exits with a code other than 0 or 2; Cursor also blocks on exit 0 with no output. See [Claude Code hook mappings](@/docs/targets/claude-reference.md#hook-mappings) for the events it ignores there.
- `x-goose.on_failure` (Goose), `x-kiro.action` (Kiro), `x-gemini.hooks`, `x-gemini.sequential`, `x-gemini.name`, `x-gemini.env` (Gemini).

`command` is not needed for a non-command handler, a valid `x-kiro.action`, or a hook that sets `x-gemini.hooks`. A hook with none of these, and no `command` under an `x-<target>` block, runs nothing, so `validate` and `lint` (LINT052) report it. Limit a non-command hook to the tools that support it with `target` or `targets`.

## Events

`event` is written as is, never translated between tools; for one spec across tools, use [portable events](@/docs/spec-format/hook-reference.md#portable-events). Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`. Other tools need their own names, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`.

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

Claude Code, Cursor, and Trae provide the variable, so sync keeps it. Gemini, Qoder, and Factory get their own project-root variable. Other tools get `$(git rev-parse --show-toplevel)` in a POSIX shell, and Codex gets it in `commandWindows` too.

When the configuration lives in a subdirectory of a Git worktree, the written path includes that subdirectory, so a hook started from a deeper directory still finds the project. Script paths also move to the tool's hook directory, such as `.claude/hooks/` to `.codex/hooks/`; custom script paths keep their directory. `sync --global` uses the Git root at run time, so it needs Git and a hook running inside the intended worktree, and cannot tell apart several configured projects in one worktree.

Exec-form `args`, escaped dollars, and single-quoted variables stay literal. A note names the hook when the tool cannot keep a parameter operator such as `${CLAUDE_PROJECT_DIR:-fallback}` or <code>${&#35;CLAUDE_PROJECT_DIR}</code>, a nested substitution or here-document, non-POSIX root syntax, or a project without a Git worktree. `on-unsupported: error` fails the sync; `silent` hides the note. For these cases, write a command for the specific tool or set `target: claude`, and set the tool's Windows command yourself.

## Which target ran a hook {#hook-target}

A shared script reads `AGNOSTIC_AI_TARGET` to pick how to reply (Claude Code and Codex read exit 2 and stderr; Cursor reads JSON).

```sh
case "$AGNOSTIC_AI_TARGET" in
  cursor) echo '{"permission":"deny","agent_message":"Blocked."}' ;;
  *) echo "Blocked." >&2; exit 2 ;;
esac
```

A spec that sets `AGNOSTIC_AI_TARGET` in its own `env` keeps that value, except on OpenCode V2. Where sync cannot set it, the parent process's value stays. How sync sets it:

| Target | How |
|---|---|
| [Claude Code](@/docs/targets/claude.md) | `env` in `.claude/settings.json` (`~/.claude/settings.json` for `sync --global`), for the whole session, so the Bash tool sees it too |
| [Cursor](@/docs/targets/cursor.md) | A `sessionStart` hook returns it, so `sessionStart` hooks and hooks that fire before it returns do not see it |
| [Codex](@/docs/targets/codex.md) | `export AGNOSTIC_AI_TARGET=codex; ` before `command`; not on Windows (`commandWindows`), and only with a POSIX session shell (a `pwsh` or `nu` login shell breaks it) |
| [Gemini](@/docs/targets/gemini.md), [Qoder](@/docs/targets/qoder.md), [Copilot](@/docs/targets/copilot.md) | `env` on each command handler |
| [Goose](@/docs/targets/goose.md), [Crush](@/docs/targets/crush.md), [Cline](@/docs/targets/cline.md) | An `export` prefix or line |
| [OpenCode](@/docs/targets/opencode.md) V1, [Kilo](@/docs/targets/kilo.md) | `.env()` on each plugin command; OpenCode V2 sets `opencode` in each child process's environment |
| [Zed](@/docs/targets/zed.md) | `env` on each task |

Trae, Factory, OpenHands, Antigravity, Kiro, Windsurf, and Augment do not get the variable. Tell them apart by their own variables, such as `TRAE_PROJECT_DIR`, `FACTORY_PROJECT_DIR`, `OPENHANDS_PROJECT_DIR`, `DEVIN_PROJECT_DIR`, or `AUGMENT_PROJECT_DIR`. `CLAUDE_PROJECT_DIR` does not identify Claude Code, because Cursor and Trae provide it too.

`sync --global` leaves a matching hand-written hook alone, so an adopted Codex, Gemini, or Qoder entry does not get the variable. Copilot reads no `env` from `.claude/settings.json`, so its copy of a Claude Code hook gets no value.

## Read the edited paths {#edited-paths}

`agnostic-ai hook paths` reads the event data a hook gets on stdin and prints each file the edit leaves on disk, one per line, relative to the directory the hook runs in (a path outside it prints in full). A tool call that is not an edit prints nothing.

```sh
agnostic-ai hook paths            # src/app.go
agnostic-ai hook paths --action   # update<TAB>src/app.go
agnostic-ai hook paths --json     # [{"action": "update", "path": "src/app.go"}]
```

- The tool comes from `AGNOSTIC_AI_TARGET`, or from `--target`, which wins. Codex on Windows (`commandWindows`) gets no `AGNOSTIC_AI_TARGET`, so pass `--target codex` there.
- With no tool named, event data whose `tool_input` holds `file_path`, `edits`, or `notebook_path` reads as Claude Code, which covers Cursor and Copilot running `.claude/settings.json` hooks. Other event data with no tool named fails, as do invalid JSON and an edit tool's `tool_input` that is not an object (exit 1).
- A plain run skips deleted files and the source of a move, so a formatter sees only files that exist. `--action` and `--json` list every change as `add`, `update`, `delete`, or `move`; a move shows as a `delete` of its source and a `move` of its destination, which `--json` gives a `from`.
- When a tool does not say whether a write created the file, the change shows as `update`.
- A hook that runs before the edit finds no new file on disk, so run formatters after the edit.

| Target | What it reads |
|---|---|
| Claude Code | `tool_input.file_path` of `Edit`, `Write`, and `MultiEdit`; `tool_input.notebook_path` of `NotebookEdit` |
| Codex | The `*** Add File:`, `*** Update File:`, `*** Delete File:`, and `*** Move to:` headers in the `apply_patch` text in `tool_input.command`, so every file in the patch |
| Cursor | `file_path` of `afterFileEdit` and `afterTabFileEdit` only; other Cursor events print nothing |
| Gemini | `tool_input.file_path` of `write_file` and `replace`, a relative path starting at `cwd` |
| Factory | `tool_input.file_path` of `Create` and `Edit`; a Factory `ApplyPatch` prints nothing, since its input is undocumented |
| Qoder | `tool_input.file_path` of `Write`; `Edit` fails, since the docs show none of its `tool_input` fields |
| Augment | `file_changes[].path` with its `changeType`; before the edit, `tool_input.path` of `str-replace-editor` and `save-file` |

Factory and Augment do not get `AGNOSTIC_AI_TARGET`, so give their hooks their own spec and pass `--target`.

<details>
<summary>Tools the command does not read</summary>

Their docs do not show an edit's event data, so the command fails:

- Windsurf, Trae, Copilot, Goose, Antigravity, OpenHands, Cline.
- Kiro: a `PostFileSave` command can use its `filePath` template variable.
- Crush: tool events set `CRUSH_TOOL_INPUT_FILE_PATH`. See [native lifecycle events and matcher limits](@/docs/targets/crush.md#hooks).
- OpenCode V1 and Kilo: plugins get tool arguments as JavaScript objects, not as event data on stdin. OpenCode V2 sends native event JSON, which this command does not read.
- Zed: no hook fires on an edit.

</details>

## Spec guard hook {#spec-guard}

`agnostic-ai init --demo` seeds two hooks that check your specs while the agent works, on Claude Code, Codex, and Gemini:

- `spec-guard-edit` runs after each edit. When the agent edits a spec, it sees the lint errors in that file on its next turn.
- `spec-guard-stop` runs when the agent stops. When specs changed without a sync, the agent sees one line: `Specs changed since the last sync: run agnostic-ai sync.` It never runs sync itself, and `sync --check` still catches a hand edit to a generated file.

```yaml
name: spec-guard-edit
targets: [claude, codex, gemini]
on: after-edit
command: 'command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard after-edit'
```

Both fail open: with `agnostic-ai` missing from `PATH`, or event data [`hook guard`](@/docs/cli-reference/maintain.md#hook-guard) cannot read, they exit 0 with no output. Other tools keep the commit-time check of [`install-hook`](@/docs/cli-reference/maintain.md#install-hook). Factory runs `after-edit` and `stop` but does not document its hook shell, so the hooks leave it out. Gemini on Windows runs hook commands in Windows PowerShell, which cannot parse the `command -v` line, so each run there fails with a non-blocking error; remove `gemini` from the hooks' `targets` on Windows.
