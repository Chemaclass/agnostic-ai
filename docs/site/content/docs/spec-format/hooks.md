+++
title = "Hooks"
description = "hooks/: commands the tool runs at events such as before a shell command or when a session starts."
weight = 50

[extra]
group = "Reference"
moved = { per-target-body-fences = "@/docs/spec-format/_index.md" }
+++

# Hooks

`hooks/` holds commands the AI tool runs at fixed points: when a session starts, before a tool call, after a file edit, when the agent stops. An instruction asks the model to do something; a hook makes it happen every time.

- **Safety checks.** Block a force push or a write to a generated file before it happens.
- **Automatic follow-up.** Format or lint a file right after the agent edits it.
- **Fresh context.** Print the branch, open issues, or service status when a session starts.
- **One script, many tools.** Tools that share event names, such as Claude Code and Codex, run one spec. `AGNOSTIC_AI_TARGET` tells a shared script which tool called it.

Sync never runs a hook. The configured tools do. [`agnostic-ai hook run`](#hook-run) runs one when you ask. Review hook specs like code.

## Write one

### Portable events {#portable-events}

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
| Claude Code | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `Bash`, `edit` `Edit\|MultiEdit\|Write\|NotebookEdit`, `read` `Read`, `web` `WebFetch\|WebSearch`, `mcp:<server>` `mcp__<server>__.*` |
| Codex | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `Bash`, `edit` `Edit\|Write`, `mcp:<server>` `mcp__<server>__.*` |
| Gemini | `session-start`, `prompt-submit`, `before-tool`, `after-tool`, `after-edit`, `stop`, `session-end` | `shell` `^run_shell_command$`, `edit` `^(write_file\|replace)$`, `read` `^(read_file\|read_many_files)$`, `web` `^(web_fetch\|google_web_search)$` |
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

Cursor, Copilot, and Cline signal a block in their own way. For them, sync runs each portable `before-tool` command (on Cursor, each `prompt-submit` command too) through a wrapper that turns exit 2 into the tool's deny reply, with stderr as the reason:

- **Cursor** gets `"permission": "deny"`, with stderr as `user_message` and `agent_message`. At exit 0 the wrapper replies `"permission": "allow"`, or passes on the JSON reply the command prints. On `prompt-submit` it replies `"continue": false` with stderr as `user_message`, or `"continue": true`. Exit 1 goes on, as in Claude Code. The wrapper is `.cursor/hooks/agnostic-ai-portable-hook.sh`.
- **Copilot** gets exit 2 with `"permissionDecision": "deny"` and stderr as `permissionDecisionReason`. Exit 1 and any other failure deny the call too, where Claude Code reports an error and goes on. A guard with a typo or a missing dependency keeps blocking. The wrapper is `.github/hooks/scripts/agnostic-ai-portable-hook.sh`.
- **Cline** gets `{"cancel": true}` with stderr as `errorMessage`, written into its event script. A cancel also stops the run. Cline ignores exit 1, so the call goes on with no report. Cline has no matcher, so the script runs the command only when the event data names one of the kind's tools.

The wrappers are bash scripts. On Windows, where a tool runs hook commands through PowerShell, it cannot start them: Cursor goes on with an error, and Copilot denies the call. A hook written with `event` syncs as written, with no wrapper. `sync --global` writes the wrapper beside the user hooks file, such as `~/.cursor/hooks/agnostic-ai-portable-hook.sh`.

Cursor gets no `after-tool`, `after-edit`, or `stop`, because its `postToolUse`, `afterFileEdit`, and `stop` cannot block: exit 2 there is a failure Cursor moves past. `session-start` and `session-end` map without a wrapper. Cursor does not wait for either one, and on `session-start` it does not add plain stdout to the session as Claude Code does.

#### Decision on stdout {#decision-on-stdout}

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

Only the top level counts, so a nested `decision` never overrides the verdict. Raw NUL and SOH bytes block. Any exit code other than 0 keeps its usual meaning.

No tool reads this object itself. On every tool, sync runs the command through a bash wrapper that turns the decision into that tool's block, each `x-gemini.hooks` command handler included. So `decision: stdout` cannot go with `commandWindows` or `shell: powershell`. Augment runs only a bare script path, so it cannot run the wrapper, and `validate` names a `decision: stdout` hook that reaches it. Stdout over 1,000,000 bytes, more than 10,000 values, or more than 64 nested containers blocks. A `\u` escape outside ASCII reads as `?`.

#### Hooks Cursor and Copilot also read {#claude-settings-copies}

Cursor and Copilot also run `.claude/settings.json` hooks. When `cursor` is a target, sync puts `[ "$AGNOSTIC_AI_TARGET" = cursor ] && exit 0;` before the Claude Code copy of each portable hook that also reaches Cursor. Cursor's `sessionStart` hook sets that variable, so the hook runs once there, as Cursor's own copy. Claude Code sets it to `claude`, and an unset variable runs the hook.

A hook still runs twice on Cursor when:

- it fires before Cursor's `sessionStart` hook returns
- it has `args` or `shell: powershell`, which has no POSIX shell to run the check
- it is a user hook from `sync --global`, which adds no check

A hook synced to `claude` and `copilot` runs twice on Copilot, which sets no variable. Both copies block on exit 2.

A spec sets `on` or `event`, never both. `match` goes with `on`, and `matcher` with `event`. `validate` and `lint` (LINT032) report:

- an unknown value
- both forms mixed in one spec
- an event that a tool the hook reaches reads differently, such as `on: stop` on Crush
- a tool kind that a tool the hook reaches lacks, such as `match: read` on Codex

A portable hook does not reach Kiro or Trae yet. Sync prints a note with the count, and [`hook run`](#hook-run) lists them as not run. Write `event` for those tools, or limit the hook with `targets`.

`agnostic-ai migrate --only hooks` rewrites `event` and `matcher` as `on` and `match` when the portable form gives every tool the hook reaches the same event and matcher. It leaves every other hook as written and says why. A Claude Code hook on `Edit|Write` stays native, since `match: edit` there also runs on `MultiEdit` and `NotebookEdit`. `lint` warns on each hook the migration would rewrite (LINT034). `agnostic-ai import` follows the same rule.

### Native events

`agnostic-ai new hook session-status` creates `hooks/session-status.yaml`. It is plain YAML with no markdown body.

```yaml
name: session-status
description: Show repository status when a session starts.
targets: [claude, codex]
event: SessionStart
command: "git status --short"
```

Format Go files after the agent edits them, on Claude Code and Codex alike. [`agnostic-ai hook paths`](#edited-paths) reads the edited files from the event JSON on stdin, whichever tool sent it.

```yaml
name: gofmt-on-edit
description: Format the Go files the agent edits.
targets: [claude, codex]
event: PostToolUse
matcher: Edit|Write
command: 'files=$(agnostic-ai hook paths) || exit 1; printf "%s\n" "$files" | grep "\.go$" | while IFS= read -r f; do gofmt -w "$f"; done'
```

Codex takes `Edit` and `Write` as aliases for `apply_patch`, so the one matcher fires on both tools. `agnostic-ai` must be on the hook's `PATH`. The `|| exit 1` fails the hook when `hook paths` fails, for example on bad JSON or a missing binary. Without it, the pipe into the loop would exit 0.

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

Command hooks receive event JSON on stdin. Read the shell command from `tool_input` and the edited paths with [`agnostic-ai hook paths`](#edited-paths). `AGNOSTIC_AI_TARGET` names the tool that ran the hook; see [which target ran a hook](#hook-target).

Sync keeps hook entries you wrote. In `.claude/settings.json`, `.codex/hooks.json`, `.cursor/hooks.json`, `.gemini/settings.json`, `.qoder/settings.json`, and `.factory/hooks.json`, it puts its own entries after yours in each event and changes or removes only those. An entry you wrote by hand that matches a spec's output, with the same matcher (for example after `import`), counts as the spec's own, so it is not written twice.

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Hook identifier. |
| `description` | no | empty | Free-form documentation. |
| `on` | `on` or `event` | none | Portable event, written as each tool's native event. See [portable events](#portable-events). |
| `match` | no | every tool | Tool kind for `on: before-tool` or `after-tool`. See [portable events](#portable-events). |
| `decision` | no | none | `stdout` reads a `{"decision": ..., "reason": ...}` object at exit 0 on `on: before-tool`. See [decision on stdout](#decision-on-stdout). |
| `event` | `on` or `event` | none | Hook event, written as is. See [events](#events). |
| `matcher` | no | empty | Regex on the tool name, or another selector the event uses. |
| `command` | command handlers only | none | Shell command, or a list where each entry becomes its own handler. |
| `args` | no | empty | Switches to **exec form**: `command` runs as a program with `args` as its arguments and no shell, so spaces, `$`, and backticks pass through unchanged. Leave unset when the command needs a pipe or `&&`. Claude Code, Qoder, and Copilot keep `args` apart. The other tools run `command` in a shell, with the args added, each quoted for a POSIX shell. On Windows, Trae's PowerShell and OpenHands' cmd.exe can read those quotes differently, and sync notes it. Augment skips a hook with `args`, with a note. |
| `type` | no | `command` | `command`, `http`, `mcp_tool`, or `prompt`, where the tool supports it. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Some tools convert to milliseconds or apply their own default. |
| `disabled` | no | `false` | Keep the hook defined but stop it running. Antigravity and Kiro write `enabled: false`; OpenCode and Kilo write no plugin module; other tools write the hook unchanged. |

Handler-specific fields are written only where the tool's schema defines them:

- `server`, `tool`, `input` (`type: mcp_tool`): Claude Code, Codex.
- `url`, `headers`, `allowedEnvVars` (HTTP handler): Claude Code, Qoder, Copilot.
- `prompt`, `model` (prompt handler): Claude Code, Qoder, Cursor, Copilot (`sessionStart` only).
- `statusMessage`, `async`: Claude Code, Codex, Qoder.
- `asyncRewake`, `shell`, `if`: Claude Code, Qoder.
- `continueOnBlock`: Claude Code. `commandWindows`: Codex, Copilot. `additionalContextLimit`: Codex. `failClosed`: Claude Code (command and HTTP handlers; no effect on `Stop`, `SubagentStop`, `TaskCompleted`, `TeammateIdle`, or `async` and `asyncRewake` command hooks, and it denies on `PermissionRequest`), Cursor. Both block when the hook cannot start, times out, or exits with a code other than 0 or 2; Cursor also blocks on exit 0 with no output, which Claude Code allows. `loop_limit`: Cursor, Trae.
- `x-goose.on_failure` (Goose), `x-kiro.action` (Kiro), `x-gemini.hooks`, `x-gemini.sequential`, `x-gemini.name`, `x-gemini.env` (Gemini).

`command` is not needed for a non-command handler, a valid `x-kiro.action`, or a hook that sets `x-gemini.hooks`. Limit a non-command hook to the tools that support it with `target` or `targets`.

## Events

`event` is written as is, never translated between tools. For one spec across tools, use [portable events](#portable-events).

- Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`, so one spec works on both.
- Other tools need their own names, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`.
- `agnostic-ai validate` flags an event a tool does not recognize.
- `sync` and `sync --check` stop before writing when an event looks like a typo of a known one, such as `PreToolUze`, and name the closest.
- Tools without hook support log a warning and skip the hook.

## Shared hook scripts

Keep one script under `.agnostic-ai/scripts/` and use that path in `command`. `agnostic-ai init --demo` seeds this guard, which blocks `git push --force` and allows `--force-with-lease` on Claude Code and Codex:

```yaml
name: no-force-push
description: Block git push --force and allow --force-with-lease.
targets: [claude, codex]
event: PreToolUse
matcher: Bash
command: '"$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/no-force-push.sh"'
commandWindows: '$s = "$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/no-force-push.sh"; if (-not (Get-Command sh -ErrorAction SilentlyContinue)) { exit 1 }; sh $s; exit $LASTEXITCODE'
timeout: 10
```

The script reads `tool_input.command` from the event JSON on stdin, with no `jq`, and splits it into words the way `sh` does. A `git push --force` inside quotes, a comment, or a heredoc body passes, and so does a quoted command inside `bash -c` or `eval`. A push with a `+main` refspec or `--mirror` counts as forced unless it uses `--force-with-lease`. `--force` or `-f` blocks even next to `--force-with-lease`. A force push written outside quotes blocks even behind a wrapper the script does not parse, such as `xargs`. On a force push it prints the reason on stderr and exits 2.

Both tools start a hook in the session directory, which can be below the project root, so the path starts at the root. Claude Code keeps [`$CLAUDE_PROJECT_DIR`](#imported-project-root-paths). Codex gets `$(git rev-parse --show-toplevel)` in both commands, plus the project's path below the Git root.

For Copilot, a hook with one plain command writes `commandWindows` as the `powershell` field and `command` as `bash`. A portable `on:` hook, `args`, or a command list keeps one `command`, with a coverage note. Cursor and Gemini CLI ignore `commandWindows`.

On Windows, Codex runs `commandWindows` with `powershell.exe -Command`. The command above handles three PowerShell quirks:

- PowerShell ignores the script's `#!/bin/sh` line, so the command names `sh`. That needs `sh` on `PATH`, as Git for Windows provides.
- PowerShell reports a failed native command as exit 1, which Codex lets through. `exit $LASTEXITCODE` passes on the script's exit 2.
- The Git root lookup resets `$LASTEXITCODE`, so the command checks for `sh` with `Get-Command` and exits 1 when it is missing.

Claude Code runs hooks with Git Bash on Windows and needs no `commandWindows`. Without Git Bash it uses PowerShell, where this guard cannot run.

{% <details summary=".agnostic-ai/scripts/no-force-push.sh"> %}
```sh
#!/bin/sh
# Blocks git push --force, -f, --mirror, or a +refspec, and lets
# --force-with-lease through. It catches a mistake. It is not a sandbox.

# Reads tool_input.command from the hook JSON on stdin and splits it into
# words the way sh would: quotes, backslashes, line continuations,
# comments, redirections, heredoc bodies, and $( ) or backtick
# substitutions. It checks each command between unquoted ; & | ( ) and
# newlines, past reserved words such as if and then. A heredoc it cannot
# read ends the check, since a missed push beats blocking text. A coarse
# pass over the unquoted text then blocks a force push the parse missed.
awk -v q='"' -v sq="'" '
function flush() {
  if (inword && redirect) redirect = 0
  else if (inword) {
    words[++n] = w
    quoted[n] = wq
  }
  w = ""
  inword = 0
  wq = 0
}

# Drops a file descriptor number written right before a redirection.
function redirect_from() {
  if (inword && !wq && w ~ /^[0-9]+$/) {
    w = ""
    inword = 0
  }
  flush()
  redirect = 1
}

# Returns the index of the word that names the program, past reserved
# words, assignments, and wrappers such as env and sudo.
function program(   i, a, wrapper) {
  for (i = 1; i <= n; i++) {
    a = words[i]
    if (!quoted[i] && a ~ /^(if|then|else|elif|do|while|until|!|time|\{)$/) continue
    if (a ~ /^[A-Za-z_][A-Za-z0-9_]*=/) continue
    if (a ~ /^(command|exec|env|nohup|nice|sudo|xargs)$/) { wrapper = a; continue }
    if (wrapper != "" && a ~ /^-/) {
      if ((wrapper == "env" && a ~ /^(-[uCS]|--(unset|chdir|split-string))$/) || (wrapper == "exec" && a == "-a") || (wrapper == "nice" && a ~ /^(-n|--adjustment)$/) || (wrapper == "xargs" && a ~ /^-[InLPsEda]$/) || (wrapper == "sudo" && a ~ /^(-[ugCDpUrtTR]|--(user|group|close-from|chdir|prompt|other-user|role|type|command-timeout|host))$/)) i++
      continue
    }
    if (i > 1 && words[i - 1] == "time" && a == "-p") continue
    return i
  }
  return n + 1
}

function check(   i, j, k, a, plus, lease, positional) {
  i = program()
  if (i > n || words[i] !~ /(^|\/)git(\.exe)?$/) return
  for (i++; i <= n && words[i] ~ /^-/; i++)
    if (words[i] ~ /^(-C|-c|--git-dir|--work-tree|--namespace)$/) i++
  if (i > n || words[i] != "push") return
  for (j = i + 1; j <= n; j++) {
    a = words[j]
    if (positional || a !~ /^-./) {
      if (a ~ /^\+/) plus = 1
      continue
    }
    if (a == "--") { positional = 1; continue }
    if (a == "--force") { blocked = 1; return }
    if (a ~ /^--force-with-lease(=|$)/) { lease = 1; continue }
    if (a == "--mirror") { plus = 1; continue }
    if (a ~ /^--(repo|receive-pack|exec|push-option)$/) { j++; continue }
    if (a ~ /^--/) continue
    for (k = 2; k <= length(a); k++) {
      if (substr(a, k, 1) == "f") { blocked = 1; return }
      if (substr(a, k, 1) == "o") { if (k == length(a)) j++; break }
    }
  }
  if (plus && !lease) blocked = 1
}

function end_command() {
  flush()
  check()
  n = 0
  redirect = 0
}

# Starts a subshell or substitution. A substitution keeps the words of the
# command around it, which continue once it closes.
function open_nested(kind, back,   i) {
  depth++
  kinds[depth] = kind
  backs[depth] = back
  if (kind == "(") { end_command(); return }
  saved_n[depth] = n
  for (i = 1; i <= n; i++) {
    saved_words[depth, i] = words[i]
    saved_quoted[depth, i] = quoted[i]
  }
  saved_w[depth] = w
  saved_wq[depth] = wq
  saved_redirect[depth] = redirect
  n = 0
  w = ""
  inword = 0
  wq = 0
  redirect = 0
}

function close_nested(   i) {
  end_command()
  mode = backs[depth]
  if (kinds[depth] != "(") {
    n = saved_n[depth]
    for (i = 1; i <= n; i++) {
      words[i] = saved_words[depth, i]
      quoted[i] = saved_quoted[depth, i]
    }
    w = saved_w[depth] "$()"
    inword = 1
    wq = saved_wq[depth]
    redirect = saved_redirect[depth]
  }
  depth--
}

# Queues the delimiter of the heredoc whose word starts at p and returns
# the position of its last character, or 0 for a form it cannot read.
function heredoc_word(p,   strip, word, c, closing) {
  strip = 0
  if (substr(cmd, p, 1) == "-") { strip = 1; p++ }
  while (substr(cmd, p, 1) == " " || substr(cmd, p, 1) == "\t") p++
  word = ""
  for (; p <= size; p++) {
    c = substr(cmd, p, 1)
    if (c == sq || c == q) {
      closing = index(substr(cmd, p + 1), c)
      if (!closing) return 0
      word = word substr(cmd, p + 1, closing - 1)
      p += closing
      continue
    }
    if (c == "\\") { p++; word = word substr(cmd, p, 1); continue }
    if (index(" \t\n;&|()<>", c)) break
    word = word c
  }
  if (word == "") return 0
  heredocs[++pending] = word
  strips[pending] = strip
  return p - 1
}

# Skips the queued heredoc bodies that start after the newline at p and
# returns the position of the newline that ends the last terminator.
function skip_bodies(p,   h, rest, end, line) {
  for (h = 1; h <= pending; h++) {
    do {
      if (p >= size) { pending = 0; return size }
      rest = substr(cmd, p + 1)
      end = index(rest, "\n")
      line = end ? substr(rest, 1, end - 1) : rest
      p = end ? p + end : size
      if (strips[h]) sub(/^\t+/, "", line)
    } while (line != heredocs[h])
  }
  pending = 0
  return p
}

# Backs up the parse with a coarse look at the unquoted text, so a wrapper
# or syntax the parse does not follow still blocks: within a span between
# separators, git, then push as its subcommand, then --force, -f, or a
# --mirror or +refspec without --force-with-lease.
function coarse(   spans, count, s, nw, ws, i, j, a, plus, lease) {
  count = split(raw, spans, /[;&|\n]/)
  for (s = 1; s <= count; s++) {
    nw = split(spans[s], ws, /[ \t\r]+/)
    for (i = 1; i <= nw; i++) {
      if (ws[i] !~ /(^|\/)git(\.exe)?$/) continue
      for (j = i + 1; j <= nw && ws[j] ~ /^-/; j++)
        if (ws[j] ~ /^(-C|-c|--git-dir|--work-tree|--namespace)$/) j++
      if (ws[j] != "push") continue
      plus = 0
      lease = 0
      for (j++; j <= nw; j++) {
        a = ws[j]
        if (a == "--force" || a ~ /^-[A-Za-z]*f[A-Za-z]*$/) return 1
        if (a ~ /^--force-with-lease(=|$)/) lease = 1
        else if (a ~ /^\+/ || a == "--mirror") plus = 1
      }
      if (plus && !lease) return 1
    }
  }
  return 0
}

{ json = json $0 "\n" }

END {
  i = index(json, q "command" q)
  if (i == 0) exit 0
  s = substr(json, i + 9)
  if (!match(s, /^[ \t\r\n]*:[ \t\r\n]*/)) exit 0
  s = substr(s, RLENGTH + 1)
  if (substr(s, 1, 1) != q) exit 0
  s = substr(s, 2)

  cmd = ""
  while (length(s) > 0) {
    c = substr(s, 1, 1)
    if (c == q) break
    if (c != "\\") { cmd = cmd c; s = substr(s, 2); continue }
    e = substr(s, 2, 1)
    if (e == "u") {
      code = tolower(substr(s, 3, 4))
      if (code == "0026") cmd = cmd "&"
      else if (code == "003c") cmd = cmd "<"
      else if (code == "003e") cmd = cmd ">"
      else if (code == "0027") cmd = cmd sq
      else cmd = cmd " "
      s = substr(s, 7)
      continue
    }
    if (e == "n") cmd = cmd "\n"
    else if (e == "t") cmd = cmd "\t"
    else if (e == "r" || e == "b" || e == "f") cmd = cmd " "
    else cmd = cmd e
    s = substr(s, 3)
  }

  n = 0
  w = ""
  inword = 0
  mode = ""
  raw = ""
  size = length(cmd)
  for (p = 1; p <= size && !blocked; p++) {
    c = substr(cmd, p, 1)
    e = substr(cmd, p + 1, 1)
    if (mode == "single") {
      if (c == sq) mode = ""
      else w = w c
      continue
    }
    if (c == "\\") {
      p++
      if (e == "\n") continue
      if (mode == "double" && e != q && e != "\\" && e != "$" && e != "`") w = w c
      w = w e
      wq = 1
      inword = 1
      if (mode == "") raw = raw " "
      continue
    }
    if (mode == "double") {
      if (c == q) mode = ""
      else if (c == "$" && e == "(") { p++; open_nested("$(", "double"); mode = "" }
      else if (c == "`") { open_nested("`", "double"); mode = "" }
      else w = w c
      continue
    }
    raw = raw " "
    if (c == q) { mode = "double"; inword = 1; wq = 1; continue }
    if (c == sq) { mode = "single"; inword = 1; wq = 1; continue }
    if (c == "#" && !inword) {
      while (p < size && substr(cmd, p + 1, 1) != "\n") p++
      continue
    }
    if (c == "$" && e == "(" && substr(cmd, p + 2, 1) != "(") { p++; open_nested("$(", ""); continue }
    if ((c == "<" || c == ">") && e == "(") { p++; open_nested("$(", ""); continue }
    if (c == "`") {
      if (depth && kinds[depth] == "`") close_nested()
      else open_nested("`", "")
      continue
    }
    if (c == "(") { open_nested("(", ""); continue }
    if (c == ")") {
      if (depth) close_nested()
      else end_command()
      continue
    }
    if (c == "<" && substr(cmd, p, 3) == "<<<") { redirect_from(); p += 2; continue }
    if (c == "<" && e == "<") {
      redirect_from()
      redirect = 0
      p = heredoc_word(p + 2)
      if (!p) { stopped = 1; break }
      continue
    }
    if (c == "&" && e == ">") {
      flush()
      redirect = 1
      p += (substr(cmd, p + 2, 1) == ">") ? 2 : 1
      continue
    }
    if (c == ">" || c == "<") {
      redirect_from()
      if ((c == ">" && (e == ">" || e == "&" || e == "|")) || (c == "<" && (e == "&" || e == ">"))) p++
      continue
    }
    if (c == " " || c == "\t" || c == "\r") { flush(); continue }
    if (index(";&|\n", c)) {
      raw = raw c
      end_command()
      if (c == "\n" && pending) p = skip_bodies(p)
      continue
    }
    raw = substr(raw, 1, length(raw) - 1) c
    w = w c
    inword = 1
  }
  if (!blocked && !stopped) end_command()
  if (!blocked && coarse()) blocked = 1
  exit (blocked ? 3 : 0)
}'
# awk exits 2 on its own errors, so a block comes back as 3.
case $? in
  0) exit 0 ;;
  3)
    echo "Blocked: git push --force rewrites the remote branch. Use --force-with-lease, or run the push yourself." >&2
    exit 2
    ;;
  *) exit 1 ;;
esac
```
{% </details> %}

Check it before a session does:

```sh
agnostic-ai hook run no-force-push --bash 'git push --force origin main' --expect block
agnostic-ai hook run no-force-push --bash 'git status' --expect allow
```

A guard like this catches a mistake. It is not a sandbox: a command written to get around it, such as a push from a script file or an alias, gets through. To block a command for good, use [`permissions.deny`](@/docs/spec-format/settings.md#permission-rules) and each tool's own sandbox: [Claude Code](https://code.claude.com/docs/en/sandboxing), [Codex](https://learn.chatgpt.com/docs/sandboxing).

`sync` copies the script into each tool's script directory and changes the command to run that copy.

- Script contents, permissions, and subdirectories under `scripts/` stay as they are.
- Make the source executable when the command runs it directly.
- A missing script fails sync.
- A file at `.agnostic-ai/scripts/<target>/guard.sh` replaces the shared script for that tool.

For a shell command that starts an interpreter, quote the path: `command: 'node ".agnostic-ai/scripts/guard.mjs"'`. The path can also follow a [project-root variable](#imported-project-root-paths). Existing `.claude/hooks/`, `.codex/hooks/`, and `.gemini/hooks/` paths keep working.

`sync --global` copies scripts from the global source root's `scripts/` directory into the selected tools' user hook directories, and its commands use absolute paths to those copies. Script sharing does not translate event names or reply formats, so pick ones every tool supports.

{% <details summary="Where each tool keeps its scripts"> %}
Most tools use `.<target>/hooks/`. The exceptions:

| Target | Script directory |
|--------|------------------|
| Goose | its plugin's `hooks/` directory, following `outputs.goose.hooks-file` |
| Windsurf | `.devin/hooks/` |
| Antigravity | `.agents/hooks/` |
| Zed | `.zed/hooks/`, when `outputs.zed.tasks-file` is set |
| Cline | `.cline/hooks/scripts/`, apart from native hook definitions |
| Copilot | `.github/hooks/scripts/`, apart from native hook definitions |
| Kiro | `.kiro/scripts/`, outside its hook-definition directory |
{% </details> %}

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

{% <details summary="Global sync"> %}
`sync --global` uses the Git root at run time for those tools. It needs Git and a hook running inside the intended Git worktree. It cannot find a project outside Git, or tell apart several configured projects in one worktree.
{% </details> %}

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

A spec that sets `AGNOSTIC_AI_TARGET` in its own `env` keeps that value. Where sync cannot set it, the parent process's value stays. Sync sets it for each tool:

- [Claude Code](@/docs/targets/claude.md): `env` in `.claude/settings.json` (`~/.claude/settings.json` for `sync --global`). Set for the whole session, so the Bash tool sees it too.
- [Cursor](@/docs/targets/cursor.md): a `sessionStart` hook returns the variable, so `sessionStart` hooks and hooks that fire before it returns do not see it.
- [Codex](@/docs/targets/codex.md): `export AGNOSTIC_AI_TARGET=codex; ` before `command`. Not set on Windows (`commandWindows`), and needs a POSIX session shell (a `pwsh` or `nu` login shell breaks it).
- [Gemini](@/docs/targets/gemini.md), [Qoder](@/docs/targets/qoder.md), [Copilot](@/docs/targets/copilot.md): `env` on each command handler.
- [Goose](@/docs/targets/goose.md), [Crush](@/docs/targets/crush.md), [Cline](@/docs/targets/cline.md): `export` prefix or line.
- [OpenCode](@/docs/targets/opencode.md), [Kilo](@/docs/targets/kilo.md): `.env()` on each plugin command. [Zed](@/docs/targets/zed.md): `env` on each task.

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

{% <details summary="Tools the command does not read"> %}
Their docs do not show an edit's event data, so the command fails:

- Windsurf, Trae, Copilot, Goose, Antigravity, OpenHands, Cline.
- Kiro: a `PostFileSave` command can use its `filePath` template variable.
- Crush: only `PreToolUse` runs, and it sets `CRUSH_TOOL_INPUT_FILE_PATH`.
- OpenCode and Kilo: plugins get tool arguments as JavaScript objects, not as event data on stdin.
- Zed: no hook fires on an edit.
{% </details> %}

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

- **Event data.** `--edit <path>` and `--bash <command>` build a `PreToolUse` or `PostToolUse` tool call (`BeforeTool` or `AfterTool` on Gemini). `--prompt <text>` builds `UserPromptSubmit` (`BeforeAgent` on Gemini). `SessionStart` takes its `source` from the matcher, or `startup` when the matcher is empty. `--payload <file>` sends a JSON file as is, for any event.
- **Matcher.** If the matcher does not match the tool or source, the tool would not run the hook, so that tool reports `allow` and why. A tool call from `--payload` matches its `tool_name` with the tool's own matcher rules, including Codex's `Edit` and `Write` aliases for `apply_patch`. Cursor, Goose, Copilot, and Kiro match the field their docs name for the event, such as the command, file path, or prompt text.
- **Missing event.** A tool without the hook's event, such as Codex for `Notification`, is listed as not run.
- **Timeout.** The timeout sync writes, or the tool's default.
- **Async.** An `async: true` hook, or an `asyncRewake: true` one on Claude Code and Qoder, runs and prints its output. Its result is `not judged` and stays out of `--expect` and the comparison, because the tool does not wait for it. The same holds for Cursor `sessionStart` and `sessionEnd` hooks, Copilot `notification` hooks, and Cline hooks on any event but `PreToolUse` and `PostToolUse`.
- **Background commands.** On macOS and Linux, a command the hook leaves running is killed once the hook exits.
- **Stale tool file.** Commands come from the spec, so the run works before a sync. The tool prints a `warning` naming the file when the synced file is missing, when no handler under the event runs the command the spec produces, or when that handler's group matcher, timeout, or (on Gemini) `env` differs from the spec. On Claude Code and Cursor it also compares `failClosed`, on Copilot `cwd`, and on Kiro `confirm`. An `env` warning names the keys, never their values. A warning does not fail the run; `sync` clears it. The file is the one each [tool page](@/docs/targets/_index.md) lists, or the path `outputs` sets.

{% <details summary="Environment variables per tool"> %}
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
{% </details> %}

{% <details summary="Shell and default timeout per tool"> %}
Where a cell says "script path only", `hook run` assumes `sh -c` for a script path with plain or single-quoted arguments. A command with shell syntax, and any command on Windows, is listed as not run. See [assumed results](#assumed-results).

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
{% </details> %}

{% <details summary="Claude Code `if` rules"> %}
A handler's `if` permission rule decides whether it runs, as in Claude Code. It applies only on `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, and `PermissionDenied`. See the [`if` field](https://code.claude.com/docs/en/hooks) and [permission rule syntax](https://code.claude.com/docs/en/permissions#permission-rule-syntax).

- `Bash(git *)` matches any part of a command split by `&&`, `||`, `;`, `|`, and `&`, and commands inside `$()` and backticks, after leading `VAR=value` assignments and wrappers such as `timeout 30` are dropped. A trailing ` *` also matches the bare command, and `:*` is the same as ` *`.
- A pattern that names more than the command runs on any `$()`, backtick, or `$VAR`. A command that does not parse always runs.
- `Edit(path)` covers `Edit`, `Write`, `MultiEdit`, and `NotebookEdit`, with gitignore patterns. `//path` is from the filesystem root, `~/path` from home, and `/path` and `path` from the project root. A bare name such as `.env` matches at any depth.
- `Tool(param:value)` matches a top-level input field.
{% </details> %}

Each command reports one decision.

- `block`: exit 2, or exit 0 with a JSON reply that has `"permissionDecision": "deny"`, `"decision": "block"`, or `"continue": false`.
- `allow`: any other exit 0.
- `error`: any other non-zero exit.
- `timeout`: the command ran past its timeout.

With `failClosed: true`, Claude Code reads an `error` or `timeout` as `block`, except that it has no effect on `Stop`, `SubagentStop`, `TaskCompleted`, and `TeammateIdle`, or on command hooks that set `async` or `asyncRewake`; sync notes these and still writes `onFailure: "block"`. On `PermissionRequest` a failure denies the request.

Exit 2 cannot stop anything on `SessionStart`, `SessionEnd`, `Notification`, `PreCompact`, and `PostCompact`, so it reads as `error` there. On `PostToolUse` the tool already ran, so `block` sends stderr back to the model. A `context` line marks output the tool adds to the session: plain stdout on `SessionStart` and `UserPromptSubmit`, or a JSON reply's `additionalContext`.

{% <details summary="How other tools read replies"> %}
- **Gemini.** It reads stdout, or stderr when stdout is empty. A JSON reply with `"decision": "deny"` or `"block"`, or `"continue": false`, is `block`, and so is plain text with an exit other than 0 and 1. No text at all, or exit 1, is `error`. `SessionStart`, `SessionEnd`, `Notification`, and `PreCompress` ignore decisions, so a block there reads as `error`. Only a JSON `additionalContext` reaches the model; plain stdout is a message for the user.
- **Trae.** As Claude Code.
- **OpenHands.** It blocks on exit 2, or on a JSON `"decision": "deny"` or `"continue": false` whatever the exit code, and only on `PreToolUse`, `UserPromptSubmit`, and `Stop`. A `context` line marks a JSON `additionalContext` on `UserPromptSubmit` and `Stop` only.
- **Goose.** It blocks on `PreToolUse` and `Stop` only: on exit 2, or on stdout starting with `{` whose `decision` is `"block"`, whatever the exit code. Exit 0 with empty stdout or `"decision": "allow"` allows. Anything else is `error`, or `block` when the action sets `x-goose.on_failure: block`.
- **Cursor.** It blocks on exit 2 on the permission events (`beforeShellExecution`, `beforeMCPExecution`, `beforeReadFile`, `beforeTabFileRead`, `subagentStart`, `preToolUse`) and `beforeSubmitPrompt`. At exit 0, a permission event blocks on `"permission": "deny"`, on `"ask"` (except on `preToolUse`, which does not enforce it), and on output that is not a valid JSON reply. No output at all fails open. `beforeSubmitPrompt` blocks on `"continue": false`. Any other failure fails open as `error`, or blocks with `failClosed: true`.
- **Crush.** Exit 2 blocks the tool call. Exit 49 blocks too and halts the whole turn, which a note says. Any other non-zero exit and a timeout do not block. At exit 0, a JSON reply with `"decision": "deny"` or `"halt": true` blocks. A reply with a `hookSpecificOutput` key is read as Claude Code's instead, and its top-level fields are ignored. A field of the wrong type voids the reply, which then allows. A `context` line marks a reply's `context` or `additionalContext`.
- **Factory.** It blocks on exit 2, except on `Notification`, `SubagentStop`, `PreCompact`, `SessionStart`, and `SessionEnd`, where exit 2 reads as `error`. At exit 0, `PreToolUse` blocks on `permissionDecision` `"deny"` or `"ask"`; `PostToolUse`, `UserPromptSubmit`, `Stop`, and `SubagentStop` block on `"decision": "block"`; and every event but `SessionEnd` blocks on `"continue": false`.
- **Copilot.** It drops `{"type": "progress"}` lines from stdout and parses the rest as one JSON reply; text that does not parse, or two objects, is no reply. `preToolUse` blocks on any non-zero exit, a command that did not start, `"permissionDecision": "deny"`, or `"ask"`. A failure other than exit 2 also counts as `error`, so `--expect block` fails on a hook that never ran. `permissionRequest` blocks on exit 2 or `"behavior": "deny"`; the commands' replies merge in order, so a later `"behavior": "allow"` overrides an earlier deny. `agentStop` and `subagentStop` block on `"decision": "block"`. Other events cannot block: exit 2 and other failures read as `error`, except exit 2 on `postToolUseFailure`, which adds context. A timeout fails open on every event. PascalCase events decide as their camelCase twins.
- **Qoder.** It blocks on exit 2 on `UserPromptSubmit`, `PreToolUse`, `Stop`, `SubagentStop`, `PreCompact`, `ConfigChange`, `Elicitation`, and `ElicitationResult`, and on any non-zero exit on `WorktreeCreate`. Other failures read as `error`, and `StopFailure` and `InstructionsLoaded` ignore the result. At exit 0, a JSON reply blocks on `"continue": false`, on `"decision": "deny"` where exit 2 blocks, on `PreToolUse` on `permissionDecision` `"deny"` or `"ask"` (which wins over `decision`), on `PermissionRequest` on a `decision.behavior` of `"deny"`, and on the elicitation events on an `action` of `"decline"` or `"cancel"`. A `hookSpecificOutput` without `hookEventName` voids the reply and reads as `error`. A `ConfigChange` with `source: policy_settings` cannot block, so a block there reads as `error`. `TaskCreated`, `TaskCompleted`, `TeammateIdle`, and `Setup` have no documented decision rules, so Qoder is listed as not run for them. A handler's `if` glob in parentheses matches the `command` of `Bash` or the `file_path` of a file tool; another tool with a glob is listed as not run.
- **Antigravity.** Only exit 0 with a JSON reply of the documented shape counts. On `PreToolUse`, `"decision": "allow"` allows; `"deny"` blocks; `"force_ask"` blocks (a note says Antigravity asks the user). `"ask"` and `"deny_unless_prior_grant"` read as `block` but are not counted, since saved permissions can let the call run, and `hook run` cannot see them. On `Stop`, `"decision": "continue"` keeps the agent running and reads as `block`; any other value allows. `PostToolUse`, `PreInvocation`, and `PostInvocation` allow on a JSON object. Anything else reads as `error` or `timeout` and is [not counted](#assumed-results).
- **Cline.** It never reads the exit code. It reads stdout as JSON: the last line starting `HOOK_CONTROL` and a tab, else the whole stdout. On `PreToolUse` and `PostToolUse`, an object with `"cancel": true` blocks, and Cline stops the run; on `PostToolUse` the tool already ran. Empty stdout and any other JSON allow, whatever the exit code; a non-zero exit adds a note that Cline ignores it. Stdout that is not JSON reads as `error`, and a timeout as `timeout`, each with a note that Cline goes on as if the hook allowed. A `context` line marks a reply's `context`, `contextModification`, or `errorMessage`. When other specs reach Cline on the same event, sync writes them into one script that shares stdout, so the result is [not counted](#assumed-results), even with `--include-assumed`.
- **Kiro.** It blocks on exit 2 on `PreToolUse` and `UserPromptSubmit`. Another non-zero exit there reads as `error` and is [not counted](#assumed-results), since Kiro's docs disagree on whether it blocks. On `PostToolUse` and `Stop`, any non-zero exit is `error`. At exit 0, `Stop` reads `"decision": "block"` from a JSON reply as `block`, since Kiro keeps the agent running (a note says so); other events read no reply. A `context` line marks stdout on `UserPromptSubmit` and a `Stop` block's `reason`.
- **Windsurf.** Devin CLI blocks on exit 2, or on exit 0 with `"decision": "block"`, on `PreToolUse`, `PermissionRequest`, `UserPromptSubmit`, and `Stop`. `"decision": "approve"`, no decision, and plain stdout allow. Any other non-zero exit is `error`. A block on `PostToolUse`, `PostCompaction`, `SessionStart`, or `SessionEnd`, a reply on a non-zero exit (except exit 2 with `"decision": "block"`), and a `decision` other than `"approve"` or `"block"` are [not counted](#assumed-results). A `PreToolUse` reply with `hookSpecificOutput.updatedInput` gets a note, since Devin CLI merges it into the tool's arguments. A `context` line marks `hookSpecificOutput.additionalContext` on `UserPromptSubmit`, `SessionStart`, and `PostToolUse`. Both count only when `hookSpecificOutput.hookEventName` names the event that ran.
- **Augment.** It blocks on exit 2 on `PreToolUse` only. It also blocks on exit 0 with `permissionDecision: "deny"`, a `decision: "block"` (inside `hookSpecificOutput` on `Stop` and `PostToolUse`), or `"continue": false`. A `context` line marks plain stdout on `SessionStart` and `hookSpecificOutput.additionalContext` at exit 0, as on Claude Code.
{% </details> %}

The run exits 1 when a counted command times out or errors, when two counted tools decide differently, or, with `--expect allow` or `--expect block`, when a counted tool decides otherwise. That makes it a CI check.

### Assumed results {#assumed-results}

Cursor, Copilot, Factory, Qoder, Antigravity, Kiro, and Windsurf (Devin CLI) leave out part of how a command runs. `hook run` runs them on these assumptions instead of skipping them:

- Shell (Cursor, Factory, Antigravity, Kiro, Windsurf, Copilot's `command` form, and Qoder without `shell`): `sh -c` on macOS and Linux, only for a script path with plain or single-quoted arguments. A command with shell syntax, such as a pipe, is listed as not run with a note to use a script path. Windows is not run. Copilot's and Qoder's exec form, which sync writes when the spec sets `args`, runs with no shell and assumes none.
- Timeout (Cursor, Windsurf): 30 seconds when the spec sets none. Set `timeout` in the spec to remove this assumption. Copilot and Antigravity document their 30 second default, and Factory and Kiro 60 seconds.
- Timeout (Kiro): 60 seconds for `timeout: 0`, which Kiro documents as no limit.
- Working directory (Copilot): the project root, for a hook without `cwd`. Set `cwd` in the spec to remove this assumption.
- Working directory (Factory, Qoder, Antigravity, Windsurf): the project root, since the docs do not say where a hook command runs. A Factory command written as `"$FACTORY_PROJECT_DIR"/path/to/script.sh`, or a Qoder command written as `"${QODER_PROJECT_DIR}"/path/to/script.sh`, counts as a script path and runs as written, with the variable set to the project root. Windsurf sets `DEVIN_PROJECT_DIR` the same way.
- Exit codes (Antigravity): the docs give no exit code a meaning. Only exit 0 with a reply of the documented shape is counted. Any other result, and the `"ask"` and `"deny_unless_prior_grant"` replies, is shown with a `not counted` note and stays out of `--expect` and the comparison even with `--include-assumed`.
- Exit codes (Kiro): exit 2 blocks `PreToolUse` and `UserPromptSubmit`. Kiro's docs disagree on whether any other non-zero exit blocks them, so that result reads as `error` with a `not counted` note.
- Not run (Kiro): an `agent` action, which runs no command; a hook with `confirm`, which asks the user first; and every trigger but `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, and `Stop`, whose event data the docs do not show.
- Replies (Windsurf): results the Devin CLI docs give no meaning are shown with a `not counted` note.
- Exec path (Copilot): with `cwd` set, a relative exec path resolves from `cwd`.

A `not counted` result stays out of `--expect` and the comparison even with `--include-assumed`, though with `--include-assumed` a timeout or error from another of that tool's commands still fails the run.

The result line ends in `(assumed: shell, timeout)`, followed by one line per assumption. An assumed result is shown but not counted: it stays out of `--expect` and the comparison unless you pass `--include-assumed`. When it disagrees with the counted results, a warning says so, and a summary such as `0 checked, 1 assumed (not counted; --include-assumed to count)` shows what was left out. With `--expect`, a run where only assumed results ran fails and asks for `--include-assumed`, so a CI check never passes on nothing. In JSON, each target has `assumptions` (`item`, `value`, `reason`) and `counted`.

A Cursor hook spec uses Cursor's own event names, so other tools are listed as not run. A Copilot hook spec with a PascalCase event such as `PreToolUse` runs on Claude Code too, which counts, so `--expect` checks Claude Code and shows Copilot beside it.

A Cline run assumes only the working directory: the project root, while Cline uses the directory the CLI started in. The VS Code extension does not read stdout when the script exits non-zero, so a reply printed before a failing exit blocks in the CLI and not in the extension.

A Crush run assumes nothing until a command reaches one of Crush's built-in programs: `jq` and, on Windows, `cat`, `ls`, `rm`, `find`, and the other core utilities, unless `CRUSH_CORE_UTILS=false`. `hook run` runs the ones on `PATH` instead, and adds a `jq` or `coreutils` assumption for each it reached. This applies only to inline commands and `./` scripts without a shebang.

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

A tool that lacks the input a flag needs is listed as not run, and the other tools still run. In each row, `tool_response` holds placeholder values, and session IDs and transcript paths are made up.

| Target | Sample event |
|---|---|
| Claude Code | `--edit` calls the first of `Write`, `Edit`, and `MultiEdit` the matcher matches, with an absolute `tool_input.file_path`. |
| Codex | `--edit` calls `apply_patch` with an `*** Add File:` or `*** Update File:` patch in `tool_input.command`, and `Edit` and `Write` match it. |
| Gemini | `--edit` calls the first of `write_file` and `replace` the matcher matches, with an absolute `tool_input.file_path`; `--bash` calls `run_shell_command`. The tool matcher is an unanchored regular expression, and a `SessionStart` source must match exactly. |
| Trae | `--bash` calls `RunCommand`. Trae lists no edit tool input, so `--edit` lists Trae as not run. The matcher is an unanchored regular expression. |
| OpenHands | `--bash` calls `terminal`. The file editor's input is undocumented, so `--edit` lists OpenHands as not run. A `*` or empty matcher matches all; one with a regex character, or written `/re/`, must match the whole tool name; any other is an exact name. |
| Goose | `--bash` calls `shell`; `--edit` calls the first of `write` and `edit` the matcher matches. `BeforeShellExecution` and `AfterShellExecution` take `--bash`, and `AfterFileEdit` and `BeforeReadFile` take `--edit`. The matcher is an unanchored regular expression on `matcher_context`; one that does not compile, such as `*`, skips the rule. |
| Cursor | `--bash` builds `beforeShellExecution` and `afterShellExecution`, and `preToolUse` and `postToolUse` on the `Shell` tool. `--edit` builds `afterFileEdit` only, so on `preToolUse` and `postToolUse` Cursor is listed as not run. `--prompt` builds `beforeSubmitPrompt`. The matcher is an unanchored regular expression. |
| Crush | Only `PreToolUse` runs. `--bash` calls `bash`; `--edit` calls the first of `edit` and `write` the matcher matches. Crush has no prompt event. The matcher is an unanchored regular expression on the lowercase tool name; one that does not compile makes Crush skip the hook, and `hook run` reports it. |
| Factory | `--bash` calls `Execute`. `--edit` calls `Create`; when the matcher picks `Edit` or `ApplyPatch`, Factory is listed as not run. `--prompt` builds `UserPromptSubmit`. Empty or `*` matches everything, letters and `\|` list exact names, and anything else is a case-sensitive regular expression. |
| Antigravity | `--bash` calls `run_command`. `--edit` calls the first of `write_to_file`, `replace_file_content`, and `multi_replace_file_content` the matcher matches. Antigravity has no prompt event; `PreInvocation`, `PostInvocation`, and `Stop` take `--payload`. The matcher is a regular expression matched against the whole tool name; empty or `*` matches everything. |
| Copilot | `--bash` builds `preToolUse` and `postToolUse` on `bash`, and `PreToolUse` on `Bash`. `--prompt` builds `userPromptSubmitted` or `UserPromptSubmit`. `--edit` is refused, and so is `--bash` on `PostToolUse`. A camelCase matcher is a regex anchored as `^(?:PATTERN)$`. Copilot runs JavaScript regexes, so a matcher with inline flags, lookaround, backreferences, `(?P<name>)`, POSIX classes, or Go-only escapes such as `\A` is listed as not run. |
| Qoder | `--bash` calls `Bash`. `--edit` calls `Write`; when the matcher picks `Edit`, Qoder is listed as not run. Empty or `*` matches everything, letters and `\|` list exact names, and anything else is a regular expression. |
| Cline | `--bash` calls `run_commands`; `--edit` calls `editor` (a model whose id holds `gpt` or `codex` calls `apply_patch` instead, which a note says). `--prompt` builds `UserPromptSubmit`, with a note that the CLI may not send that event. Cline has no matcher, so every hook fires; a portable `match` kind runs the script's tool name check, which allows a call to another tool. `PreCompact` is listed as not run. |
| Kiro | `--prompt` builds `UserPromptSubmit`, and `Stop` needs no input. `--bash` and `--edit` are refused, so `PreToolUse` and `PostToolUse` take `--payload`. The matcher is an unanchored regular expression on the tool name or its alias, and on the prompt text for `UserPromptSubmit`; `Stop` ignores it. `@mcp`, `@powers`, and `@builtin` are listed as not run. |
| Windsurf | `--bash` builds `PreToolUse`, `PostToolUse`, and `PermissionRequest` on `exec`. `--prompt` builds `UserPromptSubmit`; `Stop` and `PostCompaction` need no input. `--edit` is refused. `SessionStart` and `SessionEnd` take `--payload`. The matcher is an unanchored regular expression on `tool_name`, so a Claude-style `Bash` matcher does not fire; a matcher on another event is listed as not run. |
| Augment | `--bash` calls `launch-process`; `--edit` calls the first of `str-replace-editor` and `save-file` the matcher matches. Augment has no prompt event. The matcher is an unanchored regular expression. |

Gemini matchers compile as Go regular expressions, which reject a few JavaScript forms such as lookahead. Gemini CLI would run those, and `hook run` compares them as a literal name. `GEMINI_PLANS_DIR` is not set.

OpenCode and Kilo run hooks as plugins and Zed as tasks, with no event data on stdin, so `hook run` lists them as not run.

## Sources

Vendor docs and pinned source each tool's hook behavior is checked against:

- **Claude Code:** [Hooks guide](https://code.claude.com/docs/en/hooks-guide).
- **Codex:** [Hooks](https://learn.chatgpt.com/docs/hooks).
- **Copilot:** [Hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference), [using hooks with Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks).
- **Cursor:** [Hooks](https://cursor.com/docs/hooks).
- **Gemini CLI:** [Hooks reference](https://geminicli.com/docs/hooks/reference/), [file system tools](https://geminicli.com/docs/tools/file-system/), source [`c6bccb7`](https://github.com/google-gemini/gemini-cli/tree/c6bccb7ecbf6d8368d995455dd725ed34466faad/packages/core/src/hooks).
- **Qoder:** [Hooks](https://docs.qoder.com/cli/hooks), [hooks reference](https://docs.qoder.com/cli/hooks-reference).
- **Factory:** [Hooks](https://docs.factory.com/harness/hooks), [hooks guide](https://docs.factory.com/cli/configuration/hooks-guide).
- **Devin CLI:** [Hooks](https://docs.devin.ai/cli/extensibility/hooks), [lifecycle hooks](https://docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks).
- **Antigravity:** [Hooks](https://antigravity.google/docs/hooks).
- **Kiro:** [Hooks](https://kiro.dev/docs/hooks), [hook types](https://kiro.dev/docs/hooks/types), [hook actions](https://kiro.dev/docs/hooks/actions).
- **Trae:** [automate actions with hooks](https://docs.trae.cn/ide_automate-actions-with-hooks), [hook reference](https://docs.trae.cn/ide_hook-configuration-reference).
- **Augment:** [Hooks](https://docs.augmentcode.com/cli/hooks).
- **OpenHands:** [Hooks](https://docs.openhands.dev/openhands/usage/customization/hooks), source [`fad6377`](https://github.com/OpenHands/software-agent-sdk/tree/fad63774459171b08f889b12fd3b4d6346168c3e/openhands-sdk/openhands/sdk/hooks).
- **Goose:** [Hooks](https://goose-docs.ai/docs/guides/context-engineering/hooks/), source [`bab8ff6`](https://github.com/aaif-goose/goose/blob/bab8ff641039c9cd3331121cd84a5c6045f365ca/documentation/docs/guides/context-engineering/hooks.md) and [runner](https://github.com/aaif-goose/goose/blob/bab8ff641039c9cd3331121cd84a5c6045f365ca/crates/goose/src/hooks/mod.rs).
- **Cline:** source [`39ff235`](https://github.com/cline/cline/tree/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks): [`hook-file-hooks.ts`](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks/hook-file-hooks.ts#L234-L254), [`subprocess-runner.ts`](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks/subprocess-runner.ts#L68-L97), [tool schemas](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/extensions/tools/schemas.ts#L149-L224).
- **Crush:** source [`76cc5c5`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal): [`config/config.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/config/config.go), [`hooks/input.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/hooks/input.go), [`hooks/runner.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/hooks/runner.go), [`shell/dispatch.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/shell/dispatch.go), [`shell/run.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/shell/run.go).
- **`hook run` shell:** [mvdan.cc/sh](https://github.com/mvdan/sh) and its [core utilities](https://github.com/mvdan/sh/blob/b5028a3332a4d5d5a6cc62a999c1ba7993eb3627/x/coreutils/coreutils.go), [gojq](https://github.com/itchyny/gojq).
