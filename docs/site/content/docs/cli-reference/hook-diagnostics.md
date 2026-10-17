+++
title = "Test hook behavior"
description = "Run sample hook events and understand diagnostic results and runtime limits."
weight = 55

[extra]
group = "Reference"
+++

# Test hook behavior

## Test a hook {#hook-run}

`agnostic-ai hook run <hook>` runs a hook spec the way each tool would, before a session fires it. A hook that blocks on Claude Code and does nothing on Codex shows up here, not in a live session:

```text
$ agnostic-ai hook run protect-files --edit .github/workflows/tests.yml
claude: block (exit 2, 12ms)
  event: PreToolUse (Write)
  command: .claude/hooks/protect-files.sh
  stderr: Blocked: .github/workflows/tests.yml is protected.
codex: allow (exit 0, 9ms)
  event: PreToolUse (apply_patch)
  command: export AGNOSTIC_AI_TARGET=codex; .codex/hooks/protect-files.sh
hook protect-files: targets decide differently: claude block, codex allow
```

For each configured tool the hook reaches, it runs every command sync wrote for that tool, from the project root, or from the handler's `cwd` on Copilot.

### Event data replies

`--edit <path>` and `--bash <command>` build a `PreToolUse` or `PostToolUse` tool call (`BeforeTool` or `AfterTool` on Gemini). `--prompt <text>` builds `UserPromptSubmit` (`BeforeAgent` on Gemini). `SessionStart` takes its `source` from the matcher, or `startup` when the matcher is empty. `--payload <file>` sends a JSON file as is, for any event.
### Matcher replies

If the matcher does not match the tool or source, the tool would not run the hook, so that tool reports `allow` and why. A tool call from `--payload` matches its `tool_name` with the tool's own matcher rules, including Codex's `Edit` and `Write` aliases for `apply_patch`. Cursor, Goose, Copilot, and Kiro match the field their docs name for the event, such as the command, file path, or prompt text.
### Missing event replies

A tool without the hook's event, such as Codex for `Notification`, is listed as not run.
### Timeout replies

The timeout sync writes, or the tool's default.
### Async replies

An `async: true` hook, or an `asyncRewake: true` one on Claude Code and Qoder, runs and prints its output. Its result is `not judged` and stays out of `--expect` and the comparison, because the tool does not wait for it. The same holds for Cursor `sessionStart` and `sessionEnd` hooks, Copilot `notification` hooks, and Cline hooks on any event but `PreToolUse` and `PostToolUse`.
### Background commands replies

On macOS and Linux, a command the hook leaves running is killed once the hook exits.
### Stale tool file replies

Commands come from the spec, so the run works before a sync. The tool prints a `warning` naming the file when the synced file is missing, when no handler under the event runs the command the spec produces, or when that handler's group matcher, timeout, or (on Gemini) `env` differs from the spec. On Claude Code and Cursor it also compares `failClosed`, on Copilot `cwd`, and on Kiro `confirm`. An `env` warning names the keys, never their values. A warning does not fail the run; `sync` clears it. The file is the one each [tool page](@/docs/targets/_index.md) lists, or the path `outputs` sets.

<details>
<summary>Environment variables per tool</summary>

`hook run` first removes the variables listed here from the calling shell's env. Other variables pass through.

| Target | Variables set |
|--------|---------------|
| Claude Code | `AGNOSTIC_AI_TARGET=claude`, `CLAUDE_PROJECT_DIR` |
| Codex | none; its command sets the target itself |
| Gemini | `GEMINI_PROJECT_DIR`, `GEMINI_CWD`, `GEMINI_SESSION_ID`, `CLAUDE_PROJECT_DIR`, then the handler's `env`, which holds `AGNOSTIC_AI_TARGET=gemini` |
| Trae | `TRAE_PROJECT_DIR`, `CLAUDE_PROJECT_DIR` |
| OpenHands | `OPENHANDS_PROJECT_DIR`, `OPENHANDS_SESSION_ID`, `OPENHANDS_EVENT_TYPE`, `OPENHANDS_TOOL_NAME` |
| Goose | `PLUGIN_ROOT`, with `AGNOSTIC_AI_TARGET=goose` from its command |
| Augment | `AUGMENT_PROJECT_DIR`, `AUGMENT_CONVERSATION_ID`, `AUGMENT_HOOK_EVENT`, `AUGMENT_TOOL_NAME` |
| Cursor | `CURSOR_PROJECT_DIR`, `CURSOR_VERSION` (empty), `CLAUDE_PROJECT_DIR`, and `AGNOSTIC_AI_TARGET=cursor`, which the `sessionStart` entry that sync adds sets for later hooks |
| Crush | `CRUSH=1`, `AGENT=crush`, `AI_AGENT=crush`, `CRUSH_EVENT`, `CRUSH_TOOL_NAME`, `CRUSH_SESSION_ID`, `CRUSH_CWD`, `CRUSH_PROJECT_DIR`, `CRUSH_TOOL_INPUT_COMMAND` or `CRUSH_TOOL_INPUT_FILE_PATH` when the tool input has one, and editor and pager variables set so nothing waits for a terminal; `AGNOSTIC_AI_TARGET=crush` comes from its command |
| Copilot | the handler's `env`, which holds `AGNOSTIC_AI_TARGET=copilot`. A value with `$` is listed as not run. |
| Factory | `FACTORY_PROJECT_DIR` |
| Qoder | `QODER_PROJECT_DIR`, then the handler's `env`, which holds `AGNOSTIC_AI_TARGET=qoder` |
| Antigravity | none |
| Cline | none; the script exports `AGNOSTIC_AI_TARGET=cline` |
| Kiro | `USER_PROMPT` on `UserPromptSubmit` |
| Windsurf | `DEVIN_PROJECT_DIR` |

</details>

<details>
<summary>Shell and default timeout per tool</summary>

Where a cell says "script path only", `hook run` assumes `sh -c` for a script path with plain or single-quoted arguments. A command with shell syntax, and any command on Windows, is listed as not run. See [assumed results](@/docs/cli-reference/hook-diagnostics.md#assumed-results).

| Target | Shell | Default timeout |
|--------|-------|-----------------|
| Claude Code | `bash -c`, on Windows with Git Bash (`CLAUDE_CODE_GIT_BASH_PATH`, else the Git install that holds `git.exe`), or PowerShell when there is none and the spec sets no `shell`; exec-form `args` with no shell; `shell: powershell` with PowerShell | 600 seconds, except 30 on `UserPromptSubmit`, `PreModelSwitch`, and `PostModelSwitch`, and 10 on `MessageDisplay` |
| Codex | `sh -c`; on Windows, `commandWindows` with `powershell.exe -Command` | 600 seconds |
| Gemini | `bash -c`, or Windows PowerShell on Windows, after Gemini replaces `$GEMINI_PROJECT_DIR`, `$GEMINI_CWD`, `$GEMINI_SESSION_ID`, and `$CLAUDE_PROJECT_DIR` with the quoted root | 60 seconds |
| Trae | `bash -c`, or PowerShell on Windows | 30 seconds |
| OpenHands | `/bin/sh -c`, or `cmd.exe /c` on Windows | 60 seconds |
| Goose | `sh -c` on every platform | 30 seconds |
| Augment | Only a `.sh`, `.ps1`, `.cmd`, or `.bat` script path, run directly on macOS and Linux, and on Windows with `powershell.exe -Command` (`.ps1`) or `cmd.exe /c` (`.cmd`, `.bat`). An inline command is listed as not run. | 60 seconds |
| Cursor | Script path only | Assumed 30 seconds |
| Crush | Crush's embedded POSIX shell, on every platform. A path starting with `./`, `../`, or `/` runs through its shebang interpreter, or with no shebang as shell source. A path without that prefix, such as `hooks/guard.sh`, starts as a program; on macOS and Linux one without a shebang then fails with exit 1. | 30 seconds |
| Copilot | Exec-form `args` with no shell. A `command` is script path only. | 30 seconds |
| Factory | Script path only, where `"$FACTORY_PROJECT_DIR"` counts as a plain word | 60 seconds |
| Qoder | Exec-form `args` with no shell; `shell: bash` with `bash -c` on macOS and Linux. Without `shell`, script path only, where `"$QODER_PROJECT_DIR"` or `"${QODER_PROJECT_DIR}"` counts as a plain word. `shell: powershell`, exec form with a `$` in `command` or `args`, and a shell-form command on Windows are listed as not run. | 600 seconds |
| Antigravity | Script path only | 30 seconds |
| Cline | `bash`, running the event script sync writes. Windows is listed as not run. | 120 seconds on `PreToolUse` and `PostToolUse`, whatever the spec sets, since sync drops `timeout` |
| Kiro | Script path only | 60 seconds. `timeout: 0` turns Kiro's limit off; `hook run` still stops the command after 60 seconds and marks the timeout assumed. |
| Windsurf | Script path only | Assumed 30 seconds |

</details>

<details>
<summary>Claude Code `if` rules</summary>

A handler's `if` permission rule decides whether it runs, as in Claude Code. It applies only on `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, and `PermissionDenied`. See the [`if` field](https://code.claude.com/docs/en/hooks) and [permission rule syntax](https://code.claude.com/docs/en/permissions#permission-rule-syntax).

- `Bash(git *)` matches any part of a command split by `&&`, `||`, `;`, `|`, and `&`, and commands inside `$()` and backticks, after leading `VAR=value` assignments and wrappers such as `timeout 30` are dropped. A trailing ` *` also matches the bare command, and `:*` is the same as ` *`.
- A pattern that names more than the command runs on any `$()`, backtick, or `$VAR`. A command that does not parse always runs.
- `Edit(path)` covers `Edit`, `Write`, `MultiEdit`, and `NotebookEdit`, with gitignore patterns. `//path` is from the filesystem root, `~/path` from home, and `/path` and `path` from the project root. A bare name such as `.env` matches at any depth.
- `Tool(param:value)` matches a top-level input field.

</details>

Each command reports one decision.

- `block`: exit 2, or exit 0 with a JSON reply that has `"permissionDecision": "deny"`, `"decision": "block"`, or `"continue": false`.
- `allow`: any other exit 0.
- `error`: any other non-zero exit.
- `timeout`: the command ran past its timeout.

With `failClosed: true`, Claude Code applies the event's exit-2 behavior to a hook failure, except on `PermissionRequest`, where it denies the request. It ignores `failClosed` on `Stop`, `SubagentStop`, `TaskCompleted`, `TeammateIdle`, and command handlers with `async: true` or `asyncRewake: true`. Use a synchronous `PreToolUse` or `UserPromptSubmit` hook when a failed check must block an action. Sync notes the ignored cases and keeps `onFailure: "block"` for repeated import and sync.

Exit 2 cannot stop anything on `SessionStart`, `SessionEnd`, `Notification`, `PreCompact`, and `PostCompact`, so it reads as `error` there. On `PostToolUse` the tool already ran, so `block` sends stderr back to the model. A `context` line marks output the tool adds to the session: plain stdout on `SessionStart` and `UserPromptSubmit`, or a JSON reply's `additionalContext`.

<details>
<summary>How other tools read replies</summary>

### Gemini replies

It reads stdout, or stderr when stdout is empty. A JSON reply with `"decision": "deny"` or `"block"`, or `"continue": false`, is `block`, and so is plain text with an exit other than 0 and 1. No text at all, or exit 1, is `error`. `SessionStart`, `SessionEnd`, `Notification`, and `PreCompress` ignore decisions, so a block there reads as `error`. Only a JSON `additionalContext` reaches the model; plain stdout is a message for the user.
### Trae replies

As Claude Code.
### OpenHands replies

It blocks on exit 2, or on a JSON `"decision": "deny"` or `"continue": false` whatever the exit code, and only on `PreToolUse`, `UserPromptSubmit`, and `Stop`. A `context` line marks a JSON `additionalContext` on `UserPromptSubmit` and `Stop` only.
### Goose replies

It blocks on `PreToolUse` and `Stop` only: on exit 2, or on stdout starting with `{` whose `decision` is `"block"`, whatever the exit code. Exit 0 with empty stdout or `"decision": "allow"` allows. Anything else is `error`, or `block` when the action sets `x-goose.on_failure: block`.
### Cursor replies

It blocks on exit 2 on the permission events (`beforeShellExecution`, `beforeMCPExecution`, `beforeReadFile`, `beforeTabFileRead`, `subagentStart`, `preToolUse`) and `beforeSubmitPrompt`. At exit 0, a permission event blocks on `"permission": "deny"`, on `"ask"` (except on `preToolUse`, which does not enforce it), and on output that is not a valid JSON reply. No output at all fails open. `beforeSubmitPrompt` blocks on `"continue": false`. Any other failure fails open as `error`, or blocks with `failClosed: true`.
### Crush replies

Exit 2 blocks the tool call. Exit 49 blocks too and halts the whole turn, which a note says. Any other non-zero exit and a timeout do not block. At exit 0, a JSON reply with `"decision": "deny"` or `"halt": true` blocks. A reply with a `hookSpecificOutput` key is read as Claude Code's instead, and its top-level fields are ignored. A field of the wrong type voids the reply, which then allows. A `context` line marks a reply's `context` or `additionalContext`.
### Factory replies

It blocks on exit 2, except on `Notification`, `SubagentStop`, `PreCompact`, `SessionStart`, and `SessionEnd`, where exit 2 reads as `error`. At exit 0, `PreToolUse` blocks on `permissionDecision` `"deny"` or `"ask"`; `PostToolUse`, `UserPromptSubmit`, `Stop`, and `SubagentStop` block on `"decision": "block"`; and every event but `SessionEnd` blocks on `"continue": false`.
### Copilot replies

It drops `{"type": "progress"}` lines from stdout and parses the rest as one JSON reply; text that does not parse, or two objects, is no reply. `preToolUse` blocks on any non-zero exit, a command that did not start, `"permissionDecision": "deny"`, or `"ask"`. A failure other than exit 2 also counts as `error`, so `--expect block` fails on a hook that never ran. `permissionRequest` blocks on exit 2 or `"behavior": "deny"`; the commands' replies merge in order, so a later `"behavior": "allow"` overrides an earlier deny. `agentStop` and `subagentStop` block on `"decision": "block"`. Other events cannot block: exit 2 and other failures read as `error`, except exit 2 on `postToolUseFailure`, which adds context. A timeout fails open on every event. PascalCase events decide as their camelCase twins.
### Qoder replies

It blocks on exit 2 on `UserPromptSubmit`, `PreToolUse`, `Stop`, `SubagentStop`, `PreCompact`, `ConfigChange`, `Elicitation`, and `ElicitationResult`, and on any non-zero exit on `WorktreeCreate`. Other failures read as `error`, and `StopFailure` and `InstructionsLoaded` ignore the result. At exit 0, a JSON reply blocks on `"continue": false`, on `"decision": "deny"` where exit 2 blocks, on `PreToolUse` on `permissionDecision` `"deny"` or `"ask"` (which wins over `decision`), on `PermissionRequest` on a `decision.behavior` of `"deny"`, and on the elicitation events on an `action` of `"decline"` or `"cancel"`. A `hookSpecificOutput` without `hookEventName` voids the reply and reads as `error`. A `ConfigChange` with `source: policy_settings` cannot block, so a block there reads as `error`. `TaskCreated`, `TaskCompleted`, `TeammateIdle`, and `Setup` have no documented decision rules, so Qoder is listed as not run for them. A handler's `if` glob in parentheses matches the `command` of `Bash` or the `file_path` of a file tool; another tool with a glob is listed as not run.
### Antigravity replies

Only exit 0 with a JSON reply of the documented shape counts. On `PreToolUse`, `"decision": "allow"` reads as `allow` but is not counted: it defers to the next hook or default policy, which `hook run` cannot see. `"decision": "auto_approve"` counts as `allow`, with a note that explicit user rules still apply. `"deny"` blocks; `"force_ask"` blocks (a note says Antigravity asks the user). `"ask"` and `"deny_unless_prior_grant"` read as `block` but are not counted, since saved permissions can let the call run, and `hook run` cannot see them. On `Stop`, `"decision": "continue"` keeps the agent running and reads as `block`; any other value allows. `PostToolUse`, `SessionStart`, `PreInvocation`, and `PostInvocation` allow on a JSON object. `hook run` builds no `SessionStart` payload; pass `--payload <file>`. Anything else reads as `error` or `timeout` and is [not counted](@/docs/cli-reference/hook-diagnostics.md#assumed-results).
### Cline replies

It never reads the exit code. It reads stdout as JSON: the last line starting `HOOK_CONTROL` and a tab, else the whole stdout. On `PreToolUse` and `PostToolUse`, an object with `"cancel": true` blocks, and Cline stops the run; on `PostToolUse` the tool already ran. Empty stdout and any other JSON allow, whatever the exit code; a non-zero exit adds a note that Cline ignores it. Stdout that is not JSON reads as `error`, and a timeout as `timeout`, each with a note that Cline goes on as if the hook allowed. A `context` line marks a reply's `context`, `contextModification`, or `errorMessage`. When other specs reach Cline on the same event, sync writes them into one script that shares stdout, so the result is [not counted](@/docs/cli-reference/hook-diagnostics.md#assumed-results), even with `--include-assumed`.
### Kiro replies

CLI V3 blocks on exit 2 on `PreToolUse`. Another non-zero exit there reads as `error` and is [not counted](@/docs/cli-reference/hook-diagnostics.md#assumed-results), since Kiro's docs disagree on whether it blocks. `UserPromptSubmit` allows every exit code; a note explains that only the IDE blocks prompts. Non-zero prompt exits add the output and exit code to context. `Stop` reads exit 1 or an exit 0 JSON reply with `"decision": "block"` as `block`, since Kiro keeps the agent running (a note says so). Other non-zero exits on `PostToolUse` and `Stop` read as `error`. A `context` line marks prompt stdout, non-zero prompt exits, and a Stop continuation's supplied `reason`.
### Windsurf replies

Devin CLI blocks on exit 2, or on exit 0 with `"decision": "block"`, on `PreToolUse`, `PermissionRequest`, `UserPromptSubmit`, and `Stop`. `"decision": "approve"`, no decision, and plain stdout allow. Any other non-zero exit is `error`. A block on `PostToolUse`, `PostCompaction`, `PreCompact`, `PostCompact`, `SessionStart`, or `SessionEnd`, a reply on a non-zero exit (except exit 2 with `"decision": "block"`), and a `decision` other than `"approve"` or `"block"` are [not counted](@/docs/cli-reference/hook-diagnostics.md#assumed-results). A `PreToolUse` reply with `hookSpecificOutput.updatedInput` gets a note, since Devin CLI merges it into the tool's arguments. A `context` line marks `hookSpecificOutput.additionalContext` on `UserPromptSubmit`, `SessionStart`, and `PostToolUse`. Both count only when `hookSpecificOutput.hookEventName` names the event that ran.
### Augment replies

It blocks on exit 2 on `PreToolUse` only. It also blocks on exit 0 with `permissionDecision: "deny"`, a `decision: "block"` (inside `hookSpecificOutput` on `Stop` and `PostToolUse`), or `"continue": false`. A `context` line marks plain stdout on `SessionStart` and `hookSpecificOutput.additionalContext` at exit 0, as on Claude Code.

</details>

The run exits 1 when a counted command times out or errors, when two counted tools decide differently, or, with `--expect allow` or `--expect block`, when a counted tool decides otherwise. That makes it a CI check.

### Assumed results {#assumed-results}

Cursor, Copilot, Factory, Qoder, Antigravity, Kiro, and Windsurf (Devin CLI) leave out part of how a command runs. `hook run` runs them on these assumptions instead of skipping them:

- Shell (Cursor, Factory, Antigravity, Kiro, Windsurf, Copilot's `command` form, and Qoder without `shell`): `sh -c` on macOS and Linux, only for a script path with plain or single-quoted arguments. A command with shell syntax, such as a pipe, is listed as not run with a note to use a script path. Windows is not run. Copilot's and Qoder's exec form, which sync writes when the spec sets `args`, runs with no shell and assumes none.
- Timeout (Cursor, Windsurf): 30 seconds when the spec sets none. Set `timeout` in the spec to remove this assumption. Copilot and Antigravity document their 30 second default, and Factory and Kiro 60 seconds.
- Timeout (Kiro): 60 seconds for `timeout: 0`, which Kiro documents as no limit.
- Working directory (Copilot): the project root, for a hook without `cwd`. Set `cwd` in the spec to remove this assumption.
- Working directory (Factory, Qoder, Antigravity, Windsurf): the project root, since the docs do not say where a hook command runs. A Factory command written as `"$FACTORY_PROJECT_DIR"/path/to/script.sh`, or a Qoder command written as `"${QODER_PROJECT_DIR}"/path/to/script.sh`, counts as a script path and runs as written, with the variable set to the project root. Windsurf sets `DEVIN_PROJECT_DIR` the same way.
- Exit codes (Antigravity): the docs give no exit code a meaning. Only exit 0 with a reply of the documented shape is counted. Any other result, and the `"allow"`, `"ask"` and `"deny_unless_prior_grant"` replies, is shown with a `not counted` note and stays out of `--expect` and the comparison even with `--include-assumed`.
- Exit codes (Kiro): exit 2 blocks `PreToolUse`. Kiro's docs disagree on whether any other non-zero exit blocks it, so that result reads as `error` with a `not counted` note.
- Not run (Kiro): an `agent` action, which runs no command; a hook with `confirm`, which asks the user first; and every trigger but `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, and `Stop`, whose event data the docs do not show.
- Replies (Windsurf): results the Devin CLI docs give no meaning are shown with a `not counted` note.
- Exec path (Copilot): with `cwd` set, a relative exec path resolves from `cwd`.

### Counted and assumed results

A `not counted` result stays out of `--expect` and the comparison even with `--include-assumed`, though with `--include-assumed` a timeout or error from another of that tool's commands still fails the run.

The result line ends in `(assumed: shell, timeout)`, followed by one line per assumption. An assumed result is shown but not counted: it stays out of `--expect` and the comparison unless you pass `--include-assumed`. When it disagrees with the counted results, a warning says so, and a summary such as `0 checked, 1 assumed (not counted; --include-assumed to count)` shows what was left out. With `--expect`, a run where only assumed results ran fails and asks for `--include-assumed`, so a CI check never passes on nothing. In JSON, each target has `assumptions` (`item`, `value`, `reason`) and `counted`.

A Cursor hook spec uses Cursor's own event names, so other tools are listed as not run. A Copilot hook spec with a PascalCase event such as `PreToolUse` runs on Claude Code too, which counts, so `--expect` checks Claude Code and shows Copilot beside it.

A Cline run assumes only the working directory: the project root, while Cline uses the directory the CLI started in. The VS Code extension does not read stdout when the script exits non-zero, so a reply printed before a failing exit blocks in the CLI and not in the extension.

A Crush run assumes nothing until a command reaches one of Crush's built-in programs: `jq` and, on Windows, `cat`, `ls`, `rm`, `find`, and the other core utilities, unless `CRUSH_CORE_UTILS=false`. `hook run` runs the ones on `PATH` instead, and adds a `jq` or `coreutils` assumption for each it reached. This applies only to inline commands and `./` scripts without a shebang.

## JSON output

`--format json` prints the same results as one JSON object, for a CI job to read. Each target has a `decision` (`allow`, `block`, `error`, `timeout`, or `not run` with a `reason`), its `warnings`, its `assumptions`, whether it is `counted`, and one entry per command. `exit_code` is `null` after a timeout or a command that did not start. `error` holds the reason the run fails. The exit code is the same as in text:

```json
{
  "hook": "protect-files",
  "targets": [
    {
      "target": "claude",
      "decision": "block",
      "event": "PreToolUse",
      "trigger": "Write",
      "async": false,
      "commands": [
        {
          "command": ".claude/hooks/protect-files.sh",
          "decision": "block",
          "exit_code": 2,
          "elapsed_ms": 12,
          "timed_out": false,
          "stdout": "",
          "stderr": "Blocked: .github/workflows/tests.yml is protected.\n",
          "adds_context": false
        }
      ],
      "notes": [],
      "warnings": [],
      "assumptions": [],
      "counted": true
    }
  ]
}
```

## Sample event inputs

A tool that lacks the input a flag needs is listed as not run, and the other tools still run. In each row, `tool_response` holds placeholder values, and session IDs and transcript paths are made up.

### Claude Code sample events

`--edit` calls the first of `Write`, `Edit`, and `MultiEdit` the matcher matches, with an absolute `tool_input.file_path`. `SubagentStart` needs no input and starts the first of `general-purpose`, `Explore`, and `Plan` the matcher matches.

### Codex sample events

`--edit` calls `apply_patch` with an `*** Add File:` or `*** Update File:` patch in `tool_input.command`, and `Edit` and `Write` match it.

### Gemini sample events

`--edit` calls the first of `write_file` and `replace` the matcher matches, with an absolute `tool_input.file_path`; `--bash` calls `run_shell_command`. The tool matcher is an unanchored regular expression, and a `SessionStart` source must match exactly.

### Trae sample events

`--bash` calls `RunCommand`. Trae lists no edit tool input, so `--edit` lists Trae as not run. The matcher is an unanchored regular expression.

### OpenHands sample events

`--bash` calls `terminal`. The file editor's input is undocumented, so `--edit` lists OpenHands as not run. A `*` or empty matcher matches all; one with a regex character, or written `/re/`, must match the whole tool name; any other is an exact name.

### Goose sample events

`--bash` calls `shell`; `--edit` calls the first of `write` and `edit` the matcher matches. `BeforeShellExecution` and `AfterShellExecution` take `--bash`, and `AfterFileEdit` and `BeforeReadFile` take `--edit`. The matcher is an unanchored regular expression on `matcher_context`; one that does not compile, such as `*`, skips the rule.

### Cursor sample events

`--bash` builds `beforeShellExecution` and `afterShellExecution`, and `preToolUse` and `postToolUse` on the `Shell` tool. `--edit` builds `afterFileEdit` only, so on `preToolUse` and `postToolUse` Cursor is listed as not run. `--prompt` builds `beforeSubmitPrompt`. The matcher is an unanchored regular expression.

### Crush sample events

Only `PreToolUse` runs. `--bash` calls `bash`; `--edit` calls the first of `edit` and `write` the matcher matches. Crush has no prompt event. The matcher is an unanchored regular expression on the lowercase tool name; one that does not compile makes Crush skip the hook, and `hook run` reports it.

### Factory sample events

`--bash` calls `Execute`. `--edit` calls `Create`; when the matcher picks `Edit` or `ApplyPatch`, Factory is listed as not run. `--prompt` builds `UserPromptSubmit`. Empty or `*` matches everything, letters and `\|` list exact names, and anything else is a case-sensitive regular expression.

### Antigravity sample events

`--bash` calls `run_command`. `--edit` calls the first of `write_to_file`, `replace_file_content`, and `multi_replace_file_content` the matcher matches. Antigravity has no prompt event; `PreInvocation`, `PostInvocation`, and `Stop` take `--payload`. The matcher is a regular expression matched against the whole tool name; empty or `*` matches everything.

### Copilot sample events

`--bash` builds `preToolUse` and `postToolUse` on `bash`, and `PreToolUse` on `Bash`. `--prompt` builds `userPromptSubmitted` or `UserPromptSubmit`. `--edit` is refused, and so is `--bash` on `PostToolUse`. A camelCase matcher is a regex anchored as `^(?:PATTERN)$`. Copilot runs JavaScript regexes, so a matcher with inline flags, lookaround, backreferences, `(?P<name>)`, POSIX classes, or Go-only escapes such as `\A` is listed as not run.

### Qoder sample events

`--bash` calls `Bash`. `--edit` calls `Write`; when the matcher picks `Edit`, Qoder is listed as not run. Empty or `*` matches everything, letters and `\|` list exact names, and anything else is a regular expression.

### Cline sample events

`--bash` calls `run_commands`; `--edit` calls `editor` (a model whose id holds `gpt` or `codex` calls `apply_patch` instead, which a note says). `--prompt` builds `UserPromptSubmit`, with a note that the CLI may not send that event. Cline has no matcher, so every hook fires; a portable `match` kind runs the script's tool name check, which allows a call to another tool. `PreCompact` is listed as not run.

### Kiro sample events

`--prompt` supplies `UserPromptSubmit`; `Stop` needs no input. Tool events require `--payload`; `--bash` and `--edit` are refused. CLI V3 ignores prompt and Stop matchers. See [Kiro matchers](@/docs/cli-reference/hook-diagnostics.md#kiro-hook-run-matchers).

### Windsurf sample events

`--bash` builds `PreToolUse`, `PostToolUse`, and `PermissionRequest` on `exec`. `--prompt` builds `UserPromptSubmit`; `Stop` and `PostCompaction` need no input. `--edit` is refused. `SessionStart`, `SessionEnd`, `PreCompact`, and `PostCompact` take `--payload`. The matcher is an unanchored regular expression on `tool_name`, so a Claude-style `Bash` matcher does not fire; `PreCompact` and `PostCompact` accept `manual` or `auto`; a matcher on another event is listed as not run.

### Augment sample events

`--bash` calls `launch-process`; `--edit` calls the first of `str-replace-editor` and `save-file` the matcher matches. Augment has no prompt event. The matcher is an unanchored regular expression.

## Matcher and plugin limits

Gemini matchers compile as Go regular expressions, which reject a few JavaScript forms such as lookahead. Gemini CLI would run those, and `hook run` compares them as a literal name. `GEMINI_PLANS_DIR` is not set.

OpenCode and Kilo run hooks as plugins and Zed as tasks, so `hook run` lists them as not run. OpenCode V2 plugins receive native event JSON on stdin; the command does not simulate their runtime.

### Kiro matchers {#kiro-hook-run-matchers}

Kiro tool matchers without regular-expression syntax select tool IDs, the documented `read`, `write`, `shell`, and `web` categories, or source tags. They support `*` and `?` wildcards and also try an unanchored regular expression against the tool ID. A matcher with regular-expression syntax uses that expression only.

`@mcp`, `@builtin`, `@server`, and `@server/tool` select MCP or built-in tools. `execute_bash` and `execute_pwsh` selectors match each other. CLI 2.x aliases are not expanded.

`@powers`, `spec`, `subagent`, and `context` are listed as not run because their complete tool membership is undocumented. A named MCP selector is also not run when underscores make its server/tool boundary ambiguous. Use an anchored regular expression against the internal tool ID.

## Sources

Vendor docs and pinned source each tool's hook behavior is checked against:

- **Claude Code:** [Hooks guide](https://code.claude.com/docs/en/hooks-guide).
- **Codex:** [Hooks](https://learn.chatgpt.com/docs/hooks).
- **Copilot:** [Hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference), [using hooks with Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks).
- **Cursor:** [Hooks](https://cursor.com/docs/hooks).
- **Gemini CLI:** [Hooks reference](https://geminicli.com/docs/hooks/reference/), [file system tools](https://geminicli.com/docs/tools/file-system/), source [`c6bccb7`](https://github.com/google-gemini/gemini-cli/tree/c6bccb7ecbf6d8368d995455dd725ed34466faad/packages/core/src/hooks).
- **Qoder:** [Hooks](https://docs.qoder.com/cli/hooks), [hooks reference](https://docs.qoder.com/cli/hooks-reference).
- **Factory:** [Hooks](https://docs.factory.com/harness/hooks).
- **Devin CLI:** [Hooks](https://docs.devin.ai/cli/extensibility/hooks), [commands triggered by session events](https://docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks).
- **Antigravity:** [Hooks](https://antigravity.google/docs/hooks).
- **Kiro:** [Hooks](https://kiro.dev/docs/hooks), [hook types](https://kiro.dev/docs/hooks/types), [hook actions](https://kiro.dev/docs/hooks/actions), [CLI V3 migration](https://kiro.dev/docs/cli/v3/hooks-migration), [built-in tools](https://kiro.dev/docs/tools).
- **Trae:** [automate actions with hooks](https://docs.trae.cn/ide_automate-actions-with-hooks), [hook reference](https://docs.trae.cn/ide_hook-configuration-reference).
- **Augment:** [Hooks](https://docs.augmentcode.com/cli/hooks).
- **OpenHands:** [Hooks](https://docs.openhands.dev/openhands/usage/customization/hooks), source [`fad6377`](https://github.com/OpenHands/software-agent-sdk/tree/fad63774459171b08f889b12fd3b4d6346168c3e/openhands-sdk/openhands/sdk/hooks).
- **Goose:** [Hooks](https://goose-docs.ai/docs/guides/context-engineering/hooks/), source [`bab8ff6`](https://github.com/aaif-goose/goose/blob/bab8ff641039c9cd3331121cd84a5c6045f365ca/documentation/docs/guides/context-engineering/hooks.md) and [runner](https://github.com/aaif-goose/goose/blob/bab8ff641039c9cd3331121cd84a5c6045f365ca/crates/goose/src/hooks/mod.rs).
- **Cline:** source [`39ff235`](https://github.com/cline/cline/tree/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks): [`hook-file-hooks.ts`](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks/hook-file-hooks.ts#L234-L254), [`subprocess-runner.ts`](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks/subprocess-runner.ts#L68-L97), [tool schemas](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/extensions/tools/schemas.ts#L149-L224).
- **Crush:** source [`76cc5c5`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal): [`config/config.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/config/config.go), [`hooks/input.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/hooks/input.go), [`hooks/runner.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/hooks/runner.go), [`shell/dispatch.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/shell/dispatch.go), [`shell/run.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/shell/run.go).
- **`hook run` shell:** [mvdan.cc/sh](https://github.com/mvdan/sh) and its [core utilities](https://github.com/mvdan/sh/blob/b5028a3332a4d5d5a6cc62a999c1ba7993eb3627/x/coreutils/coreutils.go), [gojq](https://github.com/itchyny/gojq).
