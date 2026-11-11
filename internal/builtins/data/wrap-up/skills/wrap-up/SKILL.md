---
name: wrap-up
description: Write an end-of-session summary for people. Use when asked to "wrap up" or "write a wrap-up".
---

# wrap-up

Write a short end-of-session summary for people when the user asks to "wrap up" or "write a wrap-up". Never run at session start, at session end, or after a task without a request.

A wrap-up is for people, when the work ends. For the next agent that continues the work, use the `handoff` skill if it is installed. To start a second task in its own conversation, use the `side-conversation` skill if it is installed.

## Gather

1. Use what happened in this session: the request, the changes made, the checks run with their results, and what was left undone.
2. Inside a Git checkout, read local state only: `git branch --show-current`, `git status --short`, and `git log --oneline -n 20`. Outside a checkout, skip Git and say so in one line.
3. Make no network call: no fetch, pull or push, no Git hosting command or API, no web request. Name a PR only when its number or URL appeared in this session. Never guess one or look one up.
4. Report a commit only when this session made it or the user said it belongs to the task. Report uncommitted changes as uncommitted.

## Write

Reply in the conversation with these three parts, in this order, under these headings. Keep the whole reply on one screen, about 25 lines. When a part is empty, write "nothing" on one line.

```markdown
## What changed
## Still open
## Note to share
```

- **What changed**: one line per outcome, in plain words. Add the PR or the short commit hashes when there are any.
- **Still open**: one line per item, with its reason: unfinished work, failed or skipped checks, uncommitted changes, decisions waiting on someone.
- **Note to share**: at most five lines in a fenced block, ready to paste into a chat or a PR. Write for someone who did not see the session: what was done, what they need to know or do, and what is still open.

## Limits

- In every part, leave out secrets, tokens, `.env` values, and absolute home paths; use paths relative to the project root.
- In the note, use plain words. Do not copy text from files this session only read; say it in your own words.
- Show the result only. Do not post it, commit, push, write a file, or archive the conversation unless the user asks. A wrap-up request alone is not that request.
- State a fact only when the session or local Git shows it. Mark anything else as unknown.
