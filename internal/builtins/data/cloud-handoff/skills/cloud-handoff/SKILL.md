---
name: cloud-handoff
description: Push the current task to a branch and continue it in a cloud session. Use when asked to "send this to a remote agent", to "run it in the cloud", or when the user is "closing my laptop".
---

# cloud-handoff

Send the current task to a cloud session that goes on without this computer. The cloud session starts from a fresh clone: it sees no uncommitted work and no chat history. So push the work first, then write down the goal, then launch.

## When

- Only when the user asks, or agrees after you offer. Never at session start, at session end, or on your own: it pushes code and starts paid work in the cloud.
- To pass the task to the next local agent instead, with no push and no network call, use the `handoff` skill if it is installed.

## Preflight

Check each of these before you change anything. If one fails, stop and say which one and what the user can do.

1. `git rev-parse --is-inside-work-tree` prints `true`. Otherwise there is nothing to push.
2. `git remote get-url origin` prints a URL. Without a remote the cloud cannot clone the work.
3. `command -v` finds the launch command of the row you will use in the Launch table below.
4. The row's other needs are met: a terminal wrapper for the cloud launch, or an environment id.
5. The push and the launch need network access. In a sandbox without it, ask the user to allow it, or give them the lines to run, and stop.

## Branch, commit, push

1. Never push to the default branch (the one `git symbolic-ref --short refs/remotes/origin/HEAD` names, else `main` or `master`). On the default branch or a detached HEAD, create a new branch first, such as `cloud/<short-task-slug>`, and name it in your reply.
2. Show `git status --short`. Stage only the files of this task, by path. Never stage a `.env` file, a key or certificate file, or anything else that looks like a secret; leave it out and say so.
3. Commit with a short message that names the task. If the commit fails, for example a hook or signing fails, stop and show its output. Do not skip the hook. Do not change Git settings, such as signing, to get the commit through.
4. `git push -u origin <branch>`. If it fails, stop and show the error. Never force-push.

## Write the prompt

The prompt is all the cloud session knows. Write it like this:

```markdown
<The goal, as one plain sentence.> [<branch>]

## Done
## Next step
## Branch
## Verify
```

- First line: the goal in one plain sentence, with the branch name at the end in brackets. Some tools use it to make the session title.
- **Done**: what this session finished, with paths relative to the project root.
- **Next step**: the first thing to do, then what remains.
- **Branch**: the pushed branch, and that the work is committed on it.
- **Verify**: the checks to run before calling it done.
- Keep it under 60 lines. Never write secrets, tokens, or `.env` values.
- Write it to a file through a quoted heredoc, so the shell expands nothing in it, then read the file into a variable. Keep the heredoc outside `$( )`: an older bash, such as macOS `/bin/bash` and `/bin/sh`, breaks on quotes and parentheses there. Set it and launch in one shell command: a variable does not survive to the next tool call, and the launch refuses an empty prompt. The file stays in the temp folder, which the system clears.

```bash
f=$(mktemp)
cat >"$f" <<'PROMPT'
Finish the parser error messages. [cloud/parser-errors]

## Done
...
PROMPT
CLOUD_PROMPT=$(cat "$f")
export CLOUD_PROMPT
[ -n "$CLOUD_PROMPT" ] || exit 1
```

## Launch

Use the row for the tool this session runs in. Use another row only when the user names that tool.

| Tool | Launch | Needs |
|---|---|---|
| Claude Code | `claude --cloud "$CLOUD_PROMPT"`, wrapped in a terminal as shown below | A claude.ai sign-in, and `script` on macOS or Linux. `--environment <id>` only for a self-hosted environment. |
| Codex | `codex cloud exec --env "<id>" --branch "<branch>" -`, with the prompt on standard input as shown below | An environment id. The subcommand is marked experimental. |
| Any other tool | none | Falls back to a local handoff note. |

### Claude Code

`claude --cloud` refuses to run without a terminal (`Error: --cloud requires an interactive terminal.`), and a shell tool has none. Give it one with `script`, write its output to a file, wait for the `View: ` line, then kill it: after printing that line it keeps running. Feed it from `sleep`, so its input stays open and sends nothing; `</dev/null` would send it an end of input at once.

On macOS, after the prompt lines above:

```bash
log=$(mktemp)
sleep 150 | script -q /dev/null claude --cloud "$CLOUD_PROMPT" >"$log" 2>&1 &
pid=$!
for _ in $(seq 60); do grep -qE 'View: |Error' "$log" && break; sleep 2; done
sleep 2; kill "$pid" 2>/dev/null; pkill -P $$ -f '^sleep 150$'
cat "$log"
```

Run these lines at the top level of the shell command, not inside `( )`: the last line stops the input `sleep` through this shell's process id.

On Linux, start it with `sleep 150 | script -q -c 'claude --cloud "$CLOUD_PROMPT"' /dev/null >"$log" 2>&1 &` instead; the rest is the same. It reads the exported variable.

On success the output holds three lines: `Created cloud session: <title>`, `View: https://claude.ai/code/session_<id>?...`, and `Resume with: claude --teleport session_<id>`. The output may also hold terminal escape codes; read past them.

Without `script`, such as on Windows, do not launch. Show the user the prompt and the line `claude --cloud "<prompt>"` to run in their own terminal from this checkout, and stop.

### Codex

The command reads the prompt from standard input when the prompt argument is `-`, and needs no terminal. After the prompt lines above:

```bash
printf '%s\n' "$CLOUD_PROMPT" | codex cloud exec --env "<id>" --branch "<branch>" -
```

It prints one line on success: the task URL, `https://chatgpt.com/codex/tasks/<task_id>`.

The environment id is not stored anywhere this skill can read. If the user has not given one in this session, ask the user for it and stop. Tell them they can find or create it at chatgpt.com/codex/settings/environments, or browse it with `codex cloud` in their own terminal.

## After

- Start your last reply with one line that holds the session URL or id from the output, such as "Sent to the cloud: <url>". The user finds the cloud session from this line.
- Say that the cloud session starts from the pushed branch. A Claude Code session puts its work on a new branch of its own, so the user's branch stays theirs; a Codex task returns its work as a diff to apply.
- Name the command that brings the work back: `claude --teleport session_<id>`, or `codex cloud diff <url>` then `codex cloud apply <url>`.
- Say the session shows under Cloud in the companion within about a minute, when the companion lists cloud sessions.

## When it refuses

- Show the tool's message. Do not work around it: no other launch method, no retry with other flags.
- `Cloud sessions need a claude.ai sign-in.` or `Not signed in.`: the user signs in (`claude auth login` or `codex login`) and asks again.
- `no cloud environments are available for this workspace`: the user has no Codex environment yet; point to chatgpt.com/codex/settings/environments.
- ``environment '<id>' not found; run `codex cloud` to list available environments``: the id is wrong; ask the user for it and stop.
- A line starting `Branch <name> is not on GitHub`: the session started from older work. Say so; the push did not reach the remote the cloud reads.
- No row for this tool: say there is no cloud launch for it. Offer a local handoff note through the `handoff` skill if it is installed.
