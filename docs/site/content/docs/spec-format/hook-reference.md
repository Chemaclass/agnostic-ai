+++
title = "Hook reference"
description = "Portable event mappings, native handler fields, and event input contracts."
weight = 51

[extra]
group = "Reference"
+++

# Hook reference

## Portable events {#portable-events}

Write `on` and `match` instead of a tool's own event name. Sync writes each tool's native event and matcher.

```yaml
name: no-force-push
description: Block git push --force.
on: before-tool
match: shell
command: .agnostic-ai/scripts/no-force-push.sh
```

- `on` takes `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, or `session-end`. `after-edit` runs after a file edit and takes no `match`.
- `match` takes a tool kind: `shell`, `edit`, `read`, `web`, `any`, or `mcp:<server>`. It applies only to `before-tool` and `after-tool`. Leave it out, or write `any`, to match every tool.
- `edit` covers every edit tool, so a guard misses none.

Portable hooks follow Claude Code exit codes:

- Exit 0 continues.
- Exit 2 blocks, with stderr as the reason. `before-tool` blocks the tool call, `prompt-submit` blocks the prompt, and `stop` keeps the agent working.
- `after-tool` and `after-edit` map to the tool's after-tool event. `any` writes no matcher.
- Copilot only warns on exit 2 outside `before-tool`, so it gets the session events and `before-tool`.

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
| Augment | `session-start`, `before-tool`, `session-end` | `shell` `^(bash\|launch-process)$`, `edit` `^(edit\|write\|str-replace-editor\|save-file)$`, `web` `^(web-fetch\|web-search)$` |
| Crush | `before-tool` | `shell` `^bash$`, `edit` `^(edit\|multiedit\|write)$` |
| Windsurf | `prompt-submit`, `before-tool`, `stop` | `shell` `^exec$`, `edit` `^(edit\|write\|apply_patch)$`, `read` `^read$`, `web` `^(webfetch\|web_search)$` |
| Cursor | `session-start`, `prompt-submit`, `before-tool`, `session-end` | `shell` `^Shell$`, `edit` `^Write$`, `read` `^Read$` |
| Copilot | `session-start`, `before-tool`, `session-end` | `shell` `Bash`, `edit` `Edit\|Write`, `read` `Read`, `web` `WebFetch\|WebSearch` |
| Cline | `before-tool` | `shell` `run_commands\|execute_command`, `edit` `editor\|apply_patch\|replace_in_file\|write_to_file`, `read` `read_files\|read_file`, `web` `fetch_web_content\|web_fetch\|web_search` |
| Kiro | `before-tool` | `shell` `^(execute_bash\|execute_pwsh)$`, `edit` `^(fs_write\|fs_append\|str_replace\|delete_file)$`, `read` `^(read_file\|list_directory\|file_search\|grep_search\|code\|tool_search\|introspect)$`, `web` `^(web_fetch\|remote_web_search)$`, `mcp:<server>` `@<server>/*` |

A spec sets `on` or `event`, never both. `match` goes with `on`, and `matcher` with `event`. When a tool has no mapping, sync reports it. Use the native `event` name, or leave the tool out with `targets`. Trae has no portable mapping yet.

`validate` and `lint` (LINT032) report:

- an unknown value;
- both forms mixed;
- an event a reached tool reads differently, such as `on: stop` on Crush;
- a tool kind a reached tool lacks, such as `match: read` on Codex.

`agnostic-ai migrate --only hooks` rewrites `event` and `matcher` as `on` and `match` when every reached tool gets the same event and matcher. It says why it leaves any other hook alone. A Claude Code hook on `Edit|Write` stays native, since `match: edit` there also runs on `MultiEdit` and `NotebookEdit`. `lint` warns on each hook the migration would rewrite (LINT034), and `import` follows the same rule.

<details>
<summary>How Cursor, Copilot, and Cline block</summary>

Sync runs each portable `before-tool` command through a bash wrapper (on Cursor, each `prompt-submit` command too). The wrapper turns exit 2 into the tool's deny reply, with stderr as the reason. A hook written with `event` gets no wrapper.

| Tool | Exit 1 |
|---|---|
| Cursor | Goes on, as Claude Code does |
| Copilot | Denies the call, so a typo or missing dependency keeps blocking |
| Cline | Ignored, with no report |

- On Windows the tools run hook commands through PowerShell and the wrapper cannot start. Cursor goes on with an error. Copilot denies the call.
- Cursor's deny reply carries stderr as `user_message` and `agent_message`. On `prompt-submit` it is `"continue": false` with `user_message`.
- At exit 0 the Cursor wrapper passes on the JSON reply the command prints.
- Cursor gets no `after-tool`, `after-edit`, or `stop`, because exit 2 on `postToolUse`, `afterFileEdit`, and `stop` is a failure Cursor moves past.
- Cursor does not wait for `session-start` or `session-end`. It does not add `session-start` stdout to the session as Claude Code does.
- The reply each wrapper writes: [Cursor](@/docs/targets/cursor-reference.md#hooks), [Copilot](@/docs/targets/copilot-reference.md#hooks), [Cline](@/docs/targets/cline-reference.md#hooks).

</details>

<details>
<summary>Kiro portable mapping</summary>

- Kiro maps only `on: before-tool`. CLI V3 cannot block on `after-tool` or `prompt-submit`, and its `stop` uses exit 1 to keep the agent running.
- `decision: stdout` has no Kiro mapping.
- The `edit` matcher leaves out `code`, which Kiro's own `write` category includes.
- [Kiro](@/docs/targets/kiro.md) shows how tool IDs and `mcp:<server>` map.

</details>

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

| Stdout | Result |
|---|---|
| `"decision": "deny"` | Blocks, with `reason` as the message. Same as exit 2 with that message on stderr. |
| `"ask"` | Blocks too, since not every tool can ask the user. |
| `"allow"`, or empty | Goes on as a plain exit 0. It never grants more than the tool's own permission rules. |
| Anything else | Blocks, so a broken guard never lets a call through. |

"Anything else" means stdout that is not one JSON object, an object with no top-level `decision` or more than one, or another `decision` value. A non-zero exit keeps its usual meaning. `decision: stdout` cannot go with `commandWindows` or `shell: powershell`.

<details>
<summary>Parsing limits and the wrapper</summary>

- Only the top level counts, so a nested `decision` never overrides the verdict.
- Raw NUL and SOH bytes block.
- Stdout over 1,000,000 bytes, more than 10,000 values, or more than 64 nested containers blocks.
- A `\u` escape outside ASCII reads as `?`.
- No tool reads this object itself. On every tool, each `x-gemini.hooks` command handler included, sync runs the command through a bash wrapper that turns the decision into that tool's block.
- Sync writes Augment hooks without the wrapper, so `validate` names a `decision: stdout` hook that reaches Augment.

</details>

### Hooks Cursor and Copilot also read {#claude-settings-copies}

Cursor and Copilot also run `.claude/settings.json` hooks, so a hook can run twice there. Sync avoids that on Cursor, but not on Copilot.

<details>
<summary>How the double run is avoided</summary>

- When `cursor` is a target, sync puts `[ "$AGNOSTIC_AI_TARGET" = cursor ] && exit 0;` before the Claude Code copy of each portable hook that also reaches Cursor.
- Cursor's `sessionStart` hook sets that variable, so the hook runs once, as Cursor's own copy. Claude Code sets it to `claude`, and an unset variable runs the hook.
- A hook still runs twice on Cursor when it fires before Cursor's `sessionStart` hook returns, when it has `args` or `shell: powershell` (no POSIX shell to run the check), or when it is a user hook from `sync --global`, which adds no check.
- Copilot sets no variable, so a hook synced to `claude` and `copilot` [runs twice there](@/docs/targets/copilot-reference.md#hooks). Both copies block on exit 2.

</details>

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

- Codex takes `Edit` and `Write` as aliases for `apply_patch`, so the one matcher fires on both tools.
- [`agnostic-ai hook paths`](@/docs/spec-format/hook-reference.md#edited-paths) reads the edited files from the event JSON on stdin. It must be on the hook's `PATH`.
- `|| exit 1` fails the hook when `hook paths` fails (bad JSON, missing binary). Without it the loop would exit 0.
- Command hooks receive event JSON on stdin. Read the shell command from `tool_input`.
- `AGNOSTIC_AI_TARGET` names the tool that ran the hook ([details](@/docs/spec-format/hook-reference.md#hook-target)).
- [Vibe](@/docs/targets/vibe-reference.md#hooks) uses native events such as `pre_tool` and `post_agent`. Vibe 3.1.3 adds `pre_llm` and `post_llm`. Sync writes `matcher` as native `match`. Matchers take globs or regular expressions with a `re:` prefix. Vibe gets no portable events.
- [Kiro CLI 3.4.0](https://kiro.dev/docs/hooks/types/) adds `PermissionRequest` in V3 sessions, with a tool-name matcher and command actions only. The accepted alias `permissionRequest` gets validation advice to use the canonical event name.
- OpenCode runs hooks as plugins ([V1 and V2 runtimes](@/docs/targets/opencode-reference.md#opencode-v2-hooks)).

<details>
<summary>Hand-written hook entries stay</summary>

- Applies to `.claude/settings.json`, `.codex/hooks.json`, `.cursor/hooks.json`, `.gemini/settings.json`, `.qoder/settings.json`, and `.factory/hooks.json`.
- Sync puts its own entries after yours in each event, and changes or removes only those.
- A hand-written entry with the same output and matcher as a spec (for example after `import`) counts as the spec's own, so it is not written twice.

</details>

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
| `args` | no | empty | Exec form: `command` runs as a program with these arguments and no shell, so spaces, `$`, and backticks pass through. Leave unset when the command needs a pipe or `&&`. Claude Code, Qoder, Copilot, Augment, and OpenCode V2 (macOS, Linux) keep `args` apart. Other tools append them to `command`, each quoted for a POSIX shell. Trae's PowerShell and OpenHands' cmd.exe on Windows can read that differently; sync notes it. |
| `type` | no | `command` | `command`, `http`, `mcp_tool`, or `prompt`, where the tool supports it. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Tool defaults are in the [shell and timeout table](@/docs/cli-reference/hook-diagnostics.md#hook-run). |
| `disabled` | no | `false` | Keep the hook defined but stop it running. Antigravity and Kiro write `enabled: false`; OpenCode and Kilo write no plugin module; other tools write the hook unchanged. |

<details>
<summary>Handler-specific fields by tool</summary>

Written only where the tool's schema defines them:

- `server`, `tool`, `input` (`type: mcp_tool`): Claude Code, Codex.
- `url`, `headers`, `allowedEnvVars` (HTTP handler): Claude Code, Qoder, Copilot.
- `prompt`, `model` (prompt handler): Claude Code, Qoder, Cursor, Copilot (`sessionStart` only). CodeBuddy takes `prompt` without `model`, on `Stop`, `UserPromptSubmit`, and `PreToolUse`.
- `statusMessage`, `async`: Claude Code, Codex, Qoder.
- `asyncRewake`, `shell`, `if`: Claude Code, Qoder.
- `continueOnBlock`: Claude Code, CodeBuddy.
- `commandWindows`: Codex, Copilot.
- `additionalContextLimit`: Codex.
- `loop_limit`: Cursor, Trae.
- `failClosed`: Claude Code 2.1.295+ for command and HTTP handlers and 2.1.328+ for MCP-tool handlers, Cursor, and Factory (as `onFailure: "block"`). See [Claude Code handler limits](@/docs/targets/claude-reference.md#hook-mappings).
  - `true` blocks on hook failure or timeout. For command hooks, failure includes a missing executable or an exit code other than 0 or 2. Cursor also blocks on exit 0 with no output. Claude Code treats HTTP connection errors, non-2xx responses, disconnected MCP servers, tool errors and invalid output as failures. [Claude Code failure handling](https://code.claude.com/docs/en/hooks#block-the-action-when-a-hook-fails)
  - Events each tool ignores: [Claude Code hook mappings](@/docs/targets/claude-reference.md#hook-mappings), [Factory hooks](@/docs/targets/factory-reference.md#hooks).
- Goose: `x-goose.on_failure`.
- Factory: `x-factory.commandRegex` (a regex filter on the group), `x-factory.preserveRiskLevel` and `x-factory.onFailure` (per entry).
- Kiro: `x-kiro.action`.
- Gemini: `x-gemini.hooks`, `x-gemini.sequential`, `x-gemini.name`, `x-gemini.env`.

</details>

`command` is optional for a non-command handler, a valid `x-kiro.action`, or a hook that sets `x-gemini.hooks`. A hook with none of these, and no `command` under an `x-<target>` block, runs nothing: `validate` and `lint` (LINT052) report it. Limit a non-command hook to the tools that support it with `target` or `targets`.

## Events

`event` is written as is and never translated, so it works only on tools that know the name. Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`. Others need their own, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`. For one spec across tools, use [portable events](@/docs/spec-format/hook-reference.md#portable-events).

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

<details>
<summary>Project-root variable by tool, and limits</summary>

Per tool:

- Claude Code, Cursor, and Trae provide the variable, so sync keeps it.
- Gemini, Qoder, and Factory get their own project-root variable.
- Other tools get `$(git rev-parse --show-toplevel)` in a POSIX shell. Codex gets it in `commandWindows` too.

Paths:

- When the configuration lives in a subdirectory of a Git worktree, the written path includes that subdirectory, so a hook started from a deeper directory still finds the project.
- Project script paths move to the tool's hook directory, such as `.claude/hooks/` to `.codex/hooks/`. Paths under `~`, `$HOME`, or an absolute directory stay as written, and custom script paths keep their directory.
- `sync --global` uses the Git root at run time. It needs Git and a hook running inside the intended worktree, and cannot tell apart several configured projects in one worktree.

Limits:

- Exec-form `args`, escaped dollars, and single-quoted variables stay literal.
- A note names the hook when the tool cannot keep a parameter operator such as `${CLAUDE_PROJECT_DIR:-fallback}` or <code>${&#35;CLAUDE_PROJECT_DIR}</code>, a nested substitution or here-document, non-POSIX root syntax, or a project without a Git worktree.
- `on-unsupported: error` fails the sync. `silent` hides the note.
- For these cases, write a command for the specific tool or set `target: claude`, and set the tool's Windows command yourself.

</details>

## Which target ran a hook {#hook-target}

A shared script reads `AGNOSTIC_AI_TARGET` to pick its reply format: Claude Code and Codex read exit 2 and stderr, Cursor reads JSON.

```sh
case "$AGNOSTIC_AI_TARGET" in
  cursor) echo '{"permission":"deny","agent_message":"Blocked."}' ;;
  *) echo "Blocked." >&2; exit 2 ;;
esac
```

<details>
<summary>How each tool gets AGNOSTIC_AI_TARGET</summary>

- A spec that sets `AGNOSTIC_AI_TARGET` in its own `env` keeps that value, except on OpenCode V2.
- Where sync cannot set the variable, the parent process's value stays.

| Target | How |
|---|---|
| [Claude Code](@/docs/targets/claude.md) | `env` in `.claude/settings.json` (`~/.claude/settings.json` for `sync --global`), for the whole session, so the Bash tool sees it too |
| [Cursor](@/docs/targets/cursor.md) | A `sessionStart` hook returns it, so `sessionStart` hooks and hooks that fire before it returns do not see it |
| [Codex](@/docs/targets/codex.md) | `export AGNOSTIC_AI_TARGET=codex; ` before `command`; not on Windows (`commandWindows`), and only with a POSIX session shell (a `pwsh` or `nu` login shell breaks it) |
| [Gemini](@/docs/targets/gemini.md), [Qoder](@/docs/targets/qoder.md), [Copilot](@/docs/targets/copilot.md) | `env` on each command handler |
| [Goose](@/docs/targets/goose.md), [Crush](@/docs/targets/crush.md), [Cline](@/docs/targets/cline.md) | An `export` prefix or line |
| [OpenCode](@/docs/targets/opencode.md) V1, [Kilo](@/docs/targets/kilo.md) | `.env()` on each plugin command; OpenCode V2 sets `opencode` in each child process's environment |
| [Zed](@/docs/targets/zed.md) | `env` on each task |

Tools that do not get the variable: Trae, Factory, OpenHands, Antigravity, Kiro, Windsurf, Augment.

- Tell them apart by their own variables: `TRAE_PROJECT_DIR`, `FACTORY_PROJECT_DIR`, `OPENHANDS_PROJECT_DIR`, `DEVIN_PROJECT_DIR`, `AUGMENT_PROJECT_DIR`.
- `CLAUDE_PROJECT_DIR` does not identify Claude Code, because Cursor and Trae provide it too.
- `sync --global` leaves a matching hand-written hook alone, so an adopted Codex, Gemini, or Qoder entry does not get the variable.
- Copilot reads no `env` from `.claude/settings.json`, so its copy of a Claude Code hook gets no value.

</details>

## Read the edited paths {#edited-paths}

`agnostic-ai hook paths` reads the event data a hook gets on stdin and prints each file the edit leaves on disk, one per line. Paths are relative to the hook's directory; a path outside it prints in full. A tool call that is not an edit prints nothing.

```sh
agnostic-ai hook paths            # src/app.go
agnostic-ai hook paths --action   # update<TAB>src/app.go
agnostic-ai hook paths --json     # [{"action": "update", "path": "src/app.go"}]
```

Options:

- The tool comes from `AGNOSTIC_AI_TARGET`, or from `--target`, which wins. Codex on Windows (`commandWindows`) gets no `AGNOSTIC_AI_TARGET`, so pass `--target codex` there.
- A plain run skips deleted files and the source of a move, so a formatter sees only files that exist.
- `--action` and `--json` list every change as `add`, `update`, `delete`, or `move`. A move shows as a `delete` of its source and a `move` of its destination, which `--json` gives a `from`.
- When a tool does not say whether a write created the file, the change shows as `update`.
- Run formatters after the edit: a hook that runs before it finds no new file on disk.

With no tool named:

- Event data whose `tool_input` holds `file_path`, `edits`, or `notebook_path` reads as Claude Code. This covers Cursor and Copilot running `.claude/settings.json` hooks.
- Other event data fails, as do invalid JSON and an edit tool's `tool_input` that is not an object (exit 1).

What each tool reads:

| Target | What it reads |
|---|---|
| Claude Code | `tool_input.file_path` of `Edit`, `Write`, and `MultiEdit`; `tool_input.notebook_path` of `NotebookEdit` |
| Codex | The `*** Add File:`, `*** Update File:`, `*** Delete File:`, and `*** Move to:` headers in the `apply_patch` text in `tool_input.command`, so every file in the patch |
| Cursor | `file_path` of `afterFileEdit` and `afterTabFileEdit` only; other Cursor events print nothing |
| Gemini | `tool_input.file_path` of `write_file` and `replace`, a relative path starting at `cwd` |
| Factory | `tool_input.file_path` of `Create` and `Edit`; a Factory `ApplyPatch` prints nothing, since its input is undocumented |
| Qoder | `tool_input.file_path` of `Write`; `Edit` fails, since the docs show none of its `tool_input` fields |
| Augment | `file_changes[].path` with its `changeType`; before the edit, `tool_input.path` of `edit` and `write` (Auggie 1.x) or `str-replace-editor` and `save-file` (0.x) |

Factory and Augment do not get `AGNOSTIC_AI_TARGET`, so give their hooks their own spec and pass `--target`.

<details>
<summary>Tools the command does not read</summary>

The command does not read these targets:

- Windsurf, Trae, Copilot, Antigravity, OpenHands, Cline: their docs do not show an edit's event data.
- Goose: `hook paths` has no Goose decoder. Goose [documents its tool input](https://github.com/aaif-goose/goose/blob/v1.60.0/documentation/docs/guides/context-engineering/hooks.md#tool-input-keys): `write` has `path` and `content`; `edit` has `path`, `before`, and `after`.
- Kiro: a `PostFileSave` command can use its `filePath` template variable.
- Crush: tool events set `CRUSH_TOOL_INPUT_FILE_PATH`. See [native lifecycle events and matcher limits](@/docs/targets/crush-reference.md#hooks).
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

Both fail open: they exit 0 with no output when `agnostic-ai` is missing from `PATH` or when [`hook guard`](@/docs/cli-reference/maintain.md#hook-guard) cannot read the event data.

- Other tools keep the commit-time check of [`install-hook`](@/docs/cli-reference/maintain.md#install-hook).
- Factory supports `after-edit` and `stop`, but these built-ins do not target it. Its [documented launcher and preview limits](@/docs/cli-reference/hook-diagnostics.md#assumed-results) are separate from this target list.
- Gemini on Windows runs hook commands in Windows PowerShell, which cannot parse the `command -v` line, so each run fails with a non-blocking error. Remove `gemini` from the hooks' `targets` on Windows.
