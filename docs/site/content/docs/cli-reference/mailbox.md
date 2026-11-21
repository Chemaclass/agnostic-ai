+++
title = "Mailbox commands"
description = "Send, reply, close threads and answer handoffs from inside an agent session, resolve the Git refs for a review request, and count what the mailbox recorded."
weight = 48

[extra]
group = "Reference"
+++

# Mailbox commands

An agent runs these commands from inside its own Claude Code or Codex session to use the [session mailbox](@/docs/mailbox.md#let-agents-send). The [`mailbox` built-in](@/docs/mailbox.md#deliver-with-the-next-prompt) ships a skill that tells it when and how.

Each command except `refs` detects the session it runs in and acts as that session. There is no flag or field to pick another sender. Detection is context, not proof of identity: see [what sender detection is not](@/docs/mailbox.md#what-sender-detection-is-not).

A command only checks a message and stores it. It never types into a terminal, uses the clipboard, moves focus or starts a process. The reader's [prompt hook](@/docs/mailbox.md#deliver-with-the-next-prompt) delivers it.

Every command exits 0 when it sent, queued, held or read, and 1 otherwise. A refusal prints the guard, the reason and the next step on standard error, the same text the companion shows:

```text
Not sent (guard: format). A message takes one JSON object with to or thread, type, body, refs, gives and model, each once.
Remove the other fields. The mailbox sets the sender, state, number and times.
```

Outside a session, a command stops before it touches the mailbox:

```text
mailbox commands run inside a Claude Code or Codex session: cannot tell which conversation this runs in: no Claude Code, Codex or Cursor terminal agent session is among its parent processes; run it from inside a session
```

## mailbox send

```text
agnostic-ai mailbox send < message.json
```

Reads one JSON object on standard input. It takes these keys, each at most once:

| Key | Value |
|---|---|
| `to` | The reader as `tool:id`, such as `claude:4e8b1c2d-9a3f-4b6e-8c7d-1f2a3b4c5d6e`. Starts a thread. |
| `thread` | A thread this session is in. Replies to the other session. Use `to` or `thread`, not both. |
| `type` | `question`, `finding`, `review_request`, `handoff` or `done`. |
| `body` | The text of a question, finding or handoff, up to 4,000 characters. |
| `refs` | The output of [`mailbox refs`](#mailbox-refs), for a review request. |
| `gives` | `{worktree, paths}` for a handoff. |
| `model` | The model the agent says it runs: up to 80 letters, digits, spaces and `. _ : / -`. The thread stores it as stated by the agent. |

Any other key, such as `from`, `by`, `state`, `n`, a time stamp, `cap` or `events`, is refused with `format`. A review request with a `body` or an unknown `refs` key is refused with `echo`. The store checks the refs again, and drops the `branch` and `uncommitted` values it computes itself. Run a review request or a handoff from inside the worktree it names.

```bash
agnostic-ai mailbox send <<'MSG'
{"to": "claude:11111111-1111-4111-8111-111111111111", "type": "question", "body": "Does the store lock cover the claims file too?", "model": "gpt-5.5"}
MSG
```

```text
Held for approval in the companion. Thread th_20261121_033e736b, message 1.
```

With [approval](@/docs/mailbox.md#approve-agent-messages) off, the line reads `Queued for delivery with the reader's next prompt.` instead.

## mailbox done

```text
agnostic-ai mailbox done --thread <id>
```

Sends a `done` message. Once approved, the thread moves to closing and closes after its accepted messages are delivered. A second `done` while one waits for approval is refused with `closed`.

```text
Held for approval in the companion. Thread th_20261121_033e736b, message 2. The thread closes after it is approved.
```

## mailbox list and show

```text
agnostic-ai mailbox list
agnostic-ai mailbox show <thread>
```

Print the threads of this session, or one of them, as JSON. Each message shows its number, type, sender, reader, state and time. Its `body`, `refs` and `gives` show only when this session sent it or it was already delivered or staged. A model a peer agent states shows only from such a message. A message that waits for approval or delivery stays unread until the hook delivers it. `show` on a thread this session is not in is refused with `not-in-thread`.

This `list` runs in the reader's session while the question above is held:

```json
{
  "threads": [
    {
      "id": "th_20261121_033e736b",
      "status": "open",
      "cap": 8,
      "count": 1,
      "between": [
        { "tool": "codex", "id": "0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50" },
        { "tool": "claude", "id": "11111111-1111-4111-8111-111111111111" }
      ],
      "messages": [
        {
          "n": 1,
          "type": "question",
          "from": { "tool": "codex", "id": "0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50", "by": "agent" },
          "to": { "tool": "claude", "id": "11111111-1111-4111-8111-111111111111" },
          "state": "held",
          "at": "2026-11-21T20:10:53.744349Z"
        }
      ]
    }
  ]
}
```

## mailbox claim and release

```text
agnostic-ai mailbox claim [--worktree <dir>] <path>...
agnostic-ai mailbox release [--worktree <dir>] <path>...
```

Claim paths for this session, or release them, with the same rules as the companion's [**Claim files…**](@/docs/mailbox.md#hand-off-files). `--worktree` defaults to the Git root of the current folder, and a claim must run inside that worktree. A path another session holds is refused with `ownership` and names the holder.

```text
Claimed 1 path in /Users/you/Code/app.
```

## mailbox accept and decline

```text
agnostic-ai mailbox accept --thread <id> --n <n>
agnostic-ai mailbox decline --thread <id> --n <n>
```

Answer a handoff offered to this session. Accepting moves the paths only when no other session holds them at that moment. The sender of the offer gets `ownership`, and a session outside the thread gets `not-in-thread`. An agent answers a handoff only after the hook delivered it to its session.

```text
Handoff accepted. Work only in /Users/you/Code/app: src/parser.go.
```

## mailbox stats

```text
agnostic-ai mailbox stats [--since <date>] [--json]
```

Prints counts and durations from every thread on this computer. Run it anywhere; it needs no session, takes no lock, and changes no file. `--since 2026-11-01` counts only messages, events and refusals at or after that date (UTC); a thread counts when it has a message from then on. `--json` prints the same numbers as JSON with schema `mailbox-stats/v1`. The plain output prints each value as its dotted JSON key.

The output holds no message text, paths or session IDs, and draws no conclusions: zero human actions proves neither that you were watching nor that you were not. Every duration is in seconds. With no data, counts are 0 and every median, percentile and ratio is `null`. An empty or missing mailbox exits 0. A thread file that cannot be read is skipped and named on standard error.

| Key | What it counts | Unit, divided by |
|---|---|---|
| `threads.total`, `open`, `paused`, `closing`, `closed` | Threads, by status | Count |
| `threads.reachedCapWithoutDone` | Threads whose content messages reach the cap and that hold no `done` | Count |
| `messages.total`, `byType.*`, `byState.*`, `bySender.*` | Messages, by type, state and `from.by` | Count |
| `refusals.withThread` | Refused sends stored on a thread. A rejected message is not a refusal here; it shows under `humanActions.reject`. A thread keeps its last 100. | Count |
| `refusals.withoutThread` | Refused sends stored in `audit.json`, which keeps the last 500. A note says when it is full. | Count |
| `refusals.total`, `byGuard.*` | Both together, and by guard | Count |
| `refusals.perSendAttempt` | Refusals | Ratio: refusals / (messages + refusals) |
| `deliverySeconds.count`, `median`, `p90` | Delivered messages only, from `queuedAt` to `deliveredAt`. Time held for approval is not in it; staged messages are not in it. The median averages the two middle values; p90 is the nearest rank. | Seconds |
| `humanActions.pause`, `resume`, `stop`, `approve`, `reject`, `raiseCap`, `queue` | Thread events a person made | Count |
| `humanActions.per10Threads` | The seven actions above | Actions / threads × 10 |
| `handoffs.offered`, `accepted`, `declined` | Handoffs by their current answer. Rejected handoffs are not in it. | Count |
| `handoffs.conflicts` | Acceptances refused because the sender no longer held an offered path, once per handoff | Count |
| `handoffs.conflictsPer10Handoffs` | Conflicts | Conflicts / handoffs × 10 |
| `reviews.requests` | Review requests, not counting rejected ones | Count |
| `reviews.crossModel`, `sameModel`, `unknownModel` | Review requests where both models are known and differ, both are known and equal (letter case ignored, whatever the tools), or either is missing. The sender's model is the one stated on the message, or else the one stored for that session. | Count |
| `reviews.firstThreadToFirstEmittedReviewSeconds` | From the first message in the store to the delivery of the first cross-model review request | Seconds |

This run counts one thread: an approved and delivered question, a held finding, and a refused send.

```text
schemaVersion: mailbox-stats/v1
since: null
threads.total: 1
threads.open: 1
threads.paused: 0
threads.closing: 0
threads.closed: 0
threads.reachedCapWithoutDone: 0
messages.total: 2
messages.byType.question: 1
messages.byType.finding: 1
messages.byType.review_request: 0
messages.byType.handoff: 0
messages.byType.done: 0
messages.byState.held: 1
messages.byState.waiting: 0
messages.byState.reserved: 0
messages.byState.staged: 0
messages.byState.delivered: 1
messages.byState.rejected: 0
messages.byState.cancelled: 0
messages.bySender.human: 0
messages.bySender.agent: 2
refusals.total: 1
refusals.withThread: 1
refusals.withoutThread: 0
refusals.byGuard.secret: 1
refusals.perSendAttempt: 0.3333333333333333
deliverySeconds.count: 1
deliverySeconds.median: 0.023458
deliverySeconds.p90: 0.023458
humanActions.pause: 0
humanActions.resume: 0
humanActions.stop: 0
humanActions.approve: 1
humanActions.reject: 0
humanActions.raiseCap: 0
humanActions.queue: 0
humanActions.per10Threads: 10
handoffs.offered: 0
handoffs.accepted: 0
handoffs.declined: 0
handoffs.conflicts: 0
handoffs.conflictsPer10Handoffs: null
reviews.requests: 0
reviews.crossModel: 0
reviews.sameModel: 0
reviews.unknownModel: 0
reviews.firstThreadToFirstEmittedReviewSeconds: null
```

## mailbox refs

Run `agnostic-ai mailbox refs` anywhere inside a Git worktree to print the JSON for a [review request](@/docs/mailbox.md#request-a-code-review). This command only reads Git. Pass its output as `refs` to `mailbox send`, or send the request from the companion.

```text
agnostic-ai mailbox refs [--base <ref>] [--commit <ref>]
```

| Option | Result |
|---|---|
| `--base <ref>` | Compare with this ancestor of the review commit. Without a flag the command uses `origin/HEAD` if it exists. Otherwise, name a base. |
| `--commit <ref>` | Review this commit. Defaults to `HEAD`. |

The JSON includes the worktree root, full commit and base IDs, changed paths and the number of files with uncommitted changes in the current checkout. A detached checkout has no `branch` field. Paths describe the committed diff. The command exits with an error outside a worktree or when it cannot resolve a valid comparison.

This run from a subfolder reviews one committed file against `main`. The worktree path is shortened and the JSON is formatted for readability.

```bash
agnostic-ai mailbox refs --base main
```

```json
{
  "worktree": "/tmp/mailbox-review",
  "commit": "77a7058cbd1780498d98ebdef72c162d0c7eed94",
  "base": "c9b462c24d7507ed26c44541ec3e4c4b66498e10",
  "branch": "review-example",
  "paths": ["src/store.go"],
  "uncommitted": 1
}
```
