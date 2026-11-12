---
name: side-conversation
description: Start a side conversation in a new terminal tab, linked to this one. Use when asked to spin a task off, or to follow something up in its own conversation.
---

# side-conversation

Start a second conversation, seeded with a short brief, in a new terminal tab. This session goes on with its own task. The new conversation shows under this one in the Conversations list while both are in the same group.

## When

- Only when the user asks, or agrees after you offer. It opens a tab on the user's screen, so never start one on your own.
- Offer it when a second task comes up that is not needed to finish the current one.
- It is not a subagent: no result comes back to this session, and the user talks to the new conversation. When this session needs the answer, delegate instead.
- It is not a handoff: this session does not end. To pass this same task to the next agent, use the `handoff` skill if it is installed.

## Write the brief

The brief becomes the new conversation's first prompt. It follows the handoff format, cut to what a fresh session needs:

```markdown
<The goal, as one plain sentence.>

## Acceptance
## Context
## Report back
```

- First line: the goal as one plain sentence. It becomes the new row's title.
- **Acceptance**: what done looks like, when it is not obvious.
- **Context**: branch, paths relative to the project root, decisions already made, limits. Name files; do not paste them.
- **Report back**: what to tell the user at the end.
- Leave out a heading that would be empty. Keep the brief under 40 lines.
- Never write secrets, tokens, or `.env` values.
- Do not write the `Continues` line: the command adds it.

## Run

From the folder the work belongs in, pass the brief on standard input through a quoted heredoc, so the shell expands nothing in it:

```bash
agnostic-ai conversations start <<'BRIEF'
Write the release notes for the parser change.

## Context
Branch main. The change is in internal/parser/.

## Report back
The draft, and any change you were unsure about.
BRIEF
```

Add `--tool claude`, `--tool codex` or `--tool cursor` only when the user names the tool. Without it, the new conversation uses this session's tool.

## After

- Say in one line what started and where, from the command's output.
- Go on with the main task. Do not wait for the side conversation or read its transcript.

## When it refuses

- Show the message. It says why: no session found, the tool is not installed, an empty or long brief, or no tab could be opened.
- Do not work around it: no hand-written terminal scripts, no typing into another terminal. When the message gives a line to run, pass it to the user.
- If `agnostic-ai` is not on `PATH`, say so.
- In a sandbox that blocks writing the brief file or controlling the terminal, the command fails. Ask the user to allow it, or to run it themselves.
