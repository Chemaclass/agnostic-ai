+++
title = "Conversations shell API"
description = "Fields for forgotten Claude Code background shells, and the local API routes that stop a shell or a whole Claude Code chat."
weight = 46

[extra]
group = "Reference"
+++

# Conversations shell API

## `stuckShells` {#stuck-shells}

In the [conversations response](@/docs/cli-reference/conversations-api.md#get-api-views-conversations), `stuckShells` lists the background shells that passed [`conversations.stuckShellAfter`](@/docs/global-configuration-reference.md#global-conversations) in a live local Claude Code session. Each entry is `{pid, procStart, command, since, helperId?, helperName?, processCount}`. Use `pid` and the exact `procStart` string to request a stop. `since` is an RFC 3339 time for the displayed age. The server filters the command for credentials. `helperId` and `helperName` identify a matched subagent when available. `processCount` counts the processes in the shell's group at scan time.

## `POST /api/conversations/stop-shell` {#post-api-conversations-stop-shell}

Send `{tool, id, pid, procStart}` as JSON from the server's own origin. Use the tool and ID of a live local Claude Code conversation and the process identity from its `stuckShells` entry.

On macOS and Linux, the server scans again before it signals the shell's process group. It checks the live session's process identity, the shell's parent, the shell's start time, and that the shell leads a group separate from the session's. It sends SIGTERM and, if the group still needs to stop after five seconds and its identity still checks out, SIGKILL. Processes that left the group are reported if still running.

| Status | Meaning |
|--------|---------|
| 200 | The stop request completed. The body is `{stillRunning?: [{pid, procStart}]}`. |
| 400 | The request body or a field is invalid. |
| 409 | The server refused the target, for example because the session closed or the process identity changed. |
| 500 | The server could not complete the stop. |

## `POST /api/conversations/stop-session` {#post-api-conversations-stop-session}

Send `{tool, id}` as JSON from the server's own origin. Use a row whose `stoppable` is `true`.

On macOS and Linux, the server reads Claude Code's session files again and checks that the session's process still runs, with the start time its session file names. It sends SIGTERM to that process alone. If the same process still runs after five seconds, it sends SIGKILL. The transcript stays, so the chat can be resumed. Shells the session started in their own process groups get no signal from the server and may keep running.

| Status | Meaning |
|--------|---------|
| 200 | The stop request completed. The body is `{forced?, stillRunning?}`: `forced` when the process ignored SIGTERM and got SIGKILL, `stillRunning` when even that did not end it. |
| 400 | The request body or a field is invalid. |
| 409 | The server refused, for example because the session is not live here, its process changed, or a stop of it is already running. |
| 500 | The server could not complete the stop. |
