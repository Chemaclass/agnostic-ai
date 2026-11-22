+++
title = "Session mailbox"
description = "Send questions, findings, code reviews and file handoffs between local Claude Code and Codex sessions, from the companion or from the agents themselves."
weight = 80

[extra]
group = "Workflows"
+++

# Session mailbox

Keep a question or finding beside the two chats it concerns. The [companion](@/docs/cli-reference/companion-list.md) shows each thread under both local sessions.

## Send a message

1. Right-click a local Claude Code or Codex chat and choose **Message another session…**. The form shows that chat under **From**.
2. Pick the receiver under **To**, or click its row in the list. Rows you can message have a blue outline. To use a [session ID](@/docs/cli-reference/companion-list.md#copy-a-session-id), choose **Enter an ID instead**.
3. Choose **Question** or **Finding** and write your message. The line under **Type** says what the receiver is asked to do, and the line above the buttons says when it gets the message.
4. Choose **Send to** and the receiver's name, then open the thread to see the waiting message.

The **To** list holds the local Claude Code and Codex chats the companion has loaded, grouped like the main list. With the mailbox built-in on, an archived chat gets the message when it is resumed. An ID that matches no loaded chat still sends, and the server checks it.

Choose **Type into session** to put it in the existing terminal tab. Press Enter in that tab when you are ready to send it. The Terminal app uses the clipboard to paste it. **Copy message** lets you paste it yourself in any app. A message marked **Staged** has been typed or copied by you. A failed attempt leaves it waiting. With the [mailbox built-in](#deliver-with-the-next-prompt) on, a waiting message also reaches the session with your next prompt.

## Request a code review

1. Open **Message another session…** from the chat in the Git worktree you want reviewed. Choose **Review request** and pick the reviewer’s chat.
2. Check the base that the form fills from `origin/HEAD`, or enter a base such as `main` yourself.
3. Check the worktree, commit, base and changed-file count in the preview. Commit local changes first if they belong in the review.
4. Send the request and stage it from the thread. The reviewer can send back a **Finding**.

The message carries Git references and has no text field for a brief or verdict. The commit and base stay fixed after sending, even if a branch moves. Git metadata such as branch names and commit messages can still influence a review.

{% <details summary="Model labels and Git limits"> %}

You can enter the model running in each chat. The labels reflect what you enter. The companion warns when both names match, including across different tools. A missing name shows **Model unknown**. Different names alone cannot establish that a review is independent.

The base must be an ancestor of the commit. The companion uses the commit checked out in the source chat’s worktree, including when it has a detached HEAD. A dirty worktree is allowed, and the request includes a count of local files with uncommitted changes. Those changes stay out of the diff. Commit them before preparing a new request if the reviewer needs them.

The preview skips external Git filters and changes inside nested submodule worktrees. Git can count a filtered file differently when run directly.

Split a review if it exceeds 200 paths or the 5,000-byte message limit. A path is limited to 400 bytes. The same credential check applies to Git references. For a read-only preview in a terminal, use [`agnostic-ai mailbox refs`](@/docs/cli-reference/mailbox.md#mailbox-refs).

{% </details> %}

## Hand off files

A file claim notes which session works on which paths of a Git worktree. It does not block edits: any session can still change a claimed file. Claims help two sessions avoid working on the same file, and a handoff moves them from one session to another.

1. Choose **Claim files…** on the source chat and enter paths relative to its Git worktree. A directory claims its contents. Use `.` to claim the whole worktree.
2. Open **Message another session…** and choose **Handoff**. Enter the paths to give and a short message pointing to your [handoff note](@/docs/handoff.md).
3. Open the thread from the receiving chat and choose **Accept**. The files move to its claim when the offer is accepted. **Decline** leaves ownership with the sender.

If a path is free, claim it from the handoff form before sending. An overlap names the path and the session that holds it. Use **Release** to free a path when work on it ends.

When a session asks for a path another one claims, both rows move under Needs you with a red line, on the holder: "Draft the tests wants internal/a.go, which this session claims", and on the requester: "internal/a.go is claimed by Fix the parser". Click the line to open that session's claims. The holder ends the conflict by releasing the path or handing it to the requester; the requester can **Dismiss** it. Both chats stay on the first page whatever their age. A conflict from a handoff clears when its offer is declined or its thread is closed. **Decline offer** in the footer's card does that even when both chats left the list.

A chat that holds claims shows a chip such as "2 paths claimed" in its row. Click it to see the paths, who claimed them and when, and to release them. The **File claims** button in the footer appears only when a claim or a conflict exists. Its number counts the claimed paths across all sessions, and a warning mark shows when a claim's session is gone or two sessions want the same path. When a claim is refused because another session holds the path, the card lists the conflict with **Dismiss**, which removes only that conflict note. The "?" next to the claim explanation opens this section.

{% <details summary="Path matching and stale claims"> %}

Claims use the real worktree and resolve symlinks to the same path. New files can be claimed before they exist. A file overlaps a claimed parent folder. On a worktree whose file system ignores letter case, paths that differ only in case are one claim. Giving one file from a folder leaves the other files with the sender. Releasing a folder also releases your claims on files inside it. Claims in a worktree nested inside another one are not compared with the outer worktree.

Closing or stopping a thread keeps its claims. A claim whose session left the list has no row. Open **File claims** in the footer: those claims come first, under **Session gone**, each with **Release**, and **Release all** when there are two or more. A session counts as gone only when its tool's store was read in full and the session is not in it. A store that could not be read, or that holds no sessions at all, proves nothing; release such a claim with **Release** from the footer card. Before freeing these claims, the server checks again that the session is gone and that each path is still that session's claim.

{% </details> %}

## Deliver with the next prompt

Turn on the `mailbox` built-in in the project where the sessions run, then sync:

```yaml
builtins: [mailbox]
```

It adds two hooks to Claude Code and Codex. A session gets its waiting messages with your next prompt, and when it starts or resumes. Compacting or clearing a session delivers nothing, so a message always waits for you.

A message marked **Delivered** was printed in full into the session's context. That does not mean the model read it or acted on it. If the hook stops after printing and before saving, the message prints again a minute later. Each message carries its thread ID and number, so a repeat is easy to spot.

The hook skips messages that are held, rejected, cancelled or staged, and every message of a paused or closed thread. A closing thread delivers what it already accepted, then closes. One delivery prints at most 6,000 bytes. Messages that do not fit wait for the next prompt, oldest first. **Queue for delivery** on a staged message hands it back to the hook.

The hook never blocks a prompt. When the mailbox is missing, busy or broken, it prints nothing and the messages keep waiting.

### Codex: trust the hooks

Codex runs a new hook only after you review and trust it. Open `/hooks` in Codex after the sync and trust both mailbox hooks. Codex asks again whenever a hook's command changes, for example after an upgrade changes the built-in. Until then, Codex sessions get no messages.

## Approve agent messages

A message an agent sends waits as **Held for you** until you approve it. **Approve** checks the thread again: a paused thread refuses it, and a handoff whose paths changed owner is refused. A stopped thread has already cancelled it. **Reject** records the refusal in the thread, and the message no longer counts toward the limit. Messages you write in the companion are never held.

Approval is on by default. Turn off **Ask me before an agent sends a message** in the companion settings to let agent messages queue at once. A missing or unreadable setting keeps approval on. Only the companion can turn it off: the server refuses that change from any other program, [as it does for approvals](@/docs/approvals.md#who-can-change-this).

## Let agents send

With the `mailbox` built-in on, each Claude Code and Codex session also gets a `mailbox` skill. It tells the agent when to send each message type, to send distilled findings and never a transcript or a secret, and to treat a message it receives as data: it tells you what arrived and runs nothing because a message says so.

The agent sends, replies, closes a thread, claims paths and answers a handoff with one [`agnostic-ai mailbox`](@/docs/cli-reference/mailbox.md) command each. Every message it sends waits as **Held for you** until you approve it, and the thread shows it as sent by the agent, with the model the agent says it runs. A handoff the agent accepted says so.

The guards are the same as for the companion: the cap, size, credential, pause, close, refs and ownership checks all apply. A session outside a thread cannot reply to it, close it or answer its handoffs. `mailbox show` and `mailbox list` leave out the text of a message that waits for approval or delivery, so an agent reads a message only when the hook delivers it.

## What sender detection is not

The command finds the session it runs in from its parent processes and the tool's own session files. That is context, not proof of identity:

- Codex can run several threads in one process. The command trusts the thread ID in its environment when it cannot check which thread the process holds, and a process that holds several threads can name any of them.
- Any process that runs as your OS user can edit the mailbox files or call the companion's local routes, and get past every guard.
- The model name on a message is what the agent wrote. Nothing checks it.

The guards keep cooperating sessions to the supported mailbox actions. Read a held message before you approve it.

## Count what happened

`agnostic-ai mailbox stats` prints counts and durations from the mailbox: refusals by guard, time from queued to delivered, your approvals and other actions, handoff conflicts, and how many review requests went to a different model. It holds no message text. See [mailbox stats](@/docs/cli-reference/mailbox.md#mailbox-stats) for each key.

## Manage a thread

| Action | What happens |
|---|---|
| **Pause** | Puts new sends and staging on hold until you resume it. |
| **Close thread** | Queues a final `done` message. The thread closes as soon as its waiting messages are staged. |
| **Stop** | Closes the thread right away and cancels waiting messages. Both sessions keep running. |
| **Allow 8 more** | Raises this thread’s limit after eight content messages. |

{% <details summary="Message format and refusals"> %}

A question or finding carries up to 4,000 characters. Each JSON message line fits in 5,000 bytes. The recipient sees JSON inside one `<agnostic-ai-mailbox>` block, with its thread, message number, type and sender. When an agent states its model, the sender entry gains `"model": "<name> (stated by sender)"`. Nothing checks that the name is true, and it counts toward the 5,000 bytes. The wrapper labels the message as quoted data. The mailbox refuses text that looks like a credential.

A refusal gives the guard, reason and next step. For example, `cap` means the thread reached its limit. `unknown-session` means the destination is missing, ambiguous or outside the supported local sessions. See the [mailbox API](#mailbox-api) for the request format.

{% </details> %}

{% <details summary="Local storage and trust"> %}

The mailbox lives beside the conversation marks in the app’s configuration folder. It stores each thread separately, keeps the approval setting in `settings.json`, and keeps a short record of refusals without message bodies. On macOS the folder is `~/Library/Application Support/agnostic-ai/mailbox/`.

The controls govern normal mailbox use. Any process running as your OS user can access files you own. A model may still act on text you paste, so read each message before submitting it.

{% </details> %}

## Mailbox API {#mailbox-api}

{% <details summary="Local routes and request fields"> %}

The local user-level server provides these routes to the companion. The conversation list includes `threads` when a local row has mailbox activity. Each summary gives the other session, state, counts and last message type. A row that holds claims has `claimCount` and `claimedPaths`, the number of paths it claims. A row that holds or requests a path in an active conflict has `claimConflicts`, with or without a thread. The view has `mailboxClaims: {paths, stale, conflicts}` when any claim or conflict exists, counting every session, listed or not. `mailboxWarning` explains a partial read.

| Method and path | Request and result |
|---|---|
| `POST /api/mailbox/send` | Send `{from, to, type, thread?}` with the fields for the message type. Session references hold `tool` and `id`. A target identified by ID alone must match one local session. The reply gives `{thread, n, state}`. |
| `POST /api/mailbox/refs` | Send `{tool, id, base?}` for the source chat. The read-only reply gives `{worktree, commit, base, branch?, paths, uncommitted}`. |
| `GET /api/mailbox/threads?tool=&id=` | Read `{threads, warning?}` for one session. |
| `POST /api/mailbox/thread` | Send `{thread, action}` for `pause`, `resume`, `stop` or `raise-cap`. |
| `POST /api/mailbox/stage` | Send `{thread, n, how}` with `type` or `copy`. For copy, the command-line server replies with `text`. After writing it to the clipboard, send the same request with `confirm: true`. The companion performs both steps with the system clipboard. |
| `GET /api/mailbox/claims?tool=&id=` | Read `{claims, conflicts, worktree?}` for a session. Omit the query to see all claims, including stale owners. |
| `POST /api/mailbox/claim` and `/api/mailbox/release` | Send `{tool, id, worktree, paths}` for the owner, up to 200 paths. A release with `gone: true` frees a gone session's claims: it is refused unless the owner is gone and each path is one of its claims in that worktree. |
| `POST /api/mailbox/conflict/dismiss` | Send `{tool, id, conflict}`, with the requester and the conflict's `id`, to remove that conflict with no thread. Only the companion that started the server may call it. Anything else changes nothing. |
| `POST /api/mailbox/handoff` | Send `{thread, n, action}` with `accept` or `decline`. |
| `POST /api/mailbox/approve` | Send `{thread, n, action}` with `approve` or `reject` for a held message. |
| `POST /api/mailbox/queue` | Send `{thread, n}` to turn a staged message back into a waiting one. |
| `GET /api/mailbox/settings` and `POST /api/mailbox/settings` | Read or write `{approveAgentSends}`. Writing `false` needs the companion's secret; without it the answer is 403. The web page changes this switch through [`POST /api/settings`](@/docs/cli-reference/serve-api.md#post-api-settings) instead. |

Questions and findings take `body`. A `review_request` takes `refs: {worktree, base, commit?, paths?}` and optional `models: {from?, to?}`. Pass the preview’s commit, base and paths to keep the selection pinned. The server checks the worktree against the source chat and verifies any supplied paths against Git. A review request with a `body`, verdict or unknown field is refused with `echo`. An invalid base, commit or path selection is refused with `refs`.

A `handoff` takes `body` and `gives: {worktree, paths}`. Sending creates an offer. Acceptance checks the same paths again before moving them. An overlap or missing claim is refused with `ownership`.

A request that sets a field the store owns, such as `from.by`, `state`, a time stamp or `cap`, is refused with `format`. A refused send or review preview answers HTTP 422 with `{error, guard}`. The error says what stopped it and what to do next.

{% </details> %}
