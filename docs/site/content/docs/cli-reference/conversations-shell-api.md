+++
title = "Conversations shell API"
description = "Fields for forgotten Claude Code background shells, and the local API routes that stop a shell or a whole Claude Code chat."
weight = 46

[extra]
group = "Reference"
+++

# Conversations shell API

## `stuckShells` {#stuck-shells}

In the [conversations response](@/docs/cli-reference/conversations-api.md#get-api-views-conversations), `stuckShells` lists the background shells that passed [`conversations.stuckShellAfter`](@/docs/global-configuration-reference.md#global-conversations) in a live local Claude Code session. Each entry is `{pid, procStart, command, since, helperId?, helperName?, processCount}`. Use `pid` and the exact `procStart` string to request a stop. `since` is an RFC 3339 time for the displayed age. The server first masks a password passed in a known way: `-p` or `--password` to `mysql` or `mariadb`, `sshpass -p`, the password in `curl -u user:password` or `--user`, and `PGPASSWORD=`, `MYSQL_PWD=` or `SSHPASS=` before the command, as in `mysql -p<redacted>`. It withholds the command (an empty string) when one of those tools gets a password it cannot mask, such as a value with a backslash or a tool started by another command, or when its credential check finds any other sign of a password or key, such as a known token format or a key assignment, and otherwise masks any credential in a URL, as in `https://<redacted>@host/`. `-p` stays as written for every other tool, where it is often a port. A password passed under a flag the server does not know may still show. The page never redacts on its own. A command is sent on one line, cut at 2,000 characters. `helperId` and `helperName` identify a matched subagent when available. `processCount` counts the processes in the shell's group at scan time.

## `POST /api/conversations/stop-shell` {#post-api-conversations-stop-shell}

Send `{tool, id, pid, procStart}` as JSON from the server's own origin. Use the tool and ID of a live local Claude Code conversation and the process identity from its `stuckShells` entry.

On macOS and Linux, the server scans again before it signals the shell's process group. It checks the live session's process identity, the shell's parent, the shell's start time, and that the shell leads a group separate from the session's. It sends SIGTERM and, if the group still needs to stop after five seconds and its identity still checks out, SIGKILL. Processes that left the group are reported if still running.

| Status | Meaning |
|--------|---------|
| 200 | The stop request completed. The body is `{stillRunning?: [{pid, procStart}]}`. |
| 400 | The request body or a field is invalid. |
| 409 | The server refused the target, for example because the session closed or the process identity changed. |
| 500 | The server could not complete the stop. |

## `stoppable` {#stoppable}

In the [conversations response](@/docs/cli-reference/conversations-api.md#get-api-views-conversations), `stoppable` is `{pid, procStart}`, the process of a live Claude Code session that runs in a terminal on this computer, on macOS and Linux. Background, editor and Claude app sessions are not stoppable here: the program that owns them may start them again.

## `POST /api/conversations/stop-session` {#post-api-conversations-stop-session}

Send `{tool, id, pid, procStart}` as JSON from the server's own origin, with the process from the row's `stoppable`. If the chat has moved to another process, the server asks for a fresh confirmation.

On macOS and Linux, the server scans again and checks that the session still runs in a terminal, that its process has the start time its session file names, and that the process is not this server or one of its parents. It sends SIGTERM to that process alone. After five seconds, it sends SIGKILL only if the same process is still in a terminal and is safe to stop. The transcript stays, so the chat can be resumed; a forced stop can lose the line Claude Code was writing. Shells the session started in their own process groups get no signal from the server and may keep running.

| Status | Meaning |
|--------|---------|
| 200 | The stop request completed. The body is `{forced?, stillRunning?}`: `forced` when the server sent SIGKILL, `stillRunning` when the process is still running. |
| 400 | The request body or a field is invalid. |
| 409 | The server refused, for example because the session no longer runs in a terminal here, its process changed, or this server is already stopping it. |
| 500 | The server could not complete the stop. |
