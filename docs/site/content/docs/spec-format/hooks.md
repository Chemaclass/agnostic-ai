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

Sync never runs a hook; the configured tools do. [`agnostic-ai hook run`](#hook-run) runs one when you ask. Review hook specs like code.

## Write one

`agnostic-ai new hook session-status` creates `hooks/session-status.yaml`. Pure YAML, no markdown body.

```yaml
name: session-status
description: Show repository status when a session starts.
targets: [claude, codex]
event: SessionStart
command: "git status --short"
```

Format Go files after the agent edits them, on Claude Code and Codex alike. [`agnostic-ai hook paths`](#edited-paths) reads the edited files from the event JSON on stdin, whichever tool sent it:

```yaml
name: gofmt-on-edit
description: Format the Go files the agent edits.
targets: [claude, codex]
event: PostToolUse
matcher: Edit|Write
command: 'files=$(agnostic-ai hook paths) || exit 1; printf "%s\n" "$files" | grep "\.go$" | while IFS= read -r f; do gofmt -w "$f"; done'
```

Codex takes `Edit` and `Write` as aliases for `apply_patch`, so the one matcher fires on both tools. `agnostic-ai` must be on the hook's `PATH`. The `|| exit 1` makes the hook fail when `hook paths` fails, such as on a missing target, bad JSON, or a missing binary; a plain pipe into the loop would exit 0.

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

Command hooks receive event JSON on stdin; read the shell command from the target's `tool_input` fields, and the edited paths with [`agnostic-ai hook paths`](#edited-paths). `AGNOSTIC_AI_TARGET` names the target that ran the hook; see [which target ran a hook](#hook-target).

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

## Shared hook scripts

Keep one script under `.agnostic-ai/scripts/` and reference that path in `command`:

```yaml
name: guard
targets: [claude, codex]
event: PreToolUse
command: .agnostic-ai/scripts/guard.sh
```

`sync` copies the script into each target's script directory and rewrites the command to run that copy. Script bytes and permissions stay intact; make the source executable when the command runs it directly. A missing referenced script fails sync. Subdirectories under `scripts/` stay intact too. A file at `.agnostic-ai/scripts/<target>/guard.sh` overrides the shared body for that target.

Most targets use `.<target>/hooks/`. Goose uses its plugin's `hooks/` directory, following `outputs.goose.hooks-file`. Windsurf uses `.devin/hooks/`, Antigravity uses `.agents/hooks/`, and Zed uses `.zed/hooks/` when `outputs.zed.tasks-file` is set. Cline uses `.cline/hooks/scripts/` and Copilot uses `.github/hooks/scripts/` to keep helper scripts separate from native hook definitions. Kiro uses `.kiro/scripts/`, outside its hook-definition directory.

For a shell command that starts an interpreter, quote the path: `command: 'node ".agnostic-ai/scripts/guard.mjs"'`. The reference can also follow a [project-root variable](#imported-project-root-paths). Existing `.claude/hooks/`, `.codex/hooks/`, and `.gemini/hooks/` references keep their current behavior.

`sync --global` reads bodies from the global source root's `scripts/` directory and copies them into the selected tools' user hook directories. Its commands use absolute paths to those user copies. Choose a hook event and reply format each target supports; script sharing does not translate those protocols.

## Imported project-root paths

A shell-form command imported from Claude Code can use `$CLAUDE_PROJECT_DIR` or `${CLAUDE_PROJECT_DIR}` to name a script:

```yaml
name: guard
event: PreToolUse
command: 'sh "$CLAUDE_PROJECT_DIR/.claude/hooks/guard.sh"'
```

`sync` keeps the variable on Claude Code, Cursor, and Trae, which provide it. Gemini, Qoder, and Factory use their native project-root variable. Other targets use `$(git rev-parse --show-toplevel)` in a POSIX shell. If the configuration lives in a subdirectory of a Git worktree, the emitted path includes that subdirectory, so a hook started from a descendant still resolves to the configured project. The existing sibling hook-directory rewrite applies too, such as `.claude/hooks/` to `.codex/hooks/`. Custom script paths keep their directory.

`sync --global` uses the runtime Git root for those targets; it never binds a command to the global specs checkout. This fallback requires Git and a hook running inside the intended Git worktree. Global fallback hooks cannot locate a project outside Git or distinguish multiple configured projects within one worktree.

Exec-form `args` stay literal. Escaped dollars and single-quoted variables stay literal too. Parameter operators such as `${CLAUDE_PROJECT_DIR:-fallback}` and `${#CLAUDE_PROJECT_DIR}`, nested substitutions, here-documents, non-POSIX root syntax, and a project without a Git worktree get a note naming the hook when the target cannot preserve them. `on-unsupported: error` fails the sync; `silent` hides the note. Use a target-specific command or `target: claude` for these cases. For Windows, set the target's Windows command explicitly.

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

Trae, Factory, OpenHands, Antigravity, Kiro, Windsurf, and Augment do not get the variable: none has a per-hook `env`, and a prefix would break hooks that work today. Tell them apart by their own variables, such as `TRAE_PROJECT_DIR`, `FACTORY_PROJECT_DIR`, `OPENHANDS_PROJECT_DIR`, `DEVIN_PROJECT_DIR`, or `AUGMENT_PROJECT_DIR`. `CLAUDE_PROJECT_DIR` does not identify Claude Code: Cursor and Trae provide it too.

`sync --global` leaves a matching hand-written hook alone, so an adopted Codex, Gemini, or Qoder entry does not get the variable. Cursor and Copilot also run `.claude/settings.json` hooks but read no `env` from it: Cursor still gets `cursor` from `sessionStart`, Copilot gets nothing.

## Read the edited paths {#edited-paths}

`agnostic-ai hook paths` reads a hook payload on stdin and prints each file the edit leaves on disk, one per line. Paths print relative to the directory the hook runs in; a path outside it prints in full. A tool call that is no edit prints nothing. The target comes from `AGNOSTIC_AI_TARGET`, or from `--target`, which wins. Codex on Windows (`commandWindows`) gets no `AGNOSTIC_AI_TARGET`, so pass `--target codex` there. With no target at all, a payload whose `tool_input` object holds `file_path`, `edits`, or `notebook_path` reads as Claude Code: Cursor and Copilot run `.claude/settings.json` hooks with that payload and no variable. Any other payload with no target fails. Invalid JSON, and an edit tool's `tool_input` that is not an object, fail too (exit 1).

```sh
agnostic-ai hook paths            # src/app.go
agnostic-ai hook paths --action   # update<TAB>src/app.go
agnostic-ai hook paths --json     # [{"action": "update", "path": "src/app.go"}]
```

A plain run skips deleted files and the source of a move, so a formatter sees only files that exist. `--action` and `--json` list every change as `add`, `update`, `delete`, or `move`. A move reads as a `delete` of its source and a `move` of its destination, and `--json` gives the destination a `from`. When a tool does not say whether a write created the file, the change reads as `update`. In a hook that runs before the edit, a new file is not on disk yet, so run formatters after the edit.

| Target | What it reads | Vendor docs |
|---|---|---|
| Claude Code | `tool_input.file_path` of `Edit`, `Write`, and `MultiEdit`; `tool_input.notebook_path` of `NotebookEdit` | [Hooks guide](https://code.claude.com/docs/en/hooks-guide) |
| Codex | The `*** Add File:`, `*** Update File:`, `*** Delete File:`, and `*** Move to:` headers of the `apply_patch` body in `tool_input.command`, every file in the patch | [Hooks](https://learn.chatgpt.com/docs/hooks) |
| Cursor | `file_path` of `afterFileEdit` and `afterTabFileEdit` only; other Cursor events print nothing | [Hooks](https://cursor.com/docs/hooks) |
| Gemini | `tool_input.file_path` of `write_file` and `replace`, a relative path starting at `cwd` | [Hooks reference](https://geminicli.com/docs/hooks/reference/), [file system tools](https://geminicli.com/docs/tools/file-system/) |
| Factory | `tool_input.file_path` of `Create` and `Edit` | [Hooks](https://docs.factory.com/harness/hooks) |
| Augment | `file_changes[].path` with its `changeType`; before the edit, `tool_input.path` of `str-replace-editor` and `save-file` | [Hooks](https://docs.augmentcode.com/cli/hooks) |

Factory and Augment do not get `AGNOSTIC_AI_TARGET`, so give their hooks their own spec with `--target`. Factory documents no input for `ApplyPatch`, so a Factory patch prints nothing.

The command fails for a target it does not read:

- Windsurf: sync writes Devin CLI hooks, and the [Devin CLI docs](https://docs.devin.ai/cli/extensibility/hooks) name the `edit`, `write`, and `apply_patch` tools but not their `tool_input` fields.
- Qoder and Trae: the docs list no `tool_input` fields for the edit tools.
- Copilot: the docs list no `toolArgs` keys for `edit`, `create`, or `apply_patch`.
- Goose, Antigravity: the docs name the edit tools' arguments but show no edit hook payload.
- OpenHands: the docs name no file edit tool.
- Kiro: the docs list no `fs_write` input. A `PostFileSave` command can use its `filePath` template variable.
- Cline: the current docs do not describe the script payload.
- Crush: only `PreToolUse` runs, and it sets `CRUSH_TOOL_INPUT_FILE_PATH`.
- OpenCode and Kilo: plugins get tool arguments as JavaScript objects, not a payload on stdin.
- Zed: no hook fires on an edit.

## Test a hook {#hook-run}

`agnostic-ai hook run <hook>` runs a hook spec the way each target would, before a session fires it. A hook that blocks on Claude Code and does nothing on Codex shows up here instead of in a live session:

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

For each configured target the hook reaches, it runs every command sync wrote for that target, from the project root:

- **Payload.** `--edit <path>` and `--bash <command>` build a `PreToolUse` or `PostToolUse` tool call (`BeforeTool` or `AfterTool` on Gemini), `--prompt <text>` builds `UserPromptSubmit` (`BeforeAgent` on Gemini), and `SessionStart` takes its `source` from the matcher (`startup` when the matcher is empty). `--payload <file>` sends a JSON file as is, for any event.
- **Env.** Claude Code gets `AGNOSTIC_AI_TARGET=claude` and `CLAUDE_PROJECT_DIR`. Codex gets neither: its command sets the target itself. Gemini gets `GEMINI_PROJECT_DIR`, `GEMINI_CWD`, `GEMINI_SESSION_ID`, and `CLAUDE_PROJECT_DIR`, then the handler's `env`, which holds `AGNOSTIC_AI_TARGET=gemini`. These variables are removed from the calling shell's env first; other variables pass through.
- **Shell.** Claude Code commands run with `bash -c`, exec-form `args` with no shell, and `shell: powershell` with PowerShell. Codex commands run with `sh -c`; on Windows, `commandWindows` runs with `powershell.exe -Command`. Gemini commands run with `bash -c`, or Windows PowerShell on Windows, after Gemini's own replacement of `$GEMINI_PROJECT_DIR`, `$GEMINI_CWD`, `$GEMINI_SESSION_ID`, and `$CLAUDE_PROJECT_DIR` with the quoted root.
- **Timeout.** The timeout sync writes (Gemini's in milliseconds), or each tool's default: 600 seconds on Claude Code and Codex, 60 on Gemini.
- **Matcher.** A matcher that does not match the tool or source means the target would not run the hook; that target reports `allow` and why. A target without the hook's event, such as Codex for `Notification`, is listed as not run.
- **Async.** An `async: true` hook runs and prints its output, but its result is `not judged` and stays out of `--expect` and the comparison: neither tool waits for it.
- **Background commands.** On macOS and Linux, a command the hook leaves running is killed once the hook exits.

Each command reports one decision. Exit 2 is `block`, except on `SessionStart`, `SessionEnd`, `Notification`, `PreCompact`, and `PostCompact`, where it cannot stop anything and reads as `error`. On `PostToolUse` the tool already ran, so `block` sends stderr back to the model. Another non-zero exit is `error`. Exit 0 is `allow`, or `block` when stdout is a JSON reply with `"permissionDecision": "deny"`, `"decision": "block"`, or `"continue": false`. A command past its timeout is `timeout`. A `context` line marks output the target adds to the session: plain stdout on `SessionStart` and `UserPromptSubmit`, or a JSON reply's `additionalContext`.

Gemini reads its reply from stdout, or stderr when stdout is empty. A JSON reply with `"decision": "deny"` or `"block"`, or `"continue": false`, is `block`. Plain text with an exit other than 0 and 1 is `block`; with no text at all, Gemini decides nothing and the run reads `error`. Exit 1 is `error`. `SessionStart`, `SessionEnd`, `Notification`, and `PreCompress` ignore decisions, so a block there reads as `error`. Only a JSON `additionalContext` reaches the model; plain stdout is a message for the user.

The run exits 1 when a command times out or errors, when two targets decide differently, or, with `--expect allow` or `--expect block`, when a target decides otherwise. That makes it a CI check.

| Target | Payload | Vendor docs |
|---|---|---|
| Claude Code | Documented shape. `--edit` calls the first of `Write`, `Edit`, and `MultiEdit` the matcher matches, with an absolute `tool_input.file_path`. | [Hooks reference](https://code.claude.com/docs/en/hooks) |
| Codex | Documented shape. `--edit` calls `apply_patch` with an `*** Add File:` or `*** Update File:` patch in `tool_input.command`, and `Edit` and `Write` match it. | [Hooks](https://learn.chatgpt.com/docs/hooks) |
| Gemini | Shape from the source. `--edit` calls the first of `write_file` and `replace` the matcher matches, with an absolute `tool_input.file_path`; `--bash` calls `run_shell_command`. The tool matcher is an unanchored regular expression, and a `SessionStart` source must match exactly. | [Hooks reference](https://geminicli.com/docs/hooks/reference/), gemini-cli [`c6bccb7`](https://github.com/google-gemini/gemini-cli/tree/c6bccb7ecbf6d8368d995455dd725ed34466faad/packages/core/src/hooks) |

On each, `tool_response` holds placeholder values, and session IDs and transcript paths are made up. Claude Code's `if` field is not evaluated. Gemini matchers compile as Go regular expressions, which reject a few JavaScript forms such as lookahead; Gemini CLI would run those, and `hook run` compares them as a literal name. `GEMINI_PLANS_DIR` is not set.

Every other target is listed as not run. Each needs its own payload builder and decision reader, from a documented contract. The [Cursor hooks docs](https://cursor.com/docs/hooks) do not name the shell a command runs in, its default timeout, or how Windows runs it, so Cursor waits for a contract `hook run` can follow. Other targets document no edit payload (see [edited paths](#edited-paths)).
