+++
title = "Conversations delete API"
description = "The local API route that deletes an archived Claude Code session, and which files it removes."
weight = 46

[extra]
group = "Reference"
+++

# Conversations delete API

## `POST /api/conversations/delete` {#post-api-conversations-delete}

Body: `{tool, id}` with `Content-Type: application/json`, from the server's own origin.

Permanently delete a local Claude Code session under Archived from this machine. The request must meet every condition below.

- Its `state` is `completed` or `dormant`, read from a fresh scan.
- No Claude Code process or background job holds it. The server asks again right before it removes anything, and once more before the transcript.
- None of its files was written at or after the moment it was archived. For `completed` that is your archive mark: archiving it again makes a new mark. For `dormant` it is the moment the server read the row for this request, so a write in that second or later is refused.
- Its session id names exactly one transcript.

{% <details summary="What delete removes, what stays, status codes and limits"> %}

What it removes, each only when its name is exactly the session id:

- `projects/<project>/<id>/`: subagent transcripts, saved tool output and the chosen title.
- `file-history/<id>/`: file backups for undo.
- `session-env/<id>/`: hook environment.
- `uploads/<id>/`: images pasted into the session.
- `projects/<project>/<id>.jsonl`: the transcript, last, so a failure leaves the row listed and the delete can be retried.
- Its marks and its remembered terminal tab.

What stays:

- `history.jsonl`: Claude Code's prompt history, one file shared by every session.
- `projects/<project>/memory/`: project memory, shared by every session of the project.
- `tasks/session-<prefix>/` and `telemetry/` files: named after part of the id, or after more than the id, so the server cannot prove they belong to this session alone.

A location that is a link, or that holds a link, is refused, since the data it points to would stay.

Accepted limits:

- A session that starts in the moment between the last check and the removal is not seen.
- A forked session may still point at saved tool output under its parent's `projects/<project>/<parent id>/tool-results/`. Deleting the parent removes that output.

| Status | Meaning |
|--------|---------|
| 200 | `{deleted: true, freedBytes?, note?}`, sent only once the transcript is gone. `freedBytes` is what its files held. `note` says what could not be tidied after the data went, such as its entry in the marks file. |
| 400 | for an unknown tool or any other field. |
| 403 | for a request without the companion's secret or a tab's [session key](@/docs/approvals.md#who-can-change-this), or from another origin. Nothing is deleted. |
| 404 | for a conversation outside the list, or one you hid. |
| 409 | `{error, reason}` with `reason` `not-archived` (the row is outside Archived, or a saved reply time places it there and the server could not confirm closure), `running` (a process or job holds it) or `changed` (a file was written at or after the moment it was archived). |
| 422 | `{error, reason: "unsupported"}`: Codex threads (Codex deletes a thread even while it is open, so close it first and run `codex delete <id>` yourself), Cursor editor chats (Cursor keeps them all in one shared database), `cursor-agent` chats, cloud sessions. Also a session whose files the server cannot verify: its id is in two project folders, or its transcript or one of its locations is not a regular file or folder, such as a link. |
| 500 | `{error, reason: "failed", removed?, remaining?}`. Nothing was removed when the marks could not be locked or read, the files could not be measured, or the process list could not be read. When a removal began and then stopped, `removed` and `remaining` list what is gone and what is still on disk: a removal failed, or the session started running before its transcript went. The marks are kept. |

`error` is always a complete sentence.
{% </details> %}
