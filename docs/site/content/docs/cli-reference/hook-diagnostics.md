+++
title = "Test hook behavior"
description = "Run sample hook events and understand diagnostic results and runtime limits."
weight = 55

[extra]
nav_title = "Test hooks"
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

### Build sample event data {#event-data-replies}

`--edit <path>` and `--bash <command>` build a `PreToolUse` or `PostToolUse` tool call (`BeforeTool` or `AfterTool` on Gemini). `--prompt <text>` builds `UserPromptSubmit` (`BeforeAgent` on Gemini). `SessionStart` takes its `source` from the matcher, or `startup` when the matcher is empty. `--payload <file>` sends a JSON file as is, for any event.

### Apply matchers {#matcher-replies}

When the matcher does not match the tool or source, that tool reports `allow` and why, since it would not run the hook. A `--payload` tool call matches its `tool_name` by the tool's own rules, including Codex's `Edit` and `Write` aliases for `apply_patch`. Cursor, Goose, Copilot, and Kiro match the field their docs name for the event, such as the command, file path, or prompt text.

### Skip unsupported events {#missing-event-replies}

A tool without the hook's event, such as Codex for `Notification`, is not run.

### Use the configured timeout {#timeout-replies}

The command uses the timeout sync writes, or the tool's default in the table below.

### Report background hooks {#async-replies}

An `async: true` hook, or `asyncRewake: true` on Claude Code and Qoder, runs and prints its output, but the tool does not wait for it. Its result is `not judged` and stays out of `--expect` and the comparison. The same holds for Cursor `sessionStart` and `sessionEnd`, Copilot `notification`, and Cline hooks on any event but `PreToolUse` and `PostToolUse`.

### Stop leftover background commands {#background-commands-replies}

On macOS and Linux, a command the hook leaves running is killed once the hook exits.

### Find stale generated hook files {#stale-tool-file-replies}

Commands come from the spec, so a command that calls a hook script needs a `sync` first to copy the script. A `warning` names the synced file when it is missing, when no handler under the event runs the spec's command, or when that handler's group matcher, timeout, or (on Gemini) `env` differs from the spec. It also compares `failClosed` on Claude Code and Cursor, `cwd` on Copilot, and `confirm` on Kiro. An `env` warning names keys, never values. A warning does not fail the run; `sync` clears it. The file is the one each [tool page](@/docs/targets/_index.md) lists, or the path `outputs` sets.

<details>
<summary>Environment variables per tool</summary>

`hook run` clears these variables from the calling shell's env, then sets them per tool. Other variables pass through.

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
| Crush | `CRUSH=1`, `AGENT=crush`, `AI_AGENT=crush`, `CRUSH_EVENT`, `CRUSH_TOOL_NAME`, `CRUSH_SESSION_ID`, `CRUSH_CWD`, `CRUSH_PROJECT_DIR`, `CRUSH_TOOL_INPUT_COMMAND` or `CRUSH_TOOL_INPUT_FILE_PATH` when the tool input has one, and editor and pager variables that keep anything from waiting for a terminal; `AGNOSTIC_AI_TARGET=crush` comes from its command |
| Copilot | the handler's `env`, which holds `AGNOSTIC_AI_TARGET=copilot`. Copilot is not run when a value holds `$`. |
| Factory | `FACTORY_PROJECT_DIR` |
| Qoder | `QODER_PROJECT_DIR`, then the handler's `env`, which holds `AGNOSTIC_AI_TARGET=qoder` |
| Antigravity | none |
| Cline | none; the script exports `AGNOSTIC_AI_TARGET=cline` |
| Kiro | `USER_PROMPT` on `UserPromptSubmit` |
| Windsurf | `DEVIN_PROJECT_DIR` |

</details>

<details>
<summary>Shell and default timeout per tool</summary>

"Script path only" and "assumed" cells are [assumed results](@/docs/cli-reference/hook-diagnostics.md#assumed-results).

| Target | Shell | Default timeout |
|--------|-------|-----------------|
| Claude Code | `bash -c`, on Windows with Git Bash (`CLAUDE_CODE_GIT_BASH_PATH`, else the Git install that holds `git.exe`), or PowerShell when there is none and the spec sets no `shell`; exec-form `args` with no shell; `shell: powershell` with PowerShell | 600 seconds, except 30 on `UserPromptSubmit`, `PreModelSwitch`, and `PostModelSwitch`, and 10 on `MessageDisplay` |
| Codex | `sh -c`; on Windows, `commandWindows` with `powershell.exe -Command` | 600 seconds |
| Gemini | `bash -c`, or Windows PowerShell on Windows, after Gemini replaces `$GEMINI_PROJECT_DIR`, `$GEMINI_CWD`, `$GEMINI_SESSION_ID`, and `$CLAUDE_PROJECT_DIR` with the quoted root | 60 seconds |
| Trae | `bash -c`, or PowerShell on Windows | 30 seconds |
| OpenHands | `/bin/sh -c`, or `cmd.exe /c` on Windows | 60 seconds |
| Goose | `sh -c` on every platform | 30 seconds |
| Augment | A `.sh`, `.ps1`, `.cmd`, or `.bat` script path only, run directly on macOS and Linux, and on Windows with `powershell.exe -Command` (`.ps1`) or `cmd.exe /c` (`.cmd`, `.bat`). An inline command is not run. | 60 seconds |
| Cursor | Script path only | Assumed 30 seconds |
| Crush | Crush's embedded POSIX shell, on every platform. A path starting with `./`, `../`, or `/` runs through its shebang interpreter, or with no shebang as shell source. A path without that prefix, such as `hooks/guard.sh`, starts as a program; on macOS and Linux one without a shebang then fails with exit 1. | 30 seconds |
| Copilot | Exec-form `args` with no shell. A `command` is script path only. | 30 seconds |
| Factory | Script path only, where `"$FACTORY_PROJECT_DIR"` counts as a plain word | 60 seconds |
| Qoder | Exec-form `args` with no shell; `shell: bash` with `bash -c` on macOS and Linux. Without `shell`, script path only, where `"$QODER_PROJECT_DIR"` or `"${QODER_PROJECT_DIR}"` counts as a plain word. `shell: powershell`, exec form with a `$` in `command` or `args`, and a shell-form command on Windows are not run. | 600 seconds |
| Antigravity | Script path only | 30 seconds |
| Cline | `bash`, running the event script sync writes. Not run on Windows. | 120 seconds on `PreToolUse` and `PostToolUse`; sync drops `timeout` |
| Kiro | Script path only | 60 seconds, and assumed 60 for `timeout: 0` |
| Windsurf | Script path only | Assumed 30 seconds |

</details>

<details>
<summary>Claude Code `if` rules</summary>

A handler's `if` permission rule decides whether it runs, as in Claude Code. It applies only on `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, and `PermissionDenied`. See the [`if` field](https://code.claude.com/docs/en/hooks) and [permission rule syntax](https://code.claude.com/docs/en/permissions#permission-rule-syntax).

- `Bash(git *)` matches any part of a command split by `&&`, `||`, `;`, `|`, and `&`, and commands inside `$()` and backticks, past leading `VAR=value` assignments and wrappers such as `timeout 30`. A trailing ` *` also matches the bare command, and `:*` equals ` *`.
- A pattern that names more than the command runs on any `$()`, backtick, or `$VAR`. A command that does not parse always runs.
- `Edit(path)` covers `Edit`, `Write`, `MultiEdit`, and `NotebookEdit`, with gitignore patterns: `//path` from the filesystem root, `~/path` from home, `/path` and `path` from the project root, and a bare name such as `.env` at any depth. `Tool(param:value)` matches a top-level input field.

</details>

### Decisions {#decisions}

Each command reports one decision.

- `block`: exit 2, or exit 0 with a JSON reply that has `"permissionDecision": "deny"`, `"decision": "block"`, or `"continue": false`.
- `allow`: any other exit 0.
- `error`: any other non-zero exit.
- `timeout`: the command ran past its timeout.

With `failClosed: true`, Claude Code treats a hook failure as exit 2 for its event, and on `PermissionRequest` denies the request. [Claude Code hook mappings](@/docs/targets/claude-reference.md#hook-mappings) lists the events and handlers that ignore it.

Exit 2 cannot stop anything on `SessionStart`, `SessionEnd`, `Notification`, `PreCompact`, and `PostCompact`, so it reads as `error` there. On `PostToolUse` the tool already ran, so `block` sends stderr back to the model. A `context` line marks output the tool adds to the session: plain stdout on `SessionStart` and `UserPromptSubmit`, or a JSON reply's `additionalContext`.

<details>
<summary>How other tools read replies</summary>

### Gemini replies

Reads stdout, or stderr when stdout is empty. A JSON `"decision": "deny"` or `"block"`, `"continue": false`, or plain text with an exit other than 0 and 1 is `block`. No text, or exit 1, is `error`. `SessionStart`, `SessionEnd`, `Notification`, and `PreCompress` ignore decisions, so a block there reads as `error`. Only a JSON `additionalContext` reaches the model; plain stdout goes to the user.
### Trae replies

As Claude Code.
### OpenHands replies

Blocks only on `PreToolUse`, `UserPromptSubmit`, and `Stop`: on exit 2, or a JSON `"decision": "deny"` or `"continue": false` at any exit code. A `context` line marks a JSON `additionalContext` on `UserPromptSubmit` and `Stop` only.
### Goose replies

Blocks only on `PreToolUse` and `Stop`: on exit 2, or stdout starting with `{` whose `decision` is `"block"`, at any exit code. Exit 0 with empty stdout or `"decision": "allow"` allows. Anything else is `error`, or `block` with `x-goose.on_failure: block`.
### Cursor replies

Exit 2 blocks on `beforeSubmitPrompt` and the permission events: `beforeShellExecution`, `beforeMCPExecution`, `beforeReadFile`, `beforeTabFileRead`, `subagentStart`, and `preToolUse`. At exit 0, a permission event blocks on `"permission": "deny"`, on `"ask"` (except `preToolUse`, which does not enforce it), and on output that is not a valid JSON reply; no output fails open. `beforeSubmitPrompt` blocks on `"continue": false`. Other failures fail open as `error`, or block with `failClosed: true`.
### Crush replies

Exit 2 blocks the tool call; exit 49 also halts the whole turn, with a note. Other non-zero exits and a timeout do not block. At exit 0, a JSON `"decision": "deny"` or `"halt": true` blocks. A reply with a `hookSpecificOutput` key reads as Claude Code's, and its top-level fields are ignored. A field of the wrong type voids the reply, which then allows. A `context` line marks `context` or `additionalContext`.
### Factory replies

Exit 2 blocks, except on `Notification`, `SubagentStop`, `PreCompact`, `SessionStart`, and `SessionEnd`, where it reads as `error`. At exit 0, `PreToolUse` blocks on `permissionDecision` `"deny"` or `"ask"`; `PostToolUse`, `UserPromptSubmit`, `Stop`, and `SubagentStop` on `"decision": "block"`; and every event but `SessionEnd` on `"continue": false`.
### Copilot replies

Drops `{"type": "progress"}` lines from stdout and parses the rest as one JSON reply; text that does not parse, or two objects, is no reply. `preToolUse` blocks on any non-zero exit, a command that did not start, or `"permissionDecision"` `"deny"` or `"ask"`. A failure other than exit 2 also counts as `error`, so `--expect block` fails on a hook that never ran. `permissionRequest` blocks on exit 2 or `"behavior": "deny"`; replies merge in order, so a later `"behavior": "allow"` overrides an earlier deny. `agentStop` and `subagentStop` block on `"decision": "block"`. Other events cannot block: failures read as `error`, except exit 2 on `postToolUseFailure`, which adds context. A timeout fails open everywhere. PascalCase events decide as their camelCase twins.
### Qoder replies

Exit 2 blocks on `UserPromptSubmit`, `PreToolUse`, `Stop`, `SubagentStop`, `PreCompact`, `ConfigChange`, `Elicitation`, and `ElicitationResult`, and any non-zero exit blocks on `WorktreeCreate`. Other failures read as `error`; `StopFailure` and `InstructionsLoaded` ignore the result. At exit 0, a JSON reply blocks on:

- `"continue": false`
- `"decision": "deny"`, where exit 2 blocks
- `permissionDecision` `"deny"` or `"ask"` on `PreToolUse`, which wins over `decision`
- a `decision.behavior` of `"deny"` on `PermissionRequest`
- an `action` of `"decline"` or `"cancel"` on the elicitation events

A `hookSpecificOutput` without `hookEventName` voids the reply as `error`. A `ConfigChange` with `source: policy_settings` cannot block, so a block there reads as `error`. `TaskCreated`, `TaskCompleted`, `TeammateIdle`, and `Setup` have no documented decision rules, so Qoder is not run for them. A handler's `if` glob in parentheses matches the `command` of `Bash` or the `file_path` of a file tool; a glob on another tool is not run.
### Antigravity replies

Only exit 0 with a JSON reply of the documented shape counts; anything else reads as `error` or `timeout`, not counted. On `PreToolUse`, `"decision": "auto_approve"` is `allow`, with a note that explicit user rules still apply; `"deny"` blocks; `"force_ask"` blocks, with a note that Antigravity asks the user. `"allow"` (which defers to the next hook or default policy) reads as `allow`, and `"ask"` and `"deny_unless_prior_grant"` (which saved permissions can let through) read as `block`; `hook run` cannot see either, so none of the three is counted. On `Stop`, `"decision": "continue"` keeps the agent running and reads as `block`; any other value allows. `PostToolUse`, `SessionStart`, `PreInvocation`, and `PostInvocation` allow on a JSON object. `hook run` builds no `SessionStart` payload; pass `--payload <file>`.
### Cline replies

Ignores the exit code and reads stdout as JSON: the last line starting `HOOK_CONTROL` and a tab, else the whole stdout. On `PreToolUse` and `PostToolUse`, `"cancel": true` blocks and stops the Cline run; on `PostToolUse` the tool already ran. Empty stdout and other JSON allow, and a non-zero exit adds a note that Cline ignores it. Stdout that is not JSON reads as `error` and a timeout as `timeout`, each with a note that Cline goes on as if the hook allowed. A `context` line marks `context`, `contextModification`, or `errorMessage`. When other specs reach Cline on the same event, sync writes them into one script that shares stdout, so the result is not counted, even with `--include-assumed`.
### Kiro replies

CLI V3 blocks on exit 2 on `PreToolUse`. Another non-zero exit there reads as `error`, not counted, since Kiro's docs disagree on whether it blocks. `UserPromptSubmit` allows every exit code, with a note that only the IDE blocks prompts; a non-zero exit adds the output and exit code to context. `Stop` reads exit 1, or exit 0 with `"decision": "block"`, as `block`, with a note that Kiro keeps the agent running. Other non-zero exits on `PostToolUse` and `Stop` read as `error`. A `context` line marks prompt stdout, non-zero prompt exits, and a Stop continuation's `reason`.
### Windsurf replies

Devin CLI blocks on exit 2, or exit 0 with `"decision": "block"`, on `PreToolUse`, `PermissionRequest`, `UserPromptSubmit`, and `Stop`. `"decision": "approve"`, no decision, and plain stdout allow. Another non-zero exit is `error`. Not counted: a block on `PostToolUse`, `PostCompaction`, `PreCompact`, `PostCompact`, `SessionStart`, or `SessionEnd`; a reply on a non-zero exit, except exit 2 with `"decision": "block"`; and a `decision` other than `"approve"` or `"block"`. A `PreToolUse` reply with `hookSpecificOutput.updatedInput` gets a note, since Devin CLI merges it into the tool's arguments. A `context` line marks `hookSpecificOutput.additionalContext` on `UserPromptSubmit`, `SessionStart`, and `PostToolUse`. Both count only when `hookSpecificOutput.hookEventName` names the event that ran.
### Augment replies

Exit 2 blocks on `PreToolUse` only. Exit 0 blocks with `permissionDecision: "deny"`, `decision: "block"` (inside `hookSpecificOutput` on `Stop` and `PostToolUse`), or `"continue": false`. A `context` line marks plain stdout on `SessionStart` and `hookSpecificOutput.additionalContext` at exit 0, as on Claude Code.

</details>

The run exits 1, so it works as a CI check, when a counted command times out or errors, when two counted tools decide differently, or, with `--expect allow` or `--expect block`, when a counted tool decides otherwise.

### Assumed results {#assumed-results}

Some tools' docs leave out part of how a command runs. `hook run` runs those commands on an assumption instead of skipping them, ends the result line in `(assumed: shell, timeout)`, and adds one line per assumption. An assumed result stays out of `--expect` and the comparison unless you pass `--include-assumed`. A warning flags an assumed result that disagrees with the counted ones, and a summary such as `0 checked, 1 assumed (not counted; --include-assumed to count)` shows what was left out. With `--expect`, a run where only assumed results ran fails and asks for `--include-assumed`, so a CI check never passes on nothing. In JSON, each target has `assumptions` (`item`, `value`, `reason`) and `counted`.

| Assumption | Targets | What `hook run` assumes |
|---|---|---|
| Shell | Cursor, Factory, Antigravity, Kiro, Windsurf, Copilot's `command` form, Qoder without `shell` | `sh -c` on macOS and Linux, only for a script path with plain or single-quoted arguments. A command with shell syntax, such as a pipe, is not run, with a note to use a script path; Windows is not run. Copilot's and Qoder's exec form (`args`) runs with no shell and assumes none. |
| Timeout | Cursor, Windsurf | 30 seconds when the spec sets none. Set `timeout` to remove it. |
| Timeout | Kiro | 60 seconds for `timeout: 0`, which Kiro documents as no limit. |
| Working directory | Copilot | The project root for a hook without `cwd`. With `cwd`, a relative exec path resolves from it. |
| Working directory | Factory, Qoder, Antigravity, Windsurf | The project root, since the docs do not say. A command starting `"$FACTORY_PROJECT_DIR"/` or `"${QODER_PROJECT_DIR}"/` counts as a script path, with the variable set to the root; Windsurf sets `DEVIN_PROJECT_DIR` the same way. |
| Working directory | Cline | The project root, where Cline uses the directory the CLI started in. |
| `jq`, `coreutils` | Crush | Crush has built-in `jq` and, on Windows, `cat`, `ls`, `rm`, `find`, and the other core utilities, unless `CRUSH_CORE_UTILS=false`. `hook run` runs the ones on `PATH` and adds an assumption for each it reached, only for inline commands and `./` scripts without a shebang. |

### Counted and assumed results

A `not counted` result is one the replies above mark `not counted`. It stays out of `--expect` and the comparison even with `--include-assumed`, though with that flag a timeout or error from another of that tool's commands still fails the run.

Kiro is not run for an `agent` action, which runs no command; a hook with `confirm`, which asks the user first; and every trigger but `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, and `Stop`, whose event data the docs do not show.

A Cursor hook spec uses Cursor's own event names, so other tools are not run. A Copilot hook spec with a PascalCase event such as `PreToolUse` also runs on Claude Code, which counts, so `--expect` checks Claude Code and shows Copilot beside it.

The Cline VS Code extension does not read stdout when the script exits non-zero, so a reply printed before a failing exit blocks in the CLI and not in the extension.

## JSON output

`--format json` prints the same results as one JSON object. Each target has a `decision` (`allow`, `block`, `error`, `timeout`, or `not run` with a `reason`), its `warnings`, its `assumptions`, whether it is `counted`, and one entry per command. `exit_code` is `null` after a timeout or a command that did not start. `error` holds the reason the run fails. The exit code is the same as in text:

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

A tool that lacks the input a flag needs is not run, and the other tools still run. In each row, `tool_response` holds placeholder values, and session IDs and transcript paths are made up. Where a section lists several tool names, `hook run` calls the first one the matcher matches. Gemini, Trae, Goose, Cursor, Crush, Windsurf, and Augment read the matcher as an unanchored regular expression.

### Claude Code sample events

`--edit` calls `Write`, `Edit`, or `MultiEdit`, with an absolute `tool_input.file_path`. `SubagentStart` needs no input and starts `general-purpose`, `Explore`, or `Plan`.

### Codex sample events

`--edit` calls `apply_patch` with an `*** Add File:` or `*** Update File:` patch in `tool_input.command`, and `Edit` and `Write` match it.

### Gemini sample events

`--edit` calls `write_file` or `replace`, with an absolute `tool_input.file_path`; `--bash` calls `run_shell_command`. A `SessionStart` source must match exactly.

### Trae sample events

`--bash` calls `RunCommand`. Trae lists no edit tool input, so Trae is not run on `--edit`.

### OpenHands sample events

`--bash` calls `terminal`. The file editor's input is undocumented, so OpenHands is not run on `--edit`. A `*` or empty matcher matches all; one with a regex character, or written `/re/`, must match the whole tool name; any other is an exact name.

### Goose sample events

`--bash` calls `shell`; `--edit` calls `write` or `edit`. `BeforeShellExecution` and `AfterShellExecution` take `--bash`, and `AfterFileEdit` and `BeforeReadFile` take `--edit`. The matcher applies to `matcher_context`; one that does not compile, such as `*`, skips the rule.

### Cursor sample events

`--bash` builds `beforeShellExecution` and `afterShellExecution`, and `preToolUse` and `postToolUse` on the `Shell` tool. `--edit` builds `afterFileEdit` only, so on `preToolUse` and `postToolUse` Cursor is not run. `--prompt` builds `beforeSubmitPrompt`.

### Crush sample events

Only `PreToolUse` runs, and Crush has no prompt event. `--bash` calls `bash`; `--edit` calls `edit` or `write`. The matcher applies to the lowercase tool name; one that does not compile makes Crush skip the hook, and `hook run` reports it.

### Factory sample events

`--bash` calls `Execute`. `--edit` calls `Create`; when the matcher picks `Edit` or `ApplyPatch`, Factory is not run. `--prompt` builds `UserPromptSubmit`. Empty or `*` matches everything, letters and `\|` list exact names, and anything else is a case-sensitive regular expression.

### Antigravity sample events

`--bash` calls `run_command`; `--edit` calls `write_to_file`, `replace_file_content`, or `multi_replace_file_content`. Antigravity has no prompt event; `PreInvocation`, `PostInvocation`, and `Stop` take `--payload`. The matcher must match the whole tool name; empty or `*` matches everything.

### Copilot sample events

`--bash` builds `preToolUse` and `postToolUse` on `bash`, and `PreToolUse` on `Bash`. `--prompt` builds `userPromptSubmitted` or `UserPromptSubmit`. `--edit` is refused, and so is `--bash` on `PostToolUse`. A camelCase matcher is anchored as `^(?:PATTERN)$`. Copilot runs JavaScript regexes, so a matcher with inline flags, lookaround, backreferences, `(?P<name>)`, POSIX classes, or Go-only escapes such as `\A` is not run.

### Qoder sample events

`--bash` calls `Bash`. `--edit` calls `Write`; when the matcher picks `Edit`, Qoder is not run. Empty or `*` matches everything, letters and `\|` list exact names, and anything else is a regular expression.

### Cline sample events

`--bash` calls `run_commands`; `--edit` calls `editor` (a model whose id holds `gpt` or `codex` calls `apply_patch` instead, which a note says). `--prompt` builds `UserPromptSubmit`, with a note that the CLI may not send that event. Cline has no matcher, so every hook fires; a portable `match` kind runs the script's tool name check, which allows a call to another tool. `PreCompact` is not run.

### Kiro sample events

`--prompt` supplies `UserPromptSubmit`; `Stop` needs no input. Tool events require `--payload`; `--bash` and `--edit` are refused. CLI V3 ignores prompt and Stop matchers. See [Kiro matchers](@/docs/cli-reference/hook-diagnostics.md#kiro-hook-run-matchers).

### Windsurf sample events

`--bash` builds `PreToolUse`, `PostToolUse`, and `PermissionRequest` on `exec`. `--prompt` builds `UserPromptSubmit`; `Stop` and `PostCompaction` need no input. `--edit` is refused. `SessionStart`, `SessionEnd`, `PreCompact`, and `PostCompact` take `--payload`. The matcher applies to `tool_name`, so a Claude-style `Bash` matcher does not fire. `PreCompact` and `PostCompact` accept `manual` or `auto`; a matcher on another event is not run.

### Augment sample events

`--bash` calls `launch-process`; `--edit` calls `str-replace-editor` or `save-file`. Augment has no prompt event.

## Matcher and plugin limits

Gemini matchers compile as Go regular expressions, which reject a few JavaScript forms such as lookahead. Gemini CLI would run those, and `hook run` compares them as a literal name. `GEMINI_PLANS_DIR` is not set.

OpenCode and Kilo run hooks as plugins and Zed as tasks, so they are not run. OpenCode V2 plugins receive native event JSON on stdin; the command does not simulate their runtime.

### Kiro matchers {#kiro-hook-run-matchers}

Kiro tool matchers without regular-expression syntax select tool IDs, the documented `read`, `write`, `shell`, and `web` categories, or source tags. They support `*` and `?` wildcards and also try an unanchored regular expression against the tool ID. A matcher with regular-expression syntax uses that expression only.

`@mcp`, `@builtin`, `@server`, and `@server/tool` select MCP or built-in tools. `execute_bash` and `execute_pwsh` selectors match each other. CLI 2.x aliases are not expanded.

`@powers`, `spec`, `subagent`, and `context` are not run because their complete tool membership is undocumented. A named MCP selector is also not run when underscores make its server/tool boundary ambiguous. Use an anchored regular expression against the internal tool ID.

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
