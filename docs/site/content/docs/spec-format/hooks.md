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

- **Safety checks.** Block a force push or a write to a generated file before it happens.
- **Automatic follow-up.** Format or lint a file right after the agent edits it.
- **Fresh context.** Print the branch, open issues, or service status when a session starts.
- **One script, many tools.** Tools that share event names, such as Claude Code and Codex, run one spec. `AGNOSTIC_AI_TARGET` tells a shared script which tool called it.

Sync never runs a hook. The configured tools do. [`agnostic-ai hook run`](#hook-run) runs one when you ask. Review hook specs like code.

## Write one

`agnostic-ai new hook session-status` creates `hooks/session-status.yaml`. Pure YAML, no markdown body.

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

Codex takes `Edit` and `Write` as aliases for `apply_patch`, so the one matcher fires on both tools. `agnostic-ai` must be on the hook's `PATH`. The `|| exit 1` fails the hook when `hook paths` fails, for example on a missing target, bad JSON, or a missing binary. A plain pipe into the loop would exit 0.

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

Command hooks receive event JSON on stdin. Read the shell command from the target's `tool_input` fields. Read the edited paths with [`agnostic-ai hook paths`](#edited-paths). `AGNOSTIC_AI_TARGET` names the target that ran the hook; see [which target ran a hook](#hook-target).

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Hook identifier. |
| `description` | no | empty | Free-form documentation. |
| `event` | yes | none | Hook event, written verbatim. See [events](#events). |
| `matcher` | no | empty | Regex on the tool name, or another event-specific selector. |
| `command` | command handlers only | none | Shell command, or a list where each entry becomes its own handler. |
| `args` | no | empty | Switches to **exec form**: `command` runs as an executable with `args` as its argument vector and no shell, so spaces, `$`, and backticks pass verbatim. Leave unset when the command needs a pipe or `&&`. Codex, Gemini, and Cursor have no exec form, so they get the args folded into `command`, each quoted for a POSIX shell. |
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

`event` is written verbatim. Names are never translated between tools.

- Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`, so one spec feeds both.
- Other tools need their own names, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`.
- `agnostic-ai validate` flags an event a target does not recognize.
- `sync` and `sync --check` stop before writing on an event that is a likely typo of a known one, such as `PreToolUze`, and name the closest.
- Targets without hook support log a warning and skip.

## Shared hook scripts

Keep one script under `.agnostic-ai/scripts/` and reference that path in `command`. `agnostic-ai init --demo` seeds this guard, which blocks `git push --force` and allows `--force-with-lease` on Claude Code and Codex:

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

The script reads `tool_input.command` from the event JSON on stdin, with no `jq`. It splits the command into words the way `sh` does, so a `git push --force` in quotes, a comment, or a heredoc body passes, and a push with a `+main` refspec counts as forced unless it uses `--force-with-lease`. On a force push it prints the reason on stderr and exits 2, which both tools read as a block.

Both tools start a hook in the session directory, which can be below the project root, so the path starts at the root. Claude Code keeps [`$CLAUDE_PROJECT_DIR`](#imported-project-root-paths); Codex gets `$(git rev-parse --show-toplevel)`, in both commands, plus the project's path below the Git root.

On Windows, Codex runs `commandWindows` with `powershell.exe -Command`:

- PowerShell ignores the script's `#!/bin/sh` line, so the command names `sh`. That needs `sh` on `PATH`, as Git for Windows provides.
- PowerShell reports a failed native command as exit 1, which Codex reads as a hook error and lets the push run. `exit $LASTEXITCODE` passes on the script's exit 2.
- The Git root lookup runs first and resets `$LASTEXITCODE`, so the command checks for `sh` with `Get-Command` and exits 1 when it is missing. Codex reports that as a failed hook instead of passing it.

Claude Code runs hooks with Git Bash on Windows and needs no `commandWindows`.

{% <details summary=".agnostic-ai/scripts/no-force-push.sh"> %}
```sh
#!/bin/sh
# Blocks git push --force, -f, or a +refspec, and lets --force-with-lease
# through. It catches a mistake. It is not a sandbox.

# Reads tool_input.command from the hook JSON on stdin and splits it into
# words the way sh would: quotes, backslashes, line continuations,
# comments, redirections, heredoc bodies, and $( ) or backtick
# substitutions. It checks each command between unquoted ; & | ( ) and
# newlines, past reserved words such as if and then. A heredoc it cannot
# read ends the check, since a missed push beats blocking text.
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
    if (a ~ /^(command|exec|env|nohup|nice|sudo)$/) { wrapper = a; continue }
    if (wrapper != "" && a ~ /^-/) {
      if ((wrapper == "env" && a ~ /^-[uCS]$/) || (wrapper == "exec" && a == "-a") || (wrapper == "nice" && a == "-n") || (wrapper == "sudo" && a ~ /^-[ugCDpUrtTR]$/)) i++
      continue
    }
    if (i > 1 && words[i - 1] == "time" && a == "-p") continue
    return i
  }
  return n + 1
}

function check(   i, j, k, a, plus, lease, positional) {
  i = program()
  if (i > n || words[i] !~ /(^|\/)git$/) return
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
      continue
    }
    if (mode == "double") {
      if (c == q) mode = ""
      else if (c == "$" && e == "(") { p++; open_nested("$(", "double"); mode = "" }
      else if (c == "`") { open_nested("`", "double"); mode = "" }
      else w = w c
      continue
    }
    if (c == q) { mode = "double"; inword = 1; wq = 1; continue }
    if (c == sq) { mode = "single"; inword = 1; wq = 1; continue }
    if (c == "#" && !inword) {
      while (p < size && substr(cmd, p + 1, 1) != "\n") p++
      continue
    }
    if (c == "$" && e == "(" && substr(cmd, p + 2, 1) != "(") { p++; open_nested("$(", ""); continue }
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
      end_command()
      if (c == "\n" && pending) p = skip_bodies(p)
      continue
    }
    w = w c
    inword = 1
  }
  if (!blocked && !stopped) end_command()
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

A guard like this catches a mistake. It is not a sandbox: a command written to get around it, such as a push from a script file or an alias, gets through. To stop a command for good, use [`permissions.deny`](@/docs/spec-format/settings.md#permission-rules) and each tool's own sandbox: [Claude Code](https://code.claude.com/docs/en/sandboxing), [Codex](https://learn.chatgpt.com/docs/sandboxing).

`sync` copies the script into each target's script directory and rewrites the command to run that copy.

- Script bytes, permissions, and subdirectories under `scripts/` stay intact.
- Make the source executable when the command runs it directly.
- A missing referenced script fails sync.
- A file at `.agnostic-ai/scripts/<target>/guard.sh` overrides the shared body for that target.

For a shell command that starts an interpreter, quote the path: `command: 'node ".agnostic-ai/scripts/guard.mjs"'`. The reference can also follow a [project-root variable](#imported-project-root-paths). Existing `.claude/hooks/`, `.codex/hooks/`, and `.gemini/hooks/` references keep their current behavior.

`sync --global` reads bodies from the global source root's `scripts/` directory and copies them into the selected tools' user hook directories. Its commands use absolute paths to those user copies. Script sharing does not translate event names or reply formats, so choose ones each target supports.

{% <details summary="Where each target keeps its scripts"> %}
Most targets use `.<target>/hooks/`. The exceptions:

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

A shell-form command imported from Claude Code can use `$CLAUDE_PROJECT_DIR` or `${CLAUDE_PROJECT_DIR}` to name a script:

```yaml
name: guard
event: PreToolUse
command: 'sh "$CLAUDE_PROJECT_DIR/.claude/hooks/guard.sh"'
```

What `sync` writes for the variable:

- Claude Code, Cursor, and Trae provide it, so sync keeps it.
- Gemini, Qoder, and Factory use their native project-root variable.
- Other targets use `$(git rev-parse --show-toplevel)` in a POSIX shell. Codex uses it in `commandWindows` too, since PowerShell reads `$(...)` the same way.

If the configuration lives in a subdirectory of a Git worktree, the emitted path includes that subdirectory. A hook started from a descendant still resolves to the configured project. The sibling hook-directory rewrite applies too, such as `.claude/hooks/` to `.codex/hooks/`. Custom script paths keep their directory.

{% <details summary="Global sync"> %}
`sync --global` uses the runtime Git root for those targets. It never binds a command to the global specs checkout. This fallback needs Git and a hook running inside the intended Git worktree. It cannot locate a project outside Git, or tell apart multiple configured projects within one worktree.
{% </details> %}

These stay literal: exec-form `args`, escaped dollars, and single-quoted variables.

These get a note naming the hook when the target cannot preserve them:

- parameter operators such as `${CLAUDE_PROJECT_DIR:-fallback}` and <code>${&#35;CLAUDE_PROJECT_DIR}</code>
- nested substitutions and here-documents
- non-POSIX root syntax
- a project without a Git worktree

`on-unsupported: error` fails the sync; `silent` hides the note. Use a target-specific command or `target: claude` for these cases. For Windows, set the target's Windows command explicitly.

## Which target ran a hook {#hook-target}

A shared script reads `AGNOSTIC_AI_TARGET` to pick the reply protocol (Claude Code and Codex read exit code 2 and stderr; Cursor reads JSON).

```sh
case "$AGNOSTIC_AI_TARGET" in
  cursor) echo '{"permission":"deny","agent_message":"Blocked."}' ;;
  *) echo "Blocked." >&2; exit 2 ;;
esac
```

A spec that sets `AGNOSTIC_AI_TARGET` in its own `env` keeps that value. Where sync cannot set it, the parent process's value stays, such as `claude` for a tool started from Claude Code. Sync sets it per target:

- [Claude Code](@/docs/targets/claude.md): `env` in `.claude/settings.json` (`~/.claude/settings.json` for `sync --global`). Set for the whole session, so the Bash tool sees it too.
- [Cursor](@/docs/targets/cursor.md): a `sessionStart` hook returns the variable. `sessionStart` hooks and hooks that fire before it returns do not see it.
- [Codex](@/docs/targets/codex.md): `export AGNOSTIC_AI_TARGET=codex; ` before `command`. Not set on Windows (`commandWindows`), and needs a POSIX session shell (a `pwsh` or `nu` login shell breaks it).
- [Gemini](@/docs/targets/gemini.md), [Qoder](@/docs/targets/qoder.md), [Copilot](@/docs/targets/copilot.md): `env` on each command handler.
- [Goose](@/docs/targets/goose.md), [Crush](@/docs/targets/crush.md), [Cline](@/docs/targets/cline.md): `export` prefix or line.
- [OpenCode](@/docs/targets/opencode.md), [Kilo](@/docs/targets/kilo.md): `.env()` on each plugin command. [Zed](@/docs/targets/zed.md): `env` on each task.

Trae, Factory, OpenHands, Antigravity, Kiro, Windsurf, and Augment do not get the variable. None has a per-hook `env`, and a prefix would break hooks that work today. Tell them apart by their own variables, such as `TRAE_PROJECT_DIR`, `FACTORY_PROJECT_DIR`, `OPENHANDS_PROJECT_DIR`, `DEVIN_PROJECT_DIR`, or `AUGMENT_PROJECT_DIR`. `CLAUDE_PROJECT_DIR` does not identify Claude Code, because Cursor and Trae provide it too.

`sync --global` leaves a matching hand-written hook alone, so an adopted Codex, Gemini, or Qoder entry does not get the variable. Cursor and Copilot also run `.claude/settings.json` hooks but read no `env` from it. Cursor still gets `cursor` from `sessionStart`. Copilot gets nothing.

## Read the edited paths {#edited-paths}

`agnostic-ai hook paths` reads a hook payload on stdin and prints each file the edit leaves on disk, one per line.

- Paths print relative to the directory the hook runs in. A path outside it prints in full.
- A tool call that is no edit prints nothing.
- The target comes from `AGNOSTIC_AI_TARGET`, or from `--target`, which wins. Codex on Windows (`commandWindows`) gets no `AGNOSTIC_AI_TARGET`, so pass `--target codex` there.
- With no target, a payload whose `tool_input` object holds `file_path`, `edits`, or `notebook_path` reads as Claude Code. Cursor and Copilot run `.claude/settings.json` hooks with that payload and no variable.
- Any other payload with no target fails. Invalid JSON, and an edit tool's `tool_input` that is not an object, fail too (exit 1).

```sh
agnostic-ai hook paths            # src/app.go
agnostic-ai hook paths --action   # update<TAB>src/app.go
agnostic-ai hook paths --json     # [{"action": "update", "path": "src/app.go"}]
```

A plain run skips deleted files and the source of a move, so a formatter sees only files that exist. `--action` and `--json` list every change as `add`, `update`, `delete`, or `move`.

- A move reads as a `delete` of its source and a `move` of its destination. `--json` gives the destination a `from`.
- When a tool does not say whether a write created the file, the change reads as `update`.
- A hook that runs before the edit sees no new file on disk yet, so run formatters after the edit.

| Target | What it reads | Vendor docs |
|---|---|---|
| Claude Code | `tool_input.file_path` of `Edit`, `Write`, and `MultiEdit`; `tool_input.notebook_path` of `NotebookEdit` | [Hooks guide](https://code.claude.com/docs/en/hooks-guide) |
| Codex | The `*** Add File:`, `*** Update File:`, `*** Delete File:`, and `*** Move to:` headers of the `apply_patch` body in `tool_input.command`, every file in the patch | [Hooks](https://learn.chatgpt.com/docs/hooks) |
| Cursor | `file_path` of `afterFileEdit` and `afterTabFileEdit` only; other Cursor events print nothing | [Hooks](https://cursor.com/docs/hooks) |
| Gemini | `tool_input.file_path` of `write_file` and `replace`, a relative path starting at `cwd` | [Hooks reference](https://geminicli.com/docs/hooks/reference/), [file system tools](https://geminicli.com/docs/tools/file-system/) |
| Factory | `tool_input.file_path` of `Create` and `Edit` | [Hooks](https://docs.factory.com/harness/hooks) |
| Qoder | `tool_input.file_path` of `Write`; `Edit` fails, since the docs show none of its `tool_input` fields | [CLI hooks](https://docs.qoder.com/cli/hooks) |
| Augment | `file_changes[].path` with its `changeType`; before the edit, `tool_input.path` of `str-replace-editor` and `save-file` | [Hooks](https://docs.augmentcode.com/cli/hooks) |

Factory and Augment do not get `AGNOSTIC_AI_TARGET`, so give their hooks their own spec with `--target`. Factory documents no input for `ApplyPatch`, so a Factory patch prints nothing.

{% <details summary="Targets the command does not read"> %}
The command fails for these targets:

- Windsurf: sync writes Devin CLI hooks, and the [Devin CLI docs](https://docs.devin.ai/cli/extensibility/hooks) name the `edit`, `write`, and `apply_patch` tools but not their `tool_input` fields.
- Trae: the docs list no `tool_input` fields for the edit tools.
- Copilot: the docs list no `toolArgs` keys for `edit`, `create`, or `apply_patch`.
- Goose, Antigravity: the docs name the edit tools' arguments but show no edit hook payload.
- OpenHands: the docs name no file edit tool.
- Kiro: the docs list no `fs_write` input. A `PostFileSave` command can use its `filePath` template variable.
- Cline: the current docs do not describe the script payload.
- Crush: only `PreToolUse` runs, and it sets `CRUSH_TOOL_INPUT_FILE_PATH`.
- OpenCode and Kilo: plugins get tool arguments as JavaScript objects, not a payload on stdin.
- Zed: no hook fires on an edit.
{% </details> %}

## Test a hook {#hook-run}

`agnostic-ai hook run <hook>` runs a hook spec the way each target would, before a session fires it. A hook that blocks on Claude Code and does nothing on Codex shows up here, not in a live session:

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

For each configured target the hook reaches, it runs every command sync wrote for that target, from the project root, or from the handler's `cwd` on Copilot.

- **Payload.** `--edit <path>` and `--bash <command>` build a `PreToolUse` or `PostToolUse` tool call (`BeforeTool` or `AfterTool` on Gemini). `--prompt <text>` builds `UserPromptSubmit` (`BeforeAgent` on Gemini). `SessionStart` takes its `source` from the matcher, or `startup` when the matcher is empty. `--payload <file>` sends a JSON file as is, for any event.
- **Matcher.** If the matcher does not match the tool or source, the target would not run the hook. That target reports `allow` and why.
- **Matcher on `--payload`.** A tool call from `--payload` matches its `tool_name` with the target's matcher rules, including Codex's `Edit` and `Write` aliases for `apply_patch`. On Cursor, the matcher tests what Cursor documents for the event: the `command`, `tool_name`, or `subagent_type`, or a fixed name such as `Read` for `beforeReadFile`. Goose uses the supplied `matcher_context` on every event: the tool name, shell command, file path, or prompt text. On Copilot, the matcher tests `toolName`, `notification_type`, `trigger`, or `agentName`, and a PascalCase `PreToolUse` or `PermissionRequest` uses Claude-format matchers on `tool_name`. On Kiro, it tests `tool_name` or its documented alias, such as `shell` for `execute_bash`, and the `prompt` on `UserPromptSubmit`.
- **Missing event.** A target without the hook's event, such as Codex for `Notification`, is listed as not run.
- **Timeout.** The timeout sync writes (Gemini's is in milliseconds), or the tool's default.
- **Async.** An `async: true` hook, or an `asyncRewake: true` one on Claude Code and Qoder, runs and prints its output. Its result is `not judged` and stays out of `--expect` and the comparison, because Claude Code, Codex, OpenHands, and Qoder do not wait for it. Cursor `sessionStart` and `sessionEnd` hooks, Copilot `notification` hooks, and Cline hooks on any event but `PreToolUse` and `PostToolUse` are `not judged` too, because the tool runs them fire-and-forget.
- **Background commands.** On macOS and Linux, a command the hook leaves running is killed once the hook exits.
- **Stale native file.** Commands come from the spec, so the run works before a sync. The target prints a `warning` naming the file when the synced file is missing, or when no handler under the event runs the command the spec produces. It also warns when that handler's group matcher, timeout, or (on Gemini) `env` differs from the spec. An `env` warning names the keys that differ, never their values. A warning does not fail the run; `sync` clears it. The files are `.claude/settings.json`, `.codex/hooks.json`, `.gemini/settings.json`, `.trae/hooks.json`, `.openhands/hooks.json`, the Goose plugin `hooks/hooks.json`, `.augment/settings.json`, `.cursor/hooks.json`, `.github/hooks/agnostic-ai.json`, `crush.json`, `.factory/hooks.json`, `.qoder/settings.json`, `.agents/hooks.json` (Antigravity), `.kiro/hooks/<name>.json`, `.devin/hooks.v1.json` (Windsurf), `.cline/hooks/<Event>.sh`, or the path `outputs` sets. On Cursor it also compares `failClosed`, on Copilot `cwd`, and on Kiro `confirm`.

{% <details summary="Codex sync checks"> %}
A Codex matcher that joins this spec's segments with another spec's counts as in sync, since sync merges them. So does a Codex timeout when the spec sets none. On Windows, Codex compares `commandWindows`.
{% </details> %}

{% <details summary="Environment variables per target"> %}
Variables listed here are removed from the calling shell's env first. Other variables pass through.

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
| Crush | `CRUSH=1`, `AGENT=crush`, `AI_AGENT=crush`, `CRUSH_EVENT`, `CRUSH_TOOL_NAME`, `CRUSH_SESSION_ID`, `CRUSH_CWD`, `CRUSH_PROJECT_DIR`, `CRUSH_TOOL_INPUT_COMMAND` or `CRUSH_TOOL_INPUT_FILE_PATH` when the tool input has one, and `TERM`, `EDITOR`, `VISUAL`, `GIT_EDITOR`, `PAGER`, `GIT_PAGER`, `JJ_EDITOR`, and `JJ_PAGER` set so nothing waits for a terminal; `AGNOSTIC_AI_TARGET=crush` comes from its command |
| Copilot | the handler's `env`, which holds `AGNOSTIC_AI_TARGET=copilot`. A value with `$` is listed as not run, since Copilot does not document its expansion syntax. |
| Factory | `FACTORY_PROJECT_DIR` |
| Qoder | `QODER_PROJECT_DIR`, then the handler's `env`, which holds `AGNOSTIC_AI_TARGET=qoder` |
| Antigravity | none; the docs name no hook variables |
| Cline | none; the script exports `AGNOSTIC_AI_TARGET=cline` |
| Kiro | `USER_PROMPT` on `UserPromptSubmit`, which the Kiro IDE documents |
| Windsurf | `DEVIN_PROJECT_DIR` |
{% </details> %}

{% <details summary="Shell and default timeout per target"> %}
| Target | Shell | Default timeout |
|--------|-------|-----------------|
| Claude Code | `bash -c`; exec-form `args` with no shell; `shell: powershell` with PowerShell | 600 seconds, except 30 on `UserPromptSubmit`, `PreModelSwitch`, and `PostModelSwitch`, and 10 on `MessageDisplay` |
| Codex | `sh -c`; on Windows, `commandWindows` with `powershell.exe -Command` | 600 seconds |
| Gemini | `bash -c`, or Windows PowerShell on Windows, after Gemini's own replacement of `$GEMINI_PROJECT_DIR`, `$GEMINI_CWD`, `$GEMINI_SESSION_ID`, and `$CLAUDE_PROJECT_DIR` with the quoted root | 60 seconds |
| Trae | `bash -c`, or PowerShell on Windows | 30 seconds |
| OpenHands | `/bin/sh -c`, or `cmd.exe /c` on Windows, as Python's `shell=True` does | 60 seconds |
| Goose | `sh -c` on every platform | 30 seconds |
| Augment | Only a `.sh`, `.ps1`, `.cmd`, or `.bat` script path: the script itself on macOS and Linux, a `.ps1` with `powershell.exe -Command` and a `.cmd` or `.bat` with `cmd.exe /c` on Windows. An inline command is listed as not run. | 60 seconds |
| Cursor | Assumed `sh -c`, for a script path and plain or single-quoted arguments only. A command with shell syntax, and any command on Windows, is listed as not run. | Assumed 30 seconds |
| Crush | Crush's embedded POSIX shell ([mvdan.cc/sh](https://github.com/mvdan/sh) v3.14.1, in process) on every platform, Windows included. A path starting with `./`, `../`, or `/` runs through its shebang interpreter, found on `PATH` by name when the literal path is missing, or, with no shebang, as shell source in the same shell. Sync writes synced scripts as `./.crush/hooks/<name>`. A path without that prefix, such as `hooks/guard.sh`, starts as a program; on macOS and Linux one without a shebang then fails with exit 1. | 30 seconds |
| Copilot | Exec-form `args` with no shell. A `command` runs under an assumed `sh -c`, for a script path and plain or single-quoted arguments only; a command with shell syntax, and a `command` on Windows, is listed as not run. | 30 seconds |
| Factory | Assumed `sh -c`, for a script path and plain or single-quoted arguments only, where `"$FACTORY_PROJECT_DIR"` counts as a plain word. A command with shell syntax, and any command on Windows, is listed as not run. | 60 seconds |
| Qoder | Exec-form `args` with no shell; `shell: bash` with `bash -c` on macOS and Linux. Without `shell`, an assumed `sh -c`, for a script path and plain or single-quoted arguments only, where `"$QODER_PROJECT_DIR"` or `"${QODER_PROJECT_DIR}"` counts as a plain word. A command with shell syntax, `shell: powershell`, exec form with a `$` in `command` or `args`, and a shell-form command on Windows are listed as not run. | 600 seconds, from the Qoder CLI docs; the IDE's 30 seconds does not apply to `.qoder/settings.json` |
| Antigravity | Assumed `sh -c`, for a script path and plain or single-quoted arguments only. A command with shell syntax, and any command on Windows, is listed as not run. | 30 seconds |
| Cline | `bash`, as Cline runs `bash <file>`: the event script sync writes, holding this spec's commands only, with the script's path as `$0`. Windows is listed as not run. | 120 seconds on `PreToolUse` and `PostToolUse`, whatever the spec sets, since sync drops `timeout` |
| Kiro | Assumed `sh -c`, for a script path and plain or single-quoted arguments only. A command with shell syntax, and any command on Windows, is listed as not run. | 60 seconds. `timeout: 0` turns Kiro's limit off; `hook run` still stops the command after 60 seconds and marks the timeout assumed. |
| Windsurf | Assumed `sh -c`, for a script path and plain or single-quoted arguments only. A command with shell syntax, and any command on Windows, is listed as not run. | Assumed 30 seconds |
{% </details> %}

{% <details summary="Claude Code `if` rules"> %}
A handler's `if` permission rule decides whether it runs, as in Claude Code. It applies only on `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, and `PermissionDenied`, never on other events.

- `Bash(git *)` matches any subcommand of `&&`, `||`, `;`, `|`, and `&`, and commands inside `$()` and backticks, after leading `VAR=value` assignments and wrappers such as `timeout 30` are stripped.
- A trailing ` *` also matches the bare command. `:*` is the same as ` *`.
- A pattern naming more than the command runs on any `$()`, backtick, or `$VAR`. A command that does not parse always runs.
- `Edit(path)` covers `Edit`, `Write`, `MultiEdit`, and `NotebookEdit`, with gitignore patterns. `//path` is from the filesystem root, `~/path` from home, and `/path` and `path` from the project root.
- A bare name such as `.env` matches at any depth. A single directory such as `src/**` matches only under the project root, as in Claude Code v2.1.214 and later.
- `Tool(param:value)` matches a top-level input field.

See the [`if` field](https://code.claude.com/docs/en/hooks) and [permission rule syntax](https://code.claude.com/docs/en/permissions#permission-rule-syntax).
{% </details> %}

Each command reports one decision.

- `block`: exit 2, or exit 0 with a JSON reply that has `"permissionDecision": "deny"`, `"decision": "block"`, or `"continue": false`.
- `allow`: exit 0 otherwise.
- `error`: any other non-zero exit.
- `timeout`: the command ran past its timeout.

Exit 2 cannot stop anything on `SessionStart`, `SessionEnd`, `Notification`, `PreCompact`, and `PostCompact`, so it reads as `error` there. On `PostToolUse` the tool already ran, so `block` sends stderr back to the model. A `context` line marks output the target adds to the session: plain stdout on `SessionStart` and `UserPromptSubmit`, or a JSON reply's `additionalContext`.

{% <details summary="How other targets read replies"> %}
- **Gemini.** It reads stdout, or stderr when stdout is empty. A JSON reply with `"decision": "deny"` or `"block"`, or `"continue": false`, is `block`. Plain text with an exit other than 0 and 1 is `block`. With no text at all, Gemini decides nothing and the run reads `error`. Exit 1 is `error`. `SessionStart`, `SessionEnd`, `Notification`, and `PreCompress` ignore decisions, so a block there reads as `error`. Only a JSON `additionalContext` reaches the model; plain stdout is a message for the user.
- **Trae.** It reads replies as Claude Code does.
- **OpenHands.** It blocks on exit 2, or on a JSON `"decision": "deny"` or `"continue": false` whatever the exit code. It acts on a block only on `PreToolUse`, `UserPromptSubmit`, and `Stop`.
- **Goose.** It blocks on `PreToolUse` and `Stop` only: on exit 2, or on stdout starting with `{` whose `decision` is `"block"`, whatever the exit code. Exit 0 with empty stdout or `"decision": "allow"` allows. Anything else is no decision, read as `error`, or as `block` when the action sets `x-goose.on_failure: block`.
- **Cursor.** It blocks on exit 2 on the permission events (`beforeShellExecution`, `beforeMCPExecution`, `beforeReadFile`, `beforeTabFileRead`, `subagentStart`, `preToolUse`) and `beforeSubmitPrompt`. At exit 0, a permission event blocks on `"permission": "deny"`, on `"ask"` (except on `preToolUse`, which does not enforce it; a note says Cursor asks the user), and on output that is not a JSON reply, names another permission, or gives a listed field the wrong type, such as a `user_message` that is not a string. No output at all is a failure, which fails open. `beforeSubmitPrompt` blocks on `"continue": false`. Any other failure fails open as `error`, or blocks with `failClosed: true`.
- **Crush.** Exit 2 blocks the tool call. Exit 49 blocks too and halts the whole turn, which a note says. Any other non-zero exit, a timeout, and a command that does not parse do not block. At exit 0, a JSON reply with `"decision": "deny"` (any case) or `"halt": true` blocks. A reply with a `hookSpecificOutput` key, even `null`, is read as Claude Code's instead: `"permissionDecision": "deny"` blocks, and the top-level fields are ignored. A field of the wrong type, such as a `reason` that is not a string, voids the reply, which then allows. A `context` line marks a reply's `context` or `additionalContext`, which Crush appends to the tool result.
- **Factory.** It blocks on exit 2, except on `Notification`, `SubagentStop`, `PreCompact`, `SessionStart`, and `SessionEnd`, where exit 2 only shows stderr and reads as `error`. At exit 0, `PreToolUse` blocks on `permissionDecision` `"deny"` or `"ask"` (a note says Factory asks the user); `PostToolUse`, `UserPromptSubmit`, `Stop`, and `SubagentStop` block on `"decision": "block"`; and every event but `SessionEnd` blocks on `"continue": false`.
- **Copilot.** It drops `{"type": "progress"}` lines from stdout and parses the rest as one JSON reply; text that does not parse, or two objects, is no reply. `preToolUse` blocks on any non-zero exit, a command that did not start, `"permissionDecision": "deny"`, or `"ask"` (a note says Copilot CLI asks the user). A failure other than exit 2 still blocks but also counts as `error`, so `--expect block` fails on a hook that never ran. `permissionRequest` blocks on exit 2 or `"behavior": "deny"`, after merging the commands' replies in order, so a later `"behavior": "allow"` overrides an earlier deny. `agentStop` and `subagentStop` block on `"decision": "block"`. Other events cannot block: exit 2 and other failures read as `error`, except exit 2 on `postToolUseFailure`, which adds context. A timeout fails open on every event. PascalCase events decide as their camelCase twins.
- **Qoder.** It blocks on exit 2 on `UserPromptSubmit`, `PreToolUse`, `Stop`, `SubagentStop`, `PreCompact`, `ConfigChange`, `Elicitation`, and `ElicitationResult`, and on any non-zero exit on `WorktreeCreate`. Other failures read as `error`, and `StopFailure` and `InstructionsLoaded` ignore the result. At exit 0, a JSON reply blocks on `"continue": false`, on `"decision": "deny"` where exit 2 blocks, on `PreToolUse` on `permissionDecision` `"deny"` or `"ask"` (a note says Qoder asks the user), which takes precedence over `decision`, on `PermissionRequest` on a `decision.behavior` of `"deny"`, and on the elicitation events on an `action` of `"decline"` or `"cancel"`. A `hookSpecificOutput` without `hookEventName` voids the reply and reads as `error`. A `ConfigChange` with `source: policy_settings` cannot block, so a block there reads as `error` with a note. `TaskCreated`, `TaskCompleted`, `TeammateIdle`, and `Setup` have no documented decision rules, so Qoder is listed as not run for them. A handler's `if` runs it only when the tool name matches, with the matcher's rules, and the glob in parentheses matches the `command` of `Bash` or the `file_path` of a file tool; another tool with a glob is listed as not run.
- **Antigravity.** Only exit 0 with a JSON reply of the documented shape counts. On `PreToolUse`, `"decision": "allow"` allows; `"deny"` blocks; `"force_ask"` blocks (a note says Antigravity asks the user). `"ask"` and `"deny_unless_prior_grant"` read as `block` but are not counted, since a saved Always Allow setting or a prior grant lets the call run at once, and `hook run` cannot see either. On `Stop`, `"decision": "continue"` keeps the agent running and reads as `block`; any other value allows. `PostToolUse`, `PreInvocation`, and `PostInvocation` allow on a JSON object. A missing or unlisted `decision`, a field of the wrong type, output that is not a JSON object, a non-zero exit, a timeout, and a command that did not start read as `error` or `timeout` and are [not counted](#assumed-results).
- **Cline.** It never reads the exit code. It reads stdout as JSON: the last line starting `HOOK_CONTROL` and a tab, else the whole stdout. On `PreToolUse` and `PostToolUse`, an object with `"cancel": true` blocks, and Cline stops the run; on `PostToolUse` the tool already ran. Empty stdout and any other JSON allow, whatever the exit code; a non-zero exit adds a note that Cline ignores it. Stdout that is not JSON reads as `error`, and a timeout as `timeout`, each with a note that Cline logs it and goes on as if the hook allowed. A `context` line marks a reply's `context`, `contextModification`, or `errorMessage`. When other specs reach Cline on the same event, sync writes them into one script that shares stdout, so the result is [not counted](#assumed-results), even with `--include-assumed`, and a note names those specs.
- **Kiro.** It blocks on exit 2 on `PreToolUse` and `UserPromptSubmit`. Another non-zero exit there reads as `error` and is [not counted](#assumed-results), since Kiro's docs disagree on whether it blocks. On `PostToolUse` and `Stop`, any non-zero exit is `error`. At exit 0, `Stop` reads `"decision": "block"` from a JSON reply as `block`, since Kiro keeps the agent running (a note says so); other events read no reply. A `context` line marks stdout on `UserPromptSubmit` and a `Stop` block's `reason`.
- **Windsurf.** Devin CLI blocks on exit 2, or on exit 0 with `"decision": "block"`, on `PreToolUse`, `PermissionRequest`, `UserPromptSubmit`, and `Stop`. `"decision": "approve"`, no decision, and plain stdout allow. Any other non-zero exit is `error`. A block on `PostToolUse`, `PostCompaction`, `SessionStart`, or `SessionEnd`, a reply on a non-zero exit (except exit 2 with `"decision": "block"`), and a `decision` other than `"approve"` or `"block"` are [not counted](#assumed-results), since the docs do not say what Devin CLI does with them. A `PreToolUse` reply with `hookSpecificOutput.updatedInput` gets a note, since Devin CLI merges it into the tool's arguments. A `context` line marks `hookSpecificOutput.additionalContext` on `UserPromptSubmit`, `SessionStart`, and `PostToolUse`. Both count only when `hookSpecificOutput.hookEventName` names the event that ran.
- **Augment.** It blocks on exit 2 on `PreToolUse` only. It also blocks on exit 0 with `permissionDecision: "deny"`, a `decision: "block"` (inside `hookSpecificOutput` on `Stop` and `PostToolUse`), or `"continue": false`.
{% </details> %}

The run exits 1 when a counted command times out or errors, when two counted targets decide differently, or, with `--expect allow` or `--expect block`, when a counted target decides otherwise. That makes it a CI check.

### Assumed results {#assumed-results}

Cursor, Copilot, Factory, Qoder, Antigravity, Kiro, and Windsurf (Devin CLI) document their payloads and reply rules, but each leaves out part of how a command runs. `hook run` runs them on stated assumptions instead of leaving them out:

- Shell (Cursor, Factory, Antigravity, Kiro, Windsurf, Copilot's `command` form, and Qoder without `shell`): `sh -c` on macOS and Linux, only for a script path with plain or single-quoted arguments, as sync writes `args` for Cursor, which every POSIX shell reads the same way. A command with shell syntax, such as a pipe, is listed as not run with "Cursor does not document its shell; use a script path", or the same for Copilot, Factory, Qoder, Antigravity, Kiro, or Devin CLI. Windows is not run. Copilot's and Qoder's exec form, which sync writes when the spec sets `args`, runs with no shell and assumes none. Qoder documents `shell: bash` as `bash -c`, which assumes none on macOS and Linux.
- Timeout (Cursor, Windsurf): 30 seconds when the spec sets none. Set `timeout` in the spec to remove this assumption. Copilot and Antigravity document their 30 second default, and Factory and Kiro 60 seconds.
- Timeout (Kiro): 60 seconds for `timeout: 0`, which Kiro documents as no limit.
- Working directory (Copilot): the project root, for a hook without `cwd`. Set `cwd` in the spec to remove this assumption; it runs relative to the project root, as Copilot documents.
- Working directory (Factory): the project root. Factory runs hooks from "Droid's current working directory, which can differ from your repository root". A command written as the guide says, `"$FACTORY_PROJECT_DIR"/path/to/script.sh`, counts as a script path. It runs as written, with `FACTORY_PROJECT_DIR` set to the project root, so the shell expands it.
- Working directory (Qoder): the project root. Qoder documents `QODER_PROJECT_DIR` as the project's directory but not where a hook runs. A command written as the guide says, `"${QODER_PROJECT_DIR}"/path/to/script.sh`, counts as a script path and runs as written, with `QODER_PROJECT_DIR` set to the project root.
- Working directory (Antigravity): the project root. The docs do not say where a hook command runs.
- Working directory (Windsurf): the project root, with `DEVIN_PROJECT_DIR` set to it as Devin CLI documents. The docs do not say where a hook command runs.
- Exit codes (Antigravity): the docs give no exit code a meaning. Only exit 0 with a reply of the documented shape is counted. Any other result is shown with a `not counted` note and its reason, such as "Antigravity does not document exit codes", and stays out of `--expect` and the comparison even with `--include-assumed`. So do the `"ask"` and `"deny_unless_prior_grant"` replies, whose result "depends on Antigravity's saved permissions".
- Exit codes (Kiro): exit 2 blocks `PreToolUse` and `UserPromptSubmit`. Kiro's hook actions page says any other non-zero exit blocks them too, while its hook types, IDE 1.0, and troubleshooting pages say it does not. That result reads as `error` with a `not counted` note, "Kiro's docs disagree on whether a non-zero exit other than 2 blocks", and stays out of `--expect` and the comparison even with `--include-assumed`.
- Not run (Kiro): an `agent` action, which runs no command; a hook with `confirm`, which asks the user first; and every trigger but `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, and `Stop`, whose payload the docs do not show.
- Replies (Windsurf): results the Devin CLI docs give no meaning are shown with a `not counted` note, such as "Devin CLI does not document whether it reads a reply on a non-zero exit", and stay out of `--expect` and the comparison even with `--include-assumed`.
- Exec path (Copilot): with `cwd` set, a relative exec path resolves from `cwd`, as a process started there reads it and as sync writes it.

Sources: [Cursor hooks](https://cursor.com/docs/hooks), the [Copilot hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference), [using hooks with Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks) for the `bash` tool's `toolArgs`, the [Factory hooks guide](https://docs.factory.com/cli/configuration/hooks-guide), Qoder CLI [hooks](https://docs.qoder.com/cli/hooks) and its [hooks reference](https://docs.qoder.com/cli/hooks-reference), [Antigravity hooks](https://antigravity.google/docs/hooks), Kiro [hooks](https://kiro.dev/docs/hooks), [hook types](https://kiro.dev/docs/hooks/types), [hook actions](https://kiro.dev/docs/hooks/actions), and Devin CLI [hooks](https://docs.devin.ai/cli/extensibility/hooks) and [lifecycle hooks](https://docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks).

The result line ends in `(assumed: shell, timeout)`, followed by one line per assumption and the docs link. An assumed result is shown but not counted: it stays out of `--expect` and the comparison unless you pass `--include-assumed`. When it disagrees with the counted results, a warning says so, and a summary such as `0 checked, 1 assumed (not counted; --include-assumed to count)` shows what was left out. With `--expect`, a run where only assumed results ran fails and asks for `--include-assumed`, so a CI check never passes on nothing. A result with a `not counted` note stays out of `--expect`, but with `--include-assumed` a timeout or error from another of its commands, one the target documents, still fails the run. Without `--expect`, it exits 0. In JSON, each target has `assumptions` (`item`, `value`, `reason`) and `counted`.

A Cursor hook spec uses Cursor's own event names, so it runs only on Cursor; other targets are listed as not run. A Copilot hook spec with a PascalCase event such as `PreToolUse` runs on Claude Code too, which counts, so `--expect` checks Claude Code and shows Copilot beside it.

Cline's contract comes from its source at [`39ff235`](https://github.com/cline/cline/tree/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks), from the SDK hook runtime the Cline CLI runs. A Cline run assumes only the working directory: the project root, while Cline uses the directory the CLI started in. The VS Code extension does not run `.cline/hooks` scripts.

Crush's contract comes from its source, so a Crush run assumes nothing until a command reaches one of Crush's own Go programs. Its shell runs `jq` as a built-in [gojq](https://github.com/itchyny/gojq), and on Windows `cat`, `ls`, `rm`, `find`, and the other [core utilities](https://github.com/mvdan/sh/blob/b5028a3332a4d5d5a6cc62a999c1ba7993eb3627/x/coreutils/coreutils.go) in Go too, unless `CRUSH_CORE_UTILS=false`. `hook run` runs the ones on `PATH` instead, and adds a `jq` or `coreutils` assumption for each it reached. A script that starts through a shebang runs its own shell, so this applies only to inline commands and to `./` scripts without a shebang.

`--format json` prints the same results as one JSON object, for a CI job to read. Each target has a `decision` (`allow`, `block`, `error`, `timeout`, or `not run` with a `reason`), its `warnings`, its `assumptions`, whether it is `counted`, and one entry per command. `exit_code` is `null` after a timeout or a command that did not start. `error` holds the reason the run fails, and the exit code is the same as in text:

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

| Target | Payload | Vendor docs |
|---|---|---|
| Claude Code | Documented shape. `--edit` calls the first of `Write`, `Edit`, and `MultiEdit` the matcher matches, with an absolute `tool_input.file_path`. | [Hooks reference](https://code.claude.com/docs/en/hooks) |
| Codex | Documented shape. `--edit` calls `apply_patch` with an `*** Add File:` or `*** Update File:` patch in `tool_input.command`, and `Edit` and `Write` match it. | [Hooks](https://learn.chatgpt.com/docs/hooks) |
| Gemini | Shape from the source. `--edit` calls the first of `write_file` and `replace` the matcher matches, with an absolute `tool_input.file_path`; `--bash` calls `run_shell_command`. The tool matcher is an unanchored regular expression, and a `SessionStart` source must match exactly. | [Hooks reference](https://geminicli.com/docs/hooks/reference/), gemini-cli [`c6bccb7`](https://github.com/google-gemini/gemini-cli/tree/c6bccb7ecbf6d8368d995455dd725ed34466faad/packages/core/src/hooks) |
| Trae | Documented shape. `--bash` calls `RunCommand`; `llm_tool_name` repeats the tool name. Trae lists no edit tool input, so with `--edit` Trae is listed as not run and the other targets still run. The matcher is an unanchored regular expression. | [Hook reference](https://docs.trae.cn/ide_hook-configuration-reference), [automate actions with hooks](https://docs.trae.cn/ide_automate-actions-with-hooks) |
| OpenHands | Documented `HookEvent`: `event_type`, `tool_name`, `tool_input`, `message`, `session_id`, `working_dir`. `--bash` calls `terminal`; the file editor's input is undocumented, so with `--edit` OpenHands is listed as not run and the other targets still run. A `*` or empty matcher matches all; one with a regex character, or written `/re/`, must match the whole tool name; any other is an exact name. A matcher on an event with no tool never runs. | [Hooks](https://docs.openhands.dev/openhands/usage/customization/hooks), software-agent-sdk [`fad6377`](https://github.com/OpenHands/software-agent-sdk/tree/fad63774459171b08f889b12fd3b4d6346168c3e/openhands-sdk/openhands/sdk/hooks) |
| Goose | Documented shape: `event`, `session_id`, `matcher_context`, `tool_name`, `tool_input`, `working_dir`, `tool_call_id`. `--bash` calls `shell`; `--edit` calls the first of `write` and `edit` the matcher matches, with an absolute `path`. `BeforeShellExecution` and `AfterShellExecution` take `--bash`, and `AfterFileEdit` and `BeforeReadFile` take `--edit`. The matcher is an unanchored regular expression on `matcher_context`; one that does not compile, such as `*`, skips the rule. | [Hooks](https://goose-docs.ai/docs/guides/context-engineering/hooks/), goose [`bab8ff6`](https://github.com/aaif-goose/goose/blob/bab8ff641039c9cd3331121cd84a5c6045f365ca/documentation/docs/guides/context-engineering/hooks.md) docs and [runner](https://github.com/aaif-goose/goose/blob/bab8ff641039c9cd3331121cd84a5c6045f365ca/crates/goose/src/hooks/mod.rs) |
| Cursor | Documented shape: the common fields (`conversation_id`, `hook_event_name`, `workspace_roots`, ...) plus the event's own. `--bash` builds `beforeShellExecution` and `afterShellExecution` (`command`, `cwd`), and `preToolUse` and `postToolUse` on the `Shell` tool. `--edit` builds `afterFileEdit` only, since the `Write` tool's input is undocumented; on `preToolUse` and `postToolUse` Cursor is listed as not run and the other targets still run. `--prompt` builds `beforeSubmitPrompt`. The matcher is an unanchored regular expression, tested against the command on the shell events, the tool type on the tool events, and a fixed name on the rest. | [Hooks](https://cursor.com/docs/hooks) |
| Crush | Shape from the source: `event`, `session_id`, `cwd`, `tool_name`, `tool_input`, with `event` always `PreToolUse`, the only event Crush runs. `--bash` calls `bash`; `--edit` calls the first of `edit` and `write` the matcher matches, with an absolute `file_path`. Crush has no prompt event. The matcher is an unanchored regular expression on the lowercase tool name; one that does not compile makes Crush skip the hook, so `hook run` reports it. | crush [`76cc5c5`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal): [`hooks/runner.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/hooks/runner.go), [`hooks/input.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/hooks/input.go), [`shell/run.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/shell/run.go), [`shell/dispatch.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/shell/dispatch.go), [`config/config.go`](https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/config/config.go) |
| Factory | Documented shape: `session_id`, `transcript_path`, `cwd`, `permission_mode`, `hook_event_name`, plus the event's own. `--bash` calls `Execute` with `tool_input.command`. `--edit` calls `Create` with `file_path` and `content`; the docs show no input for `Edit` and `ApplyPatch`, so when the matcher picks one of them Factory is listed as not run and the other targets still run. `--prompt` builds `UserPromptSubmit` with `has_images`. Empty or `*` matches everything, letters and `\|` list exact names, and anything else is a case-sensitive regular expression. | [Hooks guide](https://docs.factory.com/cli/configuration/hooks-guide) |
| Antigravity | Documented camelCase shape: `conversationId`, `workspacePaths`, `transcriptPath`, `artifactDirectoryPath`, `modelName`, `stepIdx`, and `toolCall` with `name` and `args`; `PostToolUse` adds `error`. `--bash` calls `run_command` with `CommandLine`, `Cwd`, and `WaitMsBeforeAsync`. `--edit` calls the first of `write_to_file`, `replace_file_content`, and `multi_replace_file_content` the matcher matches, with an absolute `TargetFile` and the other arguments the docs list. Antigravity has no prompt event; `PreInvocation`, `PostInvocation`, and `Stop` take `--payload`. The matcher is a regular expression matched against the whole tool name; empty or `*` matches everything. | [Hooks](https://antigravity.google/docs/hooks) |
| Copilot | Documented shape, camelCase for a camelCase event and the VS Code compatible snake_case for a PascalCase one. `--bash` builds `preToolUse` and `postToolUse` on `bash`, with `toolArgs` as a JSON string, and `PreToolUse` on `Bash`, with a `tool_input` object. `--prompt` builds `userPromptSubmitted` or `UserPromptSubmit`. `--edit` is refused, since `toolArgs` for `edit`, `create`, and `apply_patch` are undocumented, and so is `--bash` on `PostToolUse`, whose `tool_name` is undocumented; Copilot is listed as not run and the other targets still run. A camelCase matcher is a regex anchored as `^(?:PATTERN)$`. Copilot runs JavaScript regexes, so a matcher with inline flags, lookaround, backreferences, `(?P<name>)`, POSIX classes, or Go-only escapes such as `\A` is listed as not run. | [Hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference), [using hooks](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks) |
| Qoder | Documented shape: `session_id`, `transcript_path`, `cwd`, `hook_event_name`, `permission_mode`, plus the event's own. `--bash` calls `Bash` with `tool_input.command`. `--edit` calls `Write` with `file_path` and `content`; the docs show no input for `Edit`, so when the matcher picks it Qoder is listed as not run and the other targets still run. `SessionStart` sources include `new`. Empty or `*` matches everything, letters and `\|` list exact names, and anything else is a regular expression. | [Hooks](https://docs.qoder.com/cli/hooks), [hooks reference](https://docs.qoder.com/cli/hooks-reference) |
| Cline | Shape from the source: `clineVersion`, `timestamp`, `taskId`, `sessionContext`, `workspaceRoots`, `userId`, `agent_id`, `parent_agent_id`, and `hookName` (`tool_call`, `tool_result`, or `prompt_submit`). `--bash` calls `run_commands` with `{"commands": [...]}`; `--edit` calls `editor` with an absolute `path` and an empty `new_text` (a model whose id holds `gpt` or `codex` calls `apply_patch` instead, which a note says). `preToolUse` and `postToolUse` repeat the input as `parameters`, a string map. `--prompt` builds `UserPromptSubmit`, with a note that the CLI may not send that event, since its source says orchestrated sessions seed the prompt without it. Cline has no matcher, so every hook fires. `PreCompact` is listed as not run. | cline [`39ff235`](https://github.com/cline/cline/tree/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks): [`hook-file-hooks.ts`](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks/hook-file-hooks.ts#L234-L254), [`subprocess-runner.ts`](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks/subprocess-runner.ts#L68-L97), [tool schemas](https://github.com/cline/cline/blob/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/extensions/tools/schemas.ts#L149-L224) |
| Kiro | Documented shape: `hook_event_name` in camelCase, `cwd`, and `session_id`, plus `prompt` on `userPromptSubmit` and an empty `assistant_response` on `stop`. `--prompt` builds `UserPromptSubmit`, and `Stop` needs no input. The docs show `tool_input` only for the read tool and MCP tools, so `--bash` and `--edit` are refused and `PreToolUse` and `PostToolUse` take `--payload`; Kiro is listed as not run and the other targets still run. The matcher is an unanchored regular expression on the tool name or its alias, and on the prompt text for `UserPromptSubmit`; `Stop` ignores it. `@mcp`, `@powers`, and `@builtin` are listed as not run, since the docs do not list their tools. | [Hooks](https://kiro.dev/docs/hooks), [hook types](https://kiro.dev/docs/hooks/types) |
| Windsurf | Documented Devin CLI shape: `hook_event_name`, `session_id`, `prompt_id`, plus the event's own. `--bash` builds `PreToolUse`, `PostToolUse`, and `PermissionRequest` on `exec` with `tool_input.command` and `shell_id`, and `PostToolUse` adds `tool_response`. `--prompt` builds `UserPromptSubmit`; `Stop` and `PostCompaction` need no input. `--edit` is refused, since the docs name `edit`, `write`, and `apply_patch` but not their `tool_input`; Windsurf is listed as not run and the other targets still run. `SessionStart` and `SessionEnd` list no `source` or `reason` values, so they take `--payload`. The matcher is an unanchored regular expression on `tool_name`, so a Claude-style `Bash` matcher does not fire; a matcher on another event is listed as not run. | [Hooks](https://docs.devin.ai/cli/extensibility/hooks), [lifecycle hooks](https://docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks) |
| Augment | Documented shape. `--bash` calls `launch-process`; `--edit` calls the first of `str-replace-editor` and `save-file` the matcher matches, with a `path` relative to the workspace root, and `PostToolUse` adds `file_changes`. Augment has no prompt event. The matcher is an unanchored regular expression. | [Hooks](https://docs.augmentcode.com/cli/hooks) |

On each, `tool_response` holds placeholder values, and session IDs and transcript paths are made up. Gemini matchers compile as Go regular expressions, which reject a few JavaScript forms such as lookahead; Gemini CLI would run those, and `hook run` compares them as a literal name. `GEMINI_PLANS_DIR` is not set.

OpenCode and Kilo run hooks as plugins and Zed as tasks, with no payload on stdin, so `hook run` lists them as not run.
